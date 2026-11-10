// SPDX-License-Identifier: AGPL-3.0-only

package gatheringtest

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/domain/gathering"
)

// PollContract verifies the date polls of the gathering repository of a database adapter through
// appgathering.Service (ADR-0161 §7, §13, data-model §4.19): an event created with candidates is
// unscheduled, its head spans the candidates, and it lists among the upcoming events; only those
// who see the event answer its candidates, and they change their answers until a candidate is
// confirmed, which keeps the event visible to them; the organizers add and withdraw candidates
// but the last; confirming a candidate fixes the date, closes the poll, enqueues
// event_schedule_confirmed for the voters and the invitees, and the reminders follow the date; a
// date set on an unscheduled event confirms it the same way; and the heads rebuild unscheduled.
func PollContract(t *testing.T, newHarness Factory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *dueEnv)
	}{
		{"CreatesAnUnscheduledEvent", createsPoll},
		{"TakesAnswersFromTheViewers", votes},
		{"ChangesTheCandidates", changesCandidates},
		{"ConfirmsACandidate", confirms},
		{"ConfirmsADateWithoutACandidate", confirmsByUpdate},
		{"RebuildsAnUnscheduledHead", rebuildsPoll},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newDueEnv(t, newHarness))
		})
	}
}

// candidates returns n candidates of two hours, a day apart, from the day after the clock.
func (e *dueEnv) candidates(n int) []gathering.Schedule {
	first := e.schedule()
	out := make([]gathering.Schedule, n)
	for i := range out {
		start := first.StartsAt.Add(time.Duration(i) * 24 * time.Hour)
		out[i] = gathering.Schedule{StartsAt: start, EndsAt: start.Add(2 * time.Hour)}
	}
	return out
}

