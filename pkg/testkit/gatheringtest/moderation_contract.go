// SPDX-License-Identifier: AGPL-3.0-only

package gatheringtest

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/pkg/testkit"
)

// ModerationContract verifies the moderation of the events on a database adapter through the use
// cases of appgathering.Service (ADR-0161 §14): a report points at the revision that the reporter
// sees and the moderators read every revision with its places; a moderator's cancellation records
// the moderator, the reason, and its audit log entry once, for a date poll too; the events of a
// suspended organizer that have not started, its date polls included, are cancelled, and nothing
// else; and the answers, votes, and invitations of the events that ended (a date poll at the end
// of its candidates) longer than the retention ago are erased, except those of a reported event
// and those that name a held account, while the candidates stay.
func ModerationContract(t *testing.T, newHarness Factory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *modEnv)
	}{
		{"ReportsAndCancelsAsAModerator", reportsAndCancels},
		{"CancelsTheEventsOfASuspendedOrganizer", cancelsOrganized},
		{"PurgesTheRecordsAfterTheRetention", purgesRecords},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newModEnv(t, newHarness))
		})
	}
}

// retention is the fake Retention of the contract.
type retention struct {
	mu       sync.Mutex
	period   time.Duration
	reported map[string]bool
	held     []string
}

func (r *retention) RecordsRetention(context.Context) (time.Duration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.period, nil
}

func (r *retention) ReportedGatherings(_ context.Context, ids []string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for _, id := range ids {
		if r.reported[id] {
			out[id] = true
		}
	}
	return out, nil
}

func (r *retention) ReportedGatheringsTx(ctx context.Context, _ uow.Tx, ids []string) (map[string]bool, error) {
	return r.ReportedGatherings(ctx, ids)
}

func (r *retention) HeldAccountIDs(context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.held), nil
}

func (r *retention) HeldAccountIDsTx(ctx context.Context, _ uow.Tx) ([]string, error) {
	return r.HeldAccountIDs(ctx)
}

type modEnv struct {
	*env
	retention *retention
	mu        sync.Mutex
	audits    []moderation.AuditEntry
}

func newModEnv(t *testing.T, newHarness Factory) *modEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appgathering.RegisterProjectors(projectors, h.Gathering)
	clock := testkit.NewClock(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	e := &modEnv{
		env:       &env{h: h, clock: clock, members: newMembers(), relations: newRelations()},
		retention: &retention{period: 90 * 24 * time.Hour, reported: map[string]bool{}},
	}
	projectors.Register(appmoderation.EventAuditRecorded, func(_ context.Context, _ uow.Tx, ev uow.Event) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.audits = append(e.audits, ev.(appmoderation.AuditRecorded).Entry)
		return nil
	})
	e.svc = appgathering.NewService(appgathering.Deps{
		Repository: h.Gathering, UnitOfWork: h.UnitOfWork, Clock: clock, IDs: testkit.NewIDGenerator(clock),
		RateLimiter: kernel.NewRateLimiter(testkit.NewKeyValueStore(clock), clock), Members: e.members, Relations: e.relations,
		Retention: e.retention, Store: testkit.NewKeyValueStore(clock),
	})
	return e
}

// createPoll creates an unscheduled event of organizer whose date poll has two candidates, a day
// apart, from the day after the clock.
func (e *modEnv) createPoll(t *testing.T, organizer string, s gathering.Settings) appgathering.View {
	t.Helper()
	first := e.schedule()
	second := gathering.Schedule{StartsAt: first.StartsAt.Add(24 * time.Hour), EndsAt: first.EndsAt.Add(24 * time.Hour)}
	v, err := e.svc.Create(t.Context(), organizer, appgathering.CreateInput{Settings: s, Candidates: []gathering.Schedule{first, second}})
	if err != nil {
		t.Fatalf("Create(a poll): %v", err)
	}
	return v
}

func (e *modEnv) record(t *testing.T, id string) appgathering.Record {
	t.Helper()
	rec, err := e.h.Gathering.FindGathering(t.Context(), nil, id)
	if err != nil {
		t.Fatalf("FindGathering: %v", err)
	}
	return rec
}

