// SPDX-License-Identifier: AGPL-3.0-only

package gatheringtest

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/kernel"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/pkg/testkit"
)

// DueContract verifies the due times, the reminders, and the recipients of the notifications of
// the gathering repository of a database adapter through appgathering.Service (ADR-0161 §11,
// §12, data-model §4.19): the creation, the answers, the reminders, a new date, and the
// cancellation keep gathering_dues; gathering.tick enqueues what came and moves the due time on;
// the reminders of an account read back, none included, and its erasure forgets them; the
// audience of an event lists who answered going or maybe and the invitees, a page at a time; and
// gathering_dues rebuilds.
func DueContract(t *testing.T, newHarness Factory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *dueEnv)
	}{
		{"KeepsTheDueTimes", keepsDues},
		{"TicksWhatCame", ticks},
		{"KeepsTheReminders", reminders},
		{"ListsTheAudience", audience},
		{"RebuildsTheDueTimes", rebuildDues},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newDueEnv(t, newHarness))
		})
	}
}

type dueEnv struct {
	*env
	outbox *testkit.RecordingOutbox
}

// nopNotifier stands in for the notification module, which the contract does not reach.
type nopNotifier struct{}

func (nopNotifier) NotifyEventTx(context.Context, uow.Tx, appnotification.EventNotice, []string) error {
	return nil
}

func newDueEnv(t *testing.T, newHarness Factory) *dueEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appgathering.RegisterProjectors(projectors, h.Gathering)
	appgathering.RegisterDueProjectors(projectors, h.Gathering)
	clock := testkit.NewClock(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	e := &dueEnv{env: &env{h: h, clock: clock, members: newMembers(), relations: newRelations()}, outbox: &testkit.RecordingOutbox{}}
	e.svc = appgathering.NewService(appgathering.Deps{
		Repository: h.Gathering, UnitOfWork: h.UnitOfWork, Clock: clock, IDs: testkit.NewIDGenerator(clock),
		RateLimiter: kernel.NewRateLimiter(testkit.NewKeyValueStore(clock), clock), Members: e.members, Relations: e.relations,
		Outbox: e.outbox, Notifier: nopNotifier{},
	})
	e.svc.RegisterNotificationProjectors(projectors)
	return e
}

// due returns the due time of the event id, the zero time without one.
func (e *dueEnv) due(t *testing.T, id string) time.Time {
	t.Helper()
	at, ok, err := e.h.Gathering.FindDue(t.Context(), nil, id)
	if err != nil {
		t.Fatalf("FindDue: %v", err)
	}
	if !ok {
		return time.Time{}
	}
	return at.UTC()
}

func wantDue(t *testing.T, what string, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s: due = %v, want %v", what, got, want)
	}
}

func (e *dueEnv) setReminders(t *testing.T, accountID string, minutes []int) appgathering.Reminders {
	t.Helper()
	r, err := e.svc.SetReminders(t.Context(), accountID, minutes)
	if err != nil {
		t.Fatalf("SetReminders(%v): %v", minutes, err)
	}
	return r
}

