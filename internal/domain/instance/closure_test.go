// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
)

var closureNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func TestNewClosureSchedule(t *testing.T) {
	closes := closureNow.Add(60 * 24 * time.Hour)
	s, err := instance.NewClosureSchedule(closes, time.Time{}, "  Thank you.\n")
	if err != nil || !s.ClosesAt.Equal(closes) || !s.ExportUntil.Equal(closes.Add(30*24*time.Hour)) || s.Message != "Thank you." {
		t.Errorf("NewClosureSchedule(default export) = %+v, %v", s, err)
	}
	// The closing date has no lower bound, and the export deadline may be the closing date.
	past := closureNow.Add(-time.Hour)
	if s, err := instance.NewClosureSchedule(past, past, ""); err != nil || !s.ExportUntil.Equal(past) {
		t.Errorf("NewClosureSchedule(past, same export) = %+v, %v", s, err)
	}
	for _, tc := range []struct {
		name          string
		closes, until time.Time
		message       string
		want          error
	}{
		{"no closing date", time.Time{}, closes, "", instance.ErrClosureSchedule},
		{"export before closing", closes, closes.Add(-time.Second), "", instance.ErrClosureSchedule},
		{"invalid text", closes, time.Time{}, "a\x00b", instance.ErrClosureMessage},
		{"too long", closes, time.Time{}, strings.Repeat("あ", instance.MaxClosureMessageLength+1), instance.ErrClosureMessage},
	} {
		if _, err := instance.NewClosureSchedule(tc.closes, tc.until, tc.message); !errors.Is(err, tc.want) || !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("%s: NewClosureSchedule = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func closureIn(state instance.ClosureState, closesAt time.Time) instance.Closure {
	if state == instance.ClosureNone {
		return instance.Closure{}
	}
	return instance.Closure{State: state, Schedule: instance.ClosureSchedule{ClosesAt: closesAt, ExportUntil: closesAt}}
}

func TestClosureTransitions(t *testing.T) {
	closes := closureNow.Add(time.Hour)
	ok := map[instance.ClosureState][]instance.ClosureEventKind{
		instance.ClosureNone:              {instance.ClosureEventAnnounced},
		instance.ClosureAnnounced:         {instance.ClosureEventAnnounced, instance.ClosureEventWithdrawn},
		instance.ClosureClosed:            {instance.ClosureEventDeliveryCompleted},
		instance.ClosureDeliveryCompleted: nil,
	}
	for _, state := range instance.ClosureStates() {
		for _, kind := range instance.ClosureEventKinds() {
			err := closureIn(state, closes).Check(kind, closureNow)
			want := false
			for _, k := range ok[state] {
				want = want || k == kind
			}
			if (err == nil) != want || err != nil && !errors.Is(err, instance.ErrClosureTransition) {
				t.Errorf("Check(%s) in %s = %v, want allowed %v", kind, state, err, want)
			}
		}
	}

	announced := closureIn(instance.ClosureAnnounced, closes)
	// The closure waits for the closing date, and is allowed from it on.
	if err := announced.Check(instance.ClosureEventClosed, closureNow); !errors.Is(err, instance.ErrClosureNotDue) {
		t.Errorf("Check(closed) before closes_at = %v, want ErrClosureNotDue", err)
	}
	for _, at := range []time.Time{closes, closes.Add(time.Second)} {
		if err := announced.Check(instance.ClosureEventClosed, at); err != nil {
			t.Errorf("Check(closed) at %s = %v", at, err)
		}
	}
	// Nothing goes back after the closure: no withdrawal and no new announcement.
	for _, state := range []instance.ClosureState{instance.ClosureClosed, instance.ClosureDeliveryCompleted} {
		for _, kind := range []instance.ClosureEventKind{instance.ClosureEventWithdrawn, instance.ClosureEventAnnounced, instance.ClosureEventClosed} {
			if err := closureIn(state, closes).Check(kind, closes.Add(time.Hour)); !errors.Is(err, instance.ErrClosureTransition) {
				t.Errorf("Check(%s) in %s = %v, want ErrClosureTransition", kind, state, err)
			}
		}
	}
}

func TestClosureHeadApply(t *testing.T) {
	ev := func(id string, kind instance.ClosureEventKind) instance.ClosureEvent {
		return instance.ClosureEvent{ID: id, Kind: kind}
	}
	var h instance.ClosureHead
	// An announcement after a withdrawal starts a new announcement.
	for _, step := range []struct {
		e    instance.ClosureEvent
		want instance.ClosureHead
	}{
		{ev("01", instance.ClosureEventAnnounced), instance.ClosureHead{State: instance.ClosureAnnounced, EventID: "01", AnnouncementID: "01"}},
		{ev("02", instance.ClosureEventWithdrawn), instance.ClosureHead{}},
		{ev("03", instance.ClosureEventAnnounced), instance.ClosureHead{State: instance.ClosureAnnounced, EventID: "03", AnnouncementID: "03"}},
		{ev("04", instance.ClosureEventAnnounced), instance.ClosureHead{State: instance.ClosureAnnounced, EventID: "04", AnnouncementID: "04"}},
		{ev("05", instance.ClosureEventClosed), instance.ClosureHead{State: instance.ClosureClosed, EventID: "05", AnnouncementID: "04"}},
		// An event not newer than the head changes nothing.
		{ev("03", instance.ClosureEventWithdrawn), instance.ClosureHead{State: instance.ClosureClosed, EventID: "05", AnnouncementID: "04"}},
		{ev("06", instance.ClosureEventDeliveryCompleted), instance.ClosureHead{State: instance.ClosureDeliveryCompleted, EventID: "06", AnnouncementID: "04"}},
	} {
		if h = h.Apply(step.e); h != step.want {
			t.Errorf("Apply(%+v) = %+v, want %+v", step.e, h, step.want)
		}
	}
}

func TestClosureShortNotice(t *testing.T) {
	c := instance.Closure{State: instance.ClosureAnnounced, AnnouncedAt: closureNow,
		Schedule: instance.ClosureSchedule{ClosesAt: closureNow.Add(instance.ClosureNoticeWarning - time.Second)}}
	if !c.ShortNotice() {
		t.Error("a notice under 30 days is not short")
	}
	c.Schedule.ClosesAt = closureNow.Add(instance.ClosureNoticeWarning)
	if c.ShortNotice() {
		t.Error("a notice of 30 days is short")
	}
	if (instance.Closure{}).ShortNotice() || (instance.Closure{}).CurrentState() != instance.ClosureNone {
		t.Error("no closure has a short notice or a state besides none")
	}
}

func TestClosureAllows(t *testing.T) {
	all := instance.ClosureStates()
	running := []instance.ClosureState{instance.ClosureNone, instance.ClosureAnnounced}
	delivering := []instance.ClosureState{instance.ClosureNone, instance.ClosureAnnounced, instance.ClosureClosed}
	want := map[instance.ClosureOperation][]instance.ClosureState{
		instance.ClosureSignIn:      all,
		instance.ClosureRegister:    {instance.ClosureNone},
		instance.ClosureSubscribe:   {instance.ClosureNone},
		instance.ClosureOwnData:     all,
		instance.ClosureClientAPI:   running,
		instance.ClosureStream:      running,
		instance.ClosureFederate:    running,
		instance.ClosureRetract:     delivering,
		instance.ClosureServeActors: delivering,
		instance.ClosureWebFinger:   running,
		instance.ClosureNodeInfo:    all,
	}
	for _, op := range instance.ClosureOperations() {
		for _, state := range all {
			allowed := false
			for _, s := range want[op] {
				allowed = allowed || s == state
			}
			if got := instance.ClosureAllows(state, op); got != allowed {
				t.Errorf("ClosureAllows(%s, %s) = %v, want %v", state, op, got, allowed)
			}
		}
	}
	if !instance.ClosureAllows("", instance.ClosureRegister) || instance.ClosureAllows("unknown", instance.ClosureSignIn) ||
		instance.ClosureAllows(instance.ClosureNone, "unknown") {
		t.Error("ClosureAllows of the empty or unknown state or operation")
	}
}