func reportsAndCancels(t *testing.T, e *modEnv) {
	ctx := t.Context()
	v := e.create(t, alice, settings("Reported", gathering.VisibilityPublic))
	target, err := e.svc.ReportTarget(ctx, bob, v.ID)
	if err != nil || target != (appgathering.ReportTarget{GatheringID: v.ID, RevisionID: v.Revision.ID, OrganizerAccountID: alice}) {
		t.Errorf("ReportTarget = %+v, %v; want the event, its revision, and its organizer", target, err)
	}
	hidden := e.create(t, alice, settings("Invited", gathering.VisibilityInvited))
	if _, err := e.svc.ReportTarget(ctx, bob, hidden.ID); !errors.Is(err, appgathering.ErrNotFound) {
		t.Errorf("ReportTarget(an event the reporter does not see) = %v, want ErrNotFound", err)
	}

	// The organizer moves the event after the report: the moderators read both revisions, and the
	// places of the one reported, which bob did not see.
	title, none := "Moved", []gathering.Place{}
	if _, err := e.svc.Update(ctx, alice, v.ID, appgathering.Patch{Title: &title, Places: &none}); err != nil {
		t.Fatal(err)
	}
	recs, err := e.svc.EventRecords(ctx, v.ID)
	if err != nil || len(recs.Revisions) != 2 || recs.Revisions[0].ID != target.RevisionID ||
		len(recs.Revisions[0].Settings.Places) != 3 || recs.Revisions[0].Settings.Places[0].Address != "1-1 Chiyoda" ||
		recs.Revisions[1].Settings.Title != "Moved" || len(recs.Revisions[1].Settings.Places) != 0 {
		t.Errorf("EventRecords = %+v, %v; want both revisions, the reported one with its places", recs.Revisions, err)
	}
	if _, err := e.svc.EventRecords(ctx, testkit.NamedID("unknown event")); !errors.Is(err, appgathering.ErrNotFound) {
		t.Errorf("EventRecords(unknown) = %v, want ErrNotFound", err)
	}

	moderator := testkit.NamedID("gathering-moderator")
	in := appgathering.ModeratorCancelInput{GatheringID: v.ID, ByUserID: testkit.NamedID("moderator user"), ByAccountID: moderator, Note: string(make([]rune, gathering.MaxNoteLength+1))}
	var se *gathering.SettingsError
	if err := e.svc.CancelByModerator(ctx, in); !errors.As(err, &se) {
		t.Errorf("CancelByModerator(a long note) = %v, want a *SettingsError", err)
	}
	in.Note = "Harassment"
	if err := e.svc.CancelByModerator(ctx, appgathering.ModeratorCancelInput{GatheringID: testkit.NamedID("unknown event")}); !errors.Is(err, appgathering.ErrNotFound) {
		t.Errorf("CancelByModerator(unknown) = %v, want ErrNotFound", err)
	}
	if err := e.svc.CancelByModerator(ctx, in); err != nil {
		t.Fatalf("CancelByModerator = %v", err)
	}
	c := e.record(t, v.ID).Cancellation
	if c == nil || c.Reason != gathering.CancelModerator || c.CancelledByAccountID != moderator || c.Note != "Harassment" {
		t.Errorf("cancellation = %+v, want the moderator's", c)
	}
	if err := e.svc.CancelByModerator(ctx, in); !errors.Is(err, appgathering.ErrClosed) {
		t.Errorf("CancelByModerator(a cancelled event) = %v, want ErrClosed", err)
	}

	// A date poll, which has no date yet, is reported and cancelled the same way.
	poll := e.createPoll(t, alice, settings("Poll", gathering.VisibilityPublic))
	if target, err := e.svc.ReportTarget(ctx, bob, poll.ID); err != nil || target.RevisionID != poll.Revision.ID || target.OrganizerAccountID != alice {
		t.Errorf("ReportTarget(a poll) = %+v, %v", target, err)
	}
	if err := e.svc.CancelByModerator(ctx, appgathering.ModeratorCancelInput{GatheringID: poll.ID, ByUserID: in.ByUserID, ByAccountID: moderator}); err != nil {
		t.Fatalf("CancelByModerator(a poll) = %v", err)
	}
	if rec := e.record(t, poll.ID); rec.Cancellation == nil || rec.Cancellation.Reason != gathering.CancelModerator || rec.Head.State != gathering.StateCancelled {
		t.Errorf("the poll = %+v, want it cancelled by the moderator", rec)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.audits) != 2 || e.audits[1].TargetID != poll.ID {
		t.Fatalf("audit log = %+v, want the two cancellations", e.audits)
	}
	if a := e.audits[0]; a.Action != moderation.AuditEventCancelled || a.TargetKind != moderation.AuditTargetEvent || a.TargetID != v.ID ||
		a.ActorKind != moderation.AuditActorUser || a.ActorUserID != in.ByUserID || a.SourceEventID != v.ID {
		t.Errorf("audit entry = %+v", a)
	}
}