func keepsDues(t *testing.T, e *dueEnv) {
	v := e.create(t, alice, settings("Picnic", gathering.VisibilityPublic))
	start := v.Confirmation.Schedule.StartsAt
	wantDue(t, "created", e.due(t, v.ID), start)
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	// One day before has passed already (the event starts in a day): the hour before is next.
	wantDue(t, "bob going", e.due(t, v.ID), start.Add(-time.Hour))
	e.setReminders(t, carol, []int{120})
	e.answer(t, carol, v.ID, gathering.AnswerMaybe)
	wantDue(t, "carol maybe", e.due(t, v.ID), start.Add(-2*time.Hour))
	e.answer(t, carol, v.ID, gathering.AnswerDeclined)
	wantDue(t, "carol declined", e.due(t, v.ID), start.Add(-time.Hour))
	e.setReminders(t, bob, []int{180})
	wantDue(t, "bob's reminders", e.due(t, v.ID), start.Add(-3*time.Hour))

	later := gathering.Schedule{StartsAt: start.Add(48 * time.Hour), EndsAt: start.Add(49 * time.Hour)}
	if _, err := e.svc.Update(t.Context(), alice, v.ID, appgathering.Patch{Schedule: &later}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	wantDue(t, "rescheduled", e.due(t, v.ID), later.StartsAt.Add(-3*time.Hour))
	if _, err := e.svc.Cancel(t.Context(), alice, v.ID, ""); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	wantDue(t, "cancelled", e.due(t, v.ID), time.Time{})
}

// jobKinds returns the kinds and minutes of the gathering.notify jobs among jobs, sorted.
func jobKinds(t *testing.T, jobs []json.RawMessage) []string {
	t.Helper()
	var out []string
	for _, b := range jobs {
		var p struct {
			Kind    string `json:"kind"`
			Ref     string `json:"ref"`
			Minutes int    `json:"minutes"`
		}
		if err := json.Unmarshal(b, &p); err != nil {
			t.Fatal(err)
		}
		k := p.Kind
		if p.Minutes > 0 {
			k += ":" + time.Duration(p.Minutes*int(time.Minute)).String()
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (e *dueEnv) notifyJobs(t *testing.T) []string {
	t.Helper()
	var payloads []json.RawMessage
	for _, j := range e.outbox.Take() {
		if j.Type == appgathering.JobNotify {
			payloads = append(payloads, j.Payload)
		}
	}
	return jobKinds(t, payloads)
}

func ticks(t *testing.T, e *dueEnv) {
	v := e.create(t, alice, settings("Picnic", gathering.VisibilityPublic))
	start := v.Confirmation.Schedule.StartsAt
	e.setReminders(t, bob, []int{120, 60})
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	e.outbox.Take()

	tick := func(at time.Time) []string {
		t.Helper()
		e.clock.Set(at)
		if err := e.svc.Tick(t.Context()); err != nil {
			t.Fatalf("Tick: %v", err)
		}
		return e.notifyJobs(t)
	}
	if got := tick(start.Add(-3 * time.Hour)); len(got) != 0 {
		t.Errorf("before the first reminder: jobs %v, want none", got)
	}
	// A late tick handles both reminders that came since the due time.
	if got, want := tick(start.Add(-59*time.Minute)), []string{"reminder:1h0m0s", "reminder:2h0m0s"}; !reflect.DeepEqual(got, want) {
		t.Errorf("late tick: jobs %v, want %v", got, want)
	}
	wantDue(t, "after the reminders", e.due(t, v.ID), start)
	if got := tick(start.Add(-30 * time.Minute)); len(got) != 0 {
		t.Errorf("again before the start: jobs %v, want none", got)
	}
	if got, want := tick(start.Add(time.Minute)), []string{"started"}; !reflect.DeepEqual(got, want) {
		t.Errorf("at the start: jobs %v, want %v", got, want)
	}
	wantDue(t, "after the start", e.due(t, v.ID), time.Time{})
	if got := tick(start.Add(2 * time.Minute)); len(got) != 0 {
		t.Errorf("after the start: jobs %v, want none", got)
	}
}

func reminders(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	r, err := e.svc.Reminders(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Defaults || !slices.Equal(r.Minutes, gathering.DefaultReminders) {
		t.Errorf("Reminders before a choice = %+v, want the defaults", r)
	}
	if r := e.setReminders(t, bob, []int{30, 10080, 5}); r.Defaults || !slices.Equal(r.Minutes, []int{10080, 30, 5}) {
		t.Errorf("SetReminders = %+v, want the three from the earliest", r)
	}
	if r := e.setReminders(t, bob, []int{}); r.Defaults || len(r.Minutes) != 0 {
		t.Errorf("SetReminders(none) = %+v, want no reminders", r)
	}
	for _, bad := range [][]int{{4}, {10081}, {60, 60}, {5, 10, 15, 20}} {
		if _, err := e.svc.SetReminders(ctx, bob, bad); !errors.Is(err, gathering.ErrInvalidReminders) {
			t.Errorf("SetReminders(%v) = %v, want ErrInvalidReminders", bad, err)
		}
	}
	e.setReminders(t, bob, []int{45})
	step, err := appgathering.NewEraser(e.h.UnitOfWork, e.h.Gathering).EraseAccountData(ctx, account.ErasureTarget{AccountID: bob}, "", 100)
	if err != nil || !step.Done {
		t.Fatalf("EraseAccountData = %+v, %v", step, err)
	}
	if r, err := e.svc.Reminders(ctx, bob); err != nil || !r.Defaults {
		t.Errorf("Reminders after the erasure = %+v, %v; want the defaults", r, err)
	}
	if r := e.setReminders(t, bob, nil); !r.Defaults {
		t.Errorf("SetReminders(nil) = %+v, want the defaults", r)
	}
}

func audience(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	v := e.create(t, alice, settings("Picnic", gathering.VisibilityPublic))
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	e.answer(t, carol, v.ID, gathering.AnswerMaybe)
	e.answer(t, dave, v.ID, gathering.AnswerDeclined)
	for _, id := range []string{erin, frank, bob} {
		if _, err := e.svc.Invite(ctx, alice, v.ID, id); err != nil {
			t.Fatalf("Invite: %v", err)
		}
	}
	if err := e.svc.RevokeInvitation(ctx, alice, v.ID, frank); err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}
	read := func(invitees bool, limit int) []string {
		t.Helper()
		var out []string
		after := ""
		for range 10 {
			ids, err := e.h.Gathering.Audience(ctx, nil, v.ID, invitees, after, limit)
			if err != nil {
				t.Fatalf("Audience: %v", err)
			}
			out = append(out, ids...)
			if len(ids) < limit {
				return out
			}
			after = ids[len(ids)-1]
		}
		t.Fatal("Audience does not end")
		return nil
	}
	sorted := func(ids ...string) []string {
		out := slices.Clone(ids)
		slices.Sort(out)
		return out
	}
	if got, want := read(false, 10), sorted(bob, carol); !slices.Equal(got, want) {
		t.Errorf("Audience = %v, want going and maybe %v", got, want)
	}
	if got, want := read(true, 1), sorted(bob, carol, erin); !slices.Equal(got, want) {
		t.Errorf("Audience with the invitees, a page of one = %v, want %v", got, want)
	}
	facts, err := e.h.Gathering.AudienceFacts(ctx, nil, v.ID, []string{bob, dave, erin, frank})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]appgathering.ViewerFacts{
		bob: {Invited: true, Answer: gathering.AnswerGoing}, dave: {Answer: gathering.AnswerDeclined},
		erin: {Invited: true}, frank: {},
	}
	if !reflect.DeepEqual(facts, want) {
		t.Errorf("AudienceFacts = %+v, want %+v", facts, want)
	}
	chosen, err := e.h.Gathering.Reminders(ctx, nil, []string{bob, carol})
	if err != nil || len(chosen) != 0 {
		t.Errorf("Reminders before a choice = %v, %v; want none", chosen, err)
	}
}

func rebuildDues(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	v := e.create(t, alice, settings("Picnic", gathering.VisibilityPublic))
	w := e.create(t, bob, settings("Karaoke", gathering.VisibilityPublic))
	e.answer(t, carol, v.ID, gathering.AnswerGoing)
	if _, err := e.svc.Cancel(ctx, bob, w.ID, ""); err != nil {
		t.Fatal(err)
	}
	before := e.due(t, v.ID)
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Gathering.DeleteAllDues(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	if err := appgathering.RebuildDues(ctx, e.h.UnitOfWork, e.h.Gathering, e.clock.Now()); err != nil {
		t.Fatalf("RebuildDues: %v", err)
	}
	wantDue(t, "rebuilt", e.due(t, v.ID), before)
	wantDue(t, "rebuilt, cancelled", e.due(t, w.ID), time.Time{})
}