func (e *dueEnv) createPoll(t *testing.T, organizer string, s gathering.Settings, n int) appgathering.View {
	t.Helper()
	v, err := e.svc.Create(t.Context(), organizer, appgathering.CreateInput{Settings: s, Candidates: e.candidates(n)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return v
}

func (e *dueEnv) candidateList(t *testing.T, viewer, id string) []appgathering.CandidateTally {
	t.Helper()
	cs, err := e.svc.Candidates(t.Context(), viewer, id)
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	return cs
}

func (e *dueEnv) vote(t *testing.T, accountID, id, candidateID string, v gathering.Vote) appgathering.CandidateTally {
	t.Helper()
	c, err := e.svc.Vote(t.Context(), accountID, id, candidateID, v)
	if err != nil {
		t.Fatalf("Vote(%s): %v", v, err)
	}
	return c
}

func wantErr(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: %v, want %v", what, err, want)
	}
}

func wantSettingsError(t *testing.T, what string, err error, f gathering.Field) {
	t.Helper()
	var se *gathering.SettingsError
	if !errors.As(err, &se) || se.Field != f {
		t.Errorf("%s: %v, want a SettingsError of %s", what, err, f)
	}
}

func createsPoll(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	cands := e.candidates(3)
	v := e.createPoll(t, alice, settings("Meetup", gathering.VisibilityPublic), 3)
	if v.Head.State != gathering.StateUnscheduled || v.Scheduled() || v.Confirmation.ID != "" {
		t.Errorf("created: state %s, confirmation %+v, want unscheduled without one", v.Head.State, v.Confirmation)
	}
	if want := (gathering.Schedule{StartsAt: cands[0].StartsAt, EndsAt: cands[2].EndsAt}); !v.Head.Schedule.StartsAt.Equal(want.StartsAt) ||
		!v.Head.Schedule.EndsAt.Equal(want.EndsAt) {
		t.Errorf("head = %+v, want the span of the candidates %+v", v.Head.Schedule, want)
	}
	cs := e.candidateList(t, bob, v.ID)
	if len(cs) != 3 {
		t.Fatalf("Candidates = %+v, want 3", cs)
	}
	for i, c := range cs {
		if !c.Schedule.StartsAt.Equal(cands[i].StartsAt) || c.Yes+c.Maybe+c.No != 0 || c.ViewerVote != "" || c.Confirmed {
			t.Errorf("candidate %d = %+v, want %+v without answers", i, c, cands[i])
		}
	}
	wantDue(t, "unscheduled", e.due(t, v.ID), time.Time{})
	views, _, err := e.svc.List(ctx, bob, appgathering.ListQuery{Limit: 10})
	if err != nil || len(views) != 1 || views[0].ID != v.ID {
		t.Errorf("List = %d events, %v; want the unscheduled event", len(views), err)
	}
	_, err = e.svc.Answer(ctx, bob, v.ID, gathering.AnswerGoing)
	wantErr(t, "answer before a date", err, appgathering.ErrUnscheduled)

	both := appgathering.CreateInput{Settings: settings("Both", gathering.VisibilityPublic), Schedule: e.schedule(), Candidates: e.candidates(1)}
	_, err = e.svc.Create(ctx, alice, both)
	wantSettingsError(t, "a date and candidates", err, gathering.FieldCandidates)
	_, err = e.svc.Create(ctx, alice, appgathering.CreateInput{Settings: settings("None", gathering.VisibilityPublic), Candidates: []gathering.Schedule{}})
	wantSettingsError(t, "no candidates", err, gathering.FieldCandidates)
	twice := append(e.candidates(1), e.candidates(1)...)
	_, err = e.svc.Create(ctx, alice, appgathering.CreateInput{Settings: settings("Twice", gathering.VisibilityPublic), Candidates: twice})
	wantSettingsError(t, "the same candidate twice", err, gathering.FieldCandidates)
}

func votes(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	v := e.createPoll(t, alice, settings("Secret", gathering.VisibilityInvited), 2)
	cs := e.candidateList(t, alice, v.ID)
	first := cs[0].ID

	_, err := e.svc.Vote(ctx, bob, v.ID, first, gathering.VoteYes)
	wantErr(t, "a vote of someone who does not see the event", err, appgathering.ErrNotFound)
	if _, err := e.svc.Invite(ctx, alice, v.ID, bob); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if c := e.vote(t, bob, v.ID, first, gathering.VoteYes); c.Yes != 1 || c.ViewerVote != gathering.VoteYes {
		t.Errorf("bob yes = %+v", c)
	}
	if c := e.vote(t, bob, v.ID, first, gathering.VoteMaybe); c.Yes != 0 || c.Maybe != 1 || c.ViewerVote != gathering.VoteMaybe {
		t.Errorf("bob changes to maybe = %+v", c)
	}
	e.vote(t, alice, v.ID, first, gathering.VoteYes)
	_, err = e.svc.Vote(ctx, bob, v.ID, first, "going")
	wantErr(t, "an unknown vote", err, appgathering.ErrInvalidVote)
	_, err = e.svc.Vote(ctx, bob, v.ID, v.ID, gathering.VoteYes)
	wantErr(t, "an unknown candidate", err, appgathering.ErrNotFound)

	_, _, err = e.svc.Ballots(ctx, carol, v.ID, first, "", "", 10)
	wantErr(t, "the answers for someone who does not see the event", err, appgathering.ErrNotFound)
	ballots, more, err := e.svc.Ballots(ctx, bob, v.ID, first, "", "", 1)
	if err != nil || len(ballots) != 1 || !more {
		t.Errorf("Ballots page = %+v, %v, %v; want one and more", ballots, more, err)
	}
	ballots, _, err = e.svc.Ballots(ctx, bob, v.ID, first, gathering.VoteMaybe, "", 10)
	if err != nil || len(ballots) != 1 || ballots[0].AccountID != bob || ballots[0].Vote != gathering.VoteMaybe {
		t.Errorf("Ballots of maybe = %+v, %v; want bob", ballots, err)
	}

	// The answer keeps the event visible after the invitation goes.
	if err := e.svc.RevokeInvitation(ctx, alice, v.ID, bob); err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}
	if !e.sees(t, bob, v.ID) {
		t.Error("bob, who voted, no longer sees the event after the revocation")
	}
	audience, err := e.h.Gathering.PollAudience(ctx, nil, v.ID, "", 10)
	if want := sortedIDs(alice, bob); err != nil || !slices.Equal(audience, want) {
		t.Errorf("PollAudience = %v, %v; want the voters %v", audience, err, want)
	}
}