func cancelsOrganized(t *testing.T, e *modEnv) {
	ctx := t.Context()
	a := e.create(t, alice, settings("A", gathering.VisibilityPublic))
	b := e.create(t, alice, settings("B", gathering.VisibilityInvited))
	cancelled := e.create(t, alice, settings("Cancelled", gathering.VisibilityPublic))
	if _, err := e.svc.Cancel(ctx, alice, cancelled.ID, "Rain"); err != nil {
		t.Fatal(err)
	}
	soon := gathering.Schedule{StartsAt: e.clock.Now().Add(time.Hour), EndsAt: e.clock.Now().Add(5 * time.Hour)}
	started, err := e.svc.Create(ctx, alice, appgathering.CreateInput{Settings: settings("Started", gathering.VisibilityPublic), Schedule: soon})
	if err != nil {
		t.Fatal(err)
	}
	other := e.create(t, bob, settings("Bob's", gathering.VisibilityPublic))
	poll := e.createPoll(t, alice, settings("Poll", gathering.VisibilityPublic))
	e.clock.Advance(2 * time.Hour)

	want := []string{a.ID, b.ID, poll.ID}
	slices.Sort(want)
	if got, err := e.h.Gathering.OrganizedUpcoming(ctx, nil, alice, e.clock.Now(), 10); err != nil || !slices.Equal(got, want) {
		t.Errorf("OrganizedUpcoming = %v, %v; want %v", got, err, want)
	}
	if n, err := e.svc.CancelOrganized(ctx, alice, gathering.CancelOrganizerSuspended); err != nil || n != 0 {
		t.Errorf("CancelOrganized(an active organizer) = %d, %v; want none", n, err)
	}
	e.members.mu.Lock()
	e.members.suspended[alice] = true
	e.members.mu.Unlock()
	if n, err := e.svc.CancelOrganized(ctx, alice, gathering.CancelOrganizerSuspended); err != nil || n != 3 {
		t.Errorf("CancelOrganized = %d, %v; want the two upcoming events and the poll", n, err)
	}
	for _, id := range want {
		c := e.record(t, id).Cancellation
		if c == nil || c.Reason != gathering.CancelOrganizerSuspended || c.CancelledByAccountID != "" || c.Note != "" {
			t.Errorf("cancellation of %s = %+v, want organizer_suspended by nobody", id, c)
		}
	}
	if c := e.record(t, cancelled.ID).Cancellation; c == nil || c.Reason != gathering.CancelOrganizer {
		t.Errorf("the cancellation of a cancelled event = %+v, want the organizer's", c)
	}
	for _, id := range []string{started.ID, other.ID} {
		if c := e.record(t, id).Cancellation; c != nil {
			t.Errorf("event %s = %+v, want it not cancelled", id, c)
		}
	}
	if n, err := e.svc.CancelOrganized(ctx, alice, gathering.CancelOrganizerSuspended); err != nil || n != 0 {
		t.Errorf("CancelOrganized again = %d, %v; want none", n, err)
	}
}

