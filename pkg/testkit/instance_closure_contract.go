// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"testing"
	"time"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// instanceClosureCases are the cases of InstanceRepositoryContract about the service closure
// (ADR-0088 §1): no closure until one is announced, an announcement with its schedule, a change
// of its dates, a withdrawal and a new announcement after it, refused transitions that write
// nothing, and the service_closure_heads projection equal to a rebuild.
var instanceClosureCases = []struct {
	name string
	run  func(t *testing.T, e *instanceEnv)
}{
	{"HasNoClosureUntilAnnounced", instanceHasNoClosure},
	{"AnnouncesReschedulesAndWithdrawsTheClosure", instanceAnnouncesClosure},
	{"RefusesClosureTransitions", instanceRefusesClosureTransitions},
	{"ClosureProjectionEqualsRebuild", instanceClosureProjectionEqualsRebuild},
}

func (e *instanceEnv) closure(t *testing.T) instance.Closure {
	t.Helper()
	c, err := e.service.Closure(t.Context())
	if err != nil {
		t.Fatalf("Closure = %v", err)
	}
	return c
}

func (e *instanceEnv) announceClosure(t *testing.T, closesAt, exportUntil time.Time, message string) instance.Closure {
	t.Helper()
	e.clock.Advance(time.Second)
	c, err := e.service.AnnounceClosure(t.Context(), appinstance.AnnounceClosureInput{
		ClosesAt: closesAt, ExportUntil: exportUntil, Message: message, ByUserID: ruleModerator,
	})
	if err != nil {
		t.Fatalf("AnnounceClosure = %v", err)
	}
	return c
}

func instanceHasNoClosure(t *testing.T, e *instanceEnv) {
	if c := e.closure(t); c != (instance.Closure{}) || c.CurrentState() != instance.ClosureNone {
		t.Errorf("Closure of a new instance = %+v", c)
	}
}

func instanceAnnouncesClosure(t *testing.T, e *instanceEnv) {
	closes := e.clock.Now().Add(60 * 24 * time.Hour).Truncate(time.Millisecond)
	first := e.announceClosure(t, closes, time.Time{}, "Thank you for everything.")
	want := instance.Closure{
		State: instance.ClosureAnnounced, AnnouncedAt: e.clock.Now(), ChangedAt: e.clock.Now(),
		Schedule: instance.ClosureSchedule{ClosesAt: closes, ExportUntil: closes.Add(instance.DefaultClosureExportPeriod), Message: "Thank you for everything."},
	}
	if got := e.closure(t); got != want || first != want || got.ShortNotice() {
		t.Errorf("Closure after the announcement = %+v\nAnnounceClosure = %+v\nwant %+v", got, first, want)
	}

	// A change of the dates is a new announcement, whose time starts the notice again.
	sooner, until := e.clock.Now().Add(10*24*time.Hour).Truncate(time.Millisecond), closes
	second := e.announceClosure(t, sooner, until, "")
	if second.Schedule.ClosesAt != sooner || second.Schedule.ExportUntil != until || second.Schedule.Message != "" ||
		second.AnnouncedAt != e.clock.Now() || !second.ShortNotice() {
		t.Errorf("Closure after the change of the dates = %+v", second)
	}

	e.clock.Advance(time.Second)
	withdrawn, err := e.service.WithdrawClosure(t.Context(), "")
	if err != nil || withdrawn != (instance.Closure{}) || e.closure(t) != (instance.Closure{}) {
		t.Errorf("WithdrawClosure = %+v, %v; Closure = %+v", withdrawn, err, e.closure(t))
	}
	// An announcement after the withdrawal is a new announcement.
	again := e.announceClosure(t, closes, time.Time{}, "")
	if again.State != instance.ClosureAnnounced || again.Schedule.ClosesAt != closes || e.closure(t) != again {
		t.Errorf("Closure after a new announcement = %+v", again)
	}
}

func instanceRefusesClosureTransitions(t *testing.T, e *instanceEnv) {
	if _, err := e.service.WithdrawClosure(t.Context(), ruleModerator); !errors.Is(err, instance.ErrClosureTransition) {
		t.Errorf("WithdrawClosure without an announcement = %v, want ErrClosureTransition", err)
	}
	closes := e.clock.Now().Add(time.Hour)
	_, err := e.service.AnnounceClosure(t.Context(), appinstance.AnnounceClosureInput{ClosesAt: closes, ExportUntil: closes.Add(-time.Second)})
	if !errors.Is(err, instance.ErrInvalidValue) {
		t.Errorf("AnnounceClosure(export before closing) = %v, want ErrInvalidValue", err)
	}
	if c := e.closure(t); c != (instance.Closure{}) {
		t.Errorf("a refused operation recorded %+v", c)
	}
}

func instanceClosureProjectionEqualsRebuild(t *testing.T, e *instanceEnv) {
	ctx := t.Context()
	closes := e.clock.Now().Add(40 * 24 * time.Hour).Truncate(time.Millisecond)
	e.announceClosure(t, closes, time.Time{}, "first")
	if _, err := e.service.WithdrawClosure(ctx, ""); err != nil {
		t.Fatal(err)
	}
	e.announceClosure(t, closes, time.Time{}, "second")
	want := e.announceClosure(t, closes.Add(time.Hour), time.Time{}, "third")
	if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
		t.Fatal(err)
	}
	if got := e.closure(t); got != want {
		t.Errorf("Closure after rebuild = %+v, want %+v", got, want)
	}
	if _, err := e.service.WithdrawClosure(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := appinstance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Instance); err != nil {
		t.Fatal(err)
	}
	if got := e.closure(t); got != (instance.Closure{}) {
		t.Errorf("Closure after a withdrawal and a rebuild = %+v", got)
	}
}