func sortedIDs(ids ...string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

func changesCandidates(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	v := e.createPoll(t, alice, settings("Meetup", gathering.VisibilityPublic), 1)
	if _, err := e.svc.AssignCoorganizer(ctx, alice, v.ID, carol); err != nil {
		t.Fatalf("AssignCoorganizer: %v", err)
	}
	extra := e.candidates(2)[1]
	_, err := e.svc.AddCandidate(ctx, bob, v.ID, extra)
	wantErr(t, "a candidate of someone who does not organize", err, appgathering.ErrNotOrganizer)
	added, err := e.svc.AddCandidate(ctx, carol, v.ID, extra)
	if err != nil {
		t.Fatalf("AddCandidate: %v", err)
	}
	_, err = e.svc.AddCandidate(ctx, alice, v.ID, extra)
	wantSettingsError(t, "the same candidate again", err, gathering.FieldCandidates)
	past := gathering.Schedule{StartsAt: e.clock.Now().Add(-time.Hour), EndsAt: e.clock.Now()}
	_, err = e.svc.AddCandidate(ctx, alice, v.ID, past)
	wantSettingsError(t, "a candidate in the past", err, gathering.FieldStartsAt)
	got, err := e.svc.Get(ctx, alice, v.ID)
	if err != nil || !got.Head.Schedule.EndsAt.Equal(extra.EndsAt) {
		t.Errorf("head after the addition = %+v, %v; want it to end with %v", got.Head.Schedule, err, extra.EndsAt)
	}

	first := e.candidateList(t, alice, v.ID)[0].ID
	e.vote(t, bob, v.ID, first, gathering.VoteYes)
	if err := e.svc.WithdrawCandidate(ctx, alice, v.ID, first); err != nil {
		t.Fatalf("WithdrawCandidate: %v", err)
	}
	cs := e.candidateList(t, alice, v.ID)
	if len(cs) != 1 || cs[0].ID != added.ID {
		t.Errorf("Candidates after the withdrawal = %+v, want only the added one", cs)
	}
	wantErr(t, "a withdrawn candidate again", e.svc.WithdrawCandidate(ctx, alice, v.ID, first), appgathering.ErrNotFound)
	wantErr(t, "the last candidate", e.svc.WithdrawCandidate(ctx, alice, v.ID, added.ID), appgathering.ErrLastCandidate)
	_, err = e.svc.Vote(ctx, bob, v.ID, first, gathering.VoteNo)
	wantErr(t, "a vote on a withdrawn candidate", err, appgathering.ErrNotFound)

	for i := len(e.candidateList(t, alice, v.ID)); i < gathering.MaxCandidates; i++ {
		start := extra.StartsAt.Add(time.Duration(i) * time.Hour * 7)
		if _, err := e.svc.AddCandidate(ctx, alice, v.ID, gathering.Schedule{StartsAt: start, EndsAt: start.Add(time.Hour)}); err != nil {
			t.Fatalf("AddCandidate %d: %v", i, err)
		}
	}
	more := extra.StartsAt.Add(300 * time.Hour)
	_, err = e.svc.AddCandidate(ctx, alice, v.ID, gathering.Schedule{StartsAt: more, EndsAt: more.Add(time.Hour)})
	wantSettingsError(t, "a candidate past the most", err, gathering.FieldCandidates)
}

func confirms(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	v := e.createPoll(t, alice, settings("Meetup", gathering.VisibilityPublic), 2)
	if _, err := e.svc.Invite(ctx, alice, v.ID, dave); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	cs := e.candidateList(t, alice, v.ID)
	chosen := cs[1]
	e.vote(t, bob, v.ID, chosen.ID, gathering.VoteYes)
	e.vote(t, carol, v.ID, cs[0].ID, gathering.VoteNo)
	e.outbox.Take()

	_, err := e.svc.Confirm(ctx, bob, v.ID, chosen.ID)
	wantErr(t, "a confirmation by someone who does not organize", err, appgathering.ErrNotOrganizer)
	got, err := e.svc.Confirm(ctx, alice, v.ID, chosen.ID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if got.Head.State != gathering.StateScheduled || !got.Scheduled() || !got.Confirmation.Schedule.StartsAt.Equal(chosen.Schedule.StartsAt) ||
		!got.Head.Schedule.EndsAt.Equal(chosen.Schedule.EndsAt) {
		t.Errorf("confirmed = state %s, confirmation %+v, head %+v; want the candidate %+v", got.Head.State, got.Confirmation, got.Head.Schedule, chosen.Schedule)
	}
	if jobs, want := e.notifyJobs(t), []string{"schedule_confirmed"}; !reflect.DeepEqual(jobs, want) {
		t.Errorf("jobs = %v, want %v", jobs, want)
	}
	audience, err := e.h.Gathering.PollAudience(ctx, nil, v.ID, "", 10)
	if want := sortedIDs(bob, carol, dave); err != nil || !slices.Equal(audience, want) {
		t.Errorf("PollAudience = %v, %v; want the voters and the invitee %v", audience, err, want)
	}
	marked := e.candidateList(t, bob, v.ID)
	if !marked[1].Confirmed || marked[0].Confirmed {
		t.Errorf("Candidates after the confirmation = %+v, want the second confirmed", marked)
	}

	// The reminders and the start follow the confirmed date.
	start := chosen.Schedule.StartsAt
	// The voice place of the settings adds the check of its room one day before.
	wantDue(t, "confirmed without attendees", e.due(t, v.ID), start.Add(-gathering.RoomCheckBefore))
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	wantDue(t, "bob going", e.due(t, v.ID), start.Add(-24*time.Hour))

	_, err = e.svc.Vote(ctx, carol, v.ID, chosen.ID, gathering.VoteYes)
	wantErr(t, "a vote after the confirmation", err, appgathering.ErrScheduled)
	_, err = e.svc.Confirm(ctx, alice, v.ID, cs[0].ID)
	wantErr(t, "a second confirmation", err, appgathering.ErrScheduled)
	_, err = e.svc.AddCandidate(ctx, alice, v.ID, e.candidates(4)[3])
	wantErr(t, "a candidate after the confirmation", err, appgathering.ErrScheduled)
	wantErr(t, "a withdrawal after the confirmation", e.svc.WithdrawCandidate(ctx, alice, v.ID, cs[0].ID), appgathering.ErrScheduled)

	// A started candidate is not confirmed.
	late := e.createPoll(t, alice, settings("Late", gathering.VisibilityPublic), 1)
	lateID := e.candidateList(t, alice, late.ID)[0].ID
	e.clock.Set(e.candidates(1)[0].StartsAt.Add(time.Minute))
	_, err = e.svc.Confirm(ctx, alice, late.ID, lateID)
	wantSettingsError(t, "a candidate that started", err, gathering.FieldStartsAt)
}

func confirmsByUpdate(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	v := e.createPoll(t, alice, settings("Meetup", gathering.VisibilityPublic), 2)
	e.vote(t, bob, v.ID, e.candidateList(t, alice, v.ID)[0].ID, gathering.VoteYes)
	e.outbox.Take()
	sc := e.candidates(5)[4]
	got, err := e.svc.Update(ctx, alice, v.ID, appgathering.Patch{Schedule: &sc})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Head.State != gathering.StateScheduled || !got.Confirmation.Schedule.StartsAt.Equal(sc.StartsAt) {
		t.Errorf("updated = state %s, confirmation %+v; want scheduled at %v", got.Head.State, got.Confirmation, sc.StartsAt)
	}
	if jobs, want := e.notifyJobs(t), []string{"schedule_confirmed"}; !reflect.DeepEqual(jobs, want) {
		t.Errorf("jobs = %v, want %v", jobs, want)
	}
	wantDue(t, "confirmed by the update", e.due(t, v.ID), sc.StartsAt.Add(-gathering.RoomCheckBefore))
	for _, c := range e.candidateList(t, alice, v.ID) {
		if c.Confirmed {
			t.Errorf("candidate %+v confirmed by a date without a candidate", c)
		}
	}
	// A later date is a change, not a confirmation.
	later := e.candidates(6)[5]
	if _, err := e.svc.Update(ctx, alice, v.ID, appgathering.Patch{Schedule: &later}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if jobs, want := e.notifyJobs(t), []string{"changed"}; !reflect.DeepEqual(jobs, want) {
		t.Errorf("jobs of the later date = %v, want %v", jobs, want)
	}
}

func rebuildsPoll(t *testing.T, e *dueEnv) {
	ctx := t.Context()
	v := e.createPoll(t, alice, settings("Meetup", gathering.VisibilityPublic), 2)
	cancelled := e.createPoll(t, alice, settings("Off", gathering.VisibilityPublic), 1)
	if _, err := e.svc.Cancel(ctx, alice, cancelled.ID, ""); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	before, err := e.svc.Get(ctx, alice, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := appgathering.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Gathering); err != nil {
		t.Fatalf("RebuildProjections: %v", err)
	}
	after, err := e.svc.Get(ctx, alice, v.ID)
	if err != nil || !reflect.DeepEqual(after.Head, before.Head) {
		t.Errorf("head after the rebuild = %+v, %v; want %+v", after.Head, err, before.Head)
	}
	off, err := e.svc.Get(ctx, alice, cancelled.ID)
	if err != nil || off.Head.State != gathering.StateCancelled || off.Scheduled() {
		t.Errorf("cancelled poll after the rebuild = %+v, %v", off.Head, err)
	}
}