func purgesRecords(t *testing.T, e *modEnv) {
	ctx := t.Context()
	ended := e.create(t, alice, settings("Ended", gathering.VisibilityPublic))
	e.answer(t, bob, ended.ID, gathering.AnswerGoing)
	if _, err := e.svc.Approve(ctx, alice, ended.ID, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Invite(ctx, alice, ended.ID, carol); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RevokeInvitation(ctx, alice, ended.ID, carol); err != nil {
		t.Fatal(err)
	}
	cancelled := e.create(t, alice, settings("Cancelled", gathering.VisibilityPublic))
	e.answer(t, bob, cancelled.ID, gathering.AnswerMaybe)
	if _, err := e.svc.Cancel(ctx, alice, cancelled.ID, ""); err != nil {
		t.Fatal(err)
	}
	reported := e.create(t, alice, settings("Reported", gathering.VisibilityPublic))
	e.answer(t, bob, reported.ID, gathering.AnswerGoing)
	held := e.create(t, alice, settings("Held", gathering.VisibilityPublic))
	e.answer(t, dave, held.ID, gathering.AnswerGoing)
	poll := e.createPoll(t, alice, settings("Poll", gathering.VisibilityPublic))
	pollCandidates, err := e.svc.Candidates(ctx, alice, poll.ID)
	if err != nil || len(pollCandidates) != 2 {
		t.Fatalf("Candidates = %+v, %v", pollCandidates, err)
	}
	if _, err := e.svc.Vote(ctx, erin, poll.ID, pollCandidates[0].ID, gathering.VoteYes); err != nil {
		t.Fatal(err)
	}
	e.retention.reported[reported.ID] = true
	e.retention.held = []string{dave}

	// Ninety days after the end of the events but one, which is still ahead.
	e.clock.Advance(93 * 24 * time.Hour)
	ahead := e.create(t, alice, settings("Ahead", gathering.VisibilityPublic))
	e.answer(t, bob, ahead.ID, gathering.AnswerGoing)

	if accounts, err := e.h.Gathering.RecordAccounts(ctx, nil, ended.ID); err != nil || !slices.Contains(accounts, bob) || !slices.Contains(accounts, carol) || !slices.Contains(accounts, alice) {
		t.Errorf("RecordAccounts = %v, %v; want bob, carol, and alice", accounts, err)
	}
	if accounts, err := e.h.Gathering.RecordAccounts(ctx, nil, poll.ID); err != nil || !slices.Contains(accounts, erin) {
		t.Errorf("RecordAccounts(a poll) = %v, %v; want the voter", accounts, err)
	}
	if n, err := e.svc.PurgeRecords(ctx); err != nil || n != 3 {
		t.Errorf("PurgeRecords = %d, %v; want the ended and the cancelled event and the poll past its candidates", n, err)
	}
	if accounts, err := e.h.Gathering.RecordAccounts(ctx, nil, poll.ID); err != nil || len(accounts) != 0 {
		t.Errorf("RecordAccounts(a purged poll) = %v, %v; want none", accounts, err)
	}
	if cs, err := e.svc.Candidates(ctx, alice, poll.ID); err != nil || len(cs) != 2 || cs[0].Yes != 0 {
		t.Errorf("Candidates after the purge = %+v, %v; want both candidates kept without their votes", cs, err)
	}
	for _, id := range []string{ended.ID, cancelled.ID} {
		if _, err := e.h.Gathering.FindAttendee(ctx, nil, id, bob); !errors.Is(err, appgathering.ErrNotFound) {
			t.Errorf("FindAttendee(%s) = %v, want the answer gone", id, err)
		}
		if accounts, err := e.h.Gathering.RecordAccounts(ctx, nil, id); err != nil || len(accounts) != 0 {
			t.Errorf("RecordAccounts(%s) = %v, %v; want none", id, accounts, err)
		}
		if rec := e.record(t, id); rec.Head.GoingCount != 0 || rec.Revision.Settings.Title == "" {
			t.Errorf("event %s = %+v, want it kept with nobody going", id, rec)
		}
	}
	if _, err := e.h.Gathering.FindInvitation(ctx, nil, ended.ID, carol); !errors.Is(err, appgathering.ErrNotFound) {
		t.Errorf("FindInvitation = %v, want the invitation gone", err)
	}
	for _, k := range []struct{ id, account string }{{reported.ID, bob}, {held.ID, dave}, {ahead.ID, bob}} {
		if _, err := e.h.Gathering.FindAttendee(ctx, nil, k.id, k.account); err != nil {
			t.Errorf("FindAttendee(%s) = %v, want the answer kept", k.id, err)
		}
	}
	if n, err := e.svc.PurgeRecords(ctx); err != nil || n != 0 {
		t.Errorf("PurgeRecords again = %d, %v; want none", n, err)
	}
	// The report is gone after its own retention, and the hold released.
	e.retention.mu.Lock()
	e.retention.reported, e.retention.held = map[string]bool{}, nil
	e.retention.mu.Unlock()
	if n, err := e.svc.PurgeRecords(ctx); err != nil || n != 2 {
		t.Errorf("PurgeRecords after the report and the hold = %d, %v; want both events", n, err)
	}
}
