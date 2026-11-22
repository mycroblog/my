// SPDX-License-Identifier: AGPL-3.0-only

// Package gatheringtest holds the contract of the gathering module's repository (the events of
// the local users, ADR-0161), which every database adapter runs.
package gatheringtest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/pkg/testkit"
)

// Harness is what GatheringContract needs from a database adapter: its unit of work and the
// gathering repository.
type Harness struct {
	UnitOfWork uow.UnitOfWork
	Gathering  appgathering.Repository
}

// Factory returns a harness on a new, empty database whose UnitOfWork uses opts. The factory
// registers the cleanup of everything it creates with t.Cleanup.
type Factory func(t *testing.T, opts uow.Options) Harness

// GatheringContract verifies the gathering repository of a database adapter through the use cases
// of appgathering.Service (ADR-0161): an event reads back with its settings,
// places, date, and co-organizers; each visibility shows it to exactly the accounts it names, and
// a block hides it both ways; a physical or URL place reaches only the approved attendees unless
// the event shows it to every viewer; the organizer and the co-organizers are approved
// attendees whose approval cannot change; a place URL other than http or https is refused; a full
// event refuses new answers of going; a cancelled or started event takes no answers; the
// co-organizers are bounded and change what they may; the upcoming events list by the nearest
// start; the erasure empties the revisions of an account; and the projections rebuild from the
// events alone.
func GatheringContract(t *testing.T, newHarness Factory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *env)
	}{
		{"CreatesAndReadsAnEvent", createsAndReads},
		{"ShowsEachVisibilityToItsAccounts", visibilities},
		{"HidesTheEventAcrossBlocks", blocks},
		{"KeepsTheEventForTheInvitedAndTheAnswered", invitedAndAnswered},
		{"ShowsPlacesToTheApproved", places},
		{"NeverAsksTheOrganizersForApproval", organizersApproved},
		{"RefusesURLsOtherThanHTTP", urls},
		{"RefusesGoingToAFullEvent", capacity},
		{"ClosesCancelledAndStartedEvents", closed},
		{"BoundsTheCoorganizers", coorganizers},
		{"ChecksTheOrganizer", eligibility},
		{"ReschedulesAnEvent", reschedule},
		{"ListsTheUpcomingEvents", list},
		{"ErasesTheRevisionsOfAnAccount", erasure},
		{"RebuildsItsProjections", rebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, newEnv(t, newHarness))
		})
	}
}

// The accounts of the contract.
var (
	alice = testkit.NamedID("gathering-alice")
	bob   = testkit.NamedID("gathering-bob")
	carol = testkit.NamedID("gathering-carol")
	dave  = testkit.NamedID("gathering-dave")
	erin  = testkit.NamedID("gathering-erin")
	frank = testkit.NamedID("gathering-frank")
)

// members is the fake Members of the contract: every account is a local active user who may
// organize events and host rooms, but those in remote, suspended, limited, and noVoice.
type members struct {
	mu                                  sync.Mutex
	everyone                            bool
	remote, suspended, limited, noVoice map[string]bool
	permitted                           map[string]bool
}

func newMembers() *members {
	return &members{
		everyone: true, remote: map[string]bool{}, suspended: map[string]bool{}, limited: map[string]bool{},
		noVoice: map[string]bool{}, permitted: map[string]bool{},
	}
}

func (m *members) Member(_ context.Context, id string) (appgathering.Member, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return appgathering.Member{Local: !m.remote[id], Active: !m.suspended[id], Limited: m.limited[id]}, nil
}

func (m *members) Organizer(ctx context.Context, id string) (appgathering.Organizer, error) {
	mem, _ := m.Member(ctx, id)
	m.mu.Lock()
	defer m.mu.Unlock()
	return appgathering.Organizer{Member: mem, MayCreate: m.permitted[id], MayHostVoice: !m.noVoice[id], TimeZone: "Asia/Tokyo"}, nil
}

func (m *members) EveryoneCreates(context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.everyone, nil
}

// relations is the fake Relations of the contract.
type relations struct {
	mu      sync.Mutex
	follows map[[2]string]bool
	blocks  map[[2]string]bool
}

func newRelations() *relations {
	return &relations{follows: map[[2]string]bool{}, blocks: map[[2]string]bool{}}
}

func (r *relations) follow(follower, followee string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.follows[[2]string{follower, followee}] = true
}

func (r *relations) block(blocker, blocked string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blocks[[2]string{blocker, blocked}] = true
}

func (r *relations) BlockedAmong(_ context.Context, id string, ids []string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for _, o := range ids {
		out[o] = r.blocks[[2]string{id, o}] || r.blocks[[2]string{o, id}]
	}
	return out, nil
}

func (r *relations) Following(_ context.Context, id string, ids []string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for _, o := range ids {
		out[o] = r.follows[[2]string{id, o}]
	}
	return out, nil
}

func (r *relations) FollowersAmong(_ context.Context, id string, ids []string) (map[string]bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]bool{}
	for _, o := range ids {
		out[o] = r.follows[[2]string{o, id}]
	}
	return out, nil
}

type env struct {
	h         Harness
	clock     *testkit.Clock
	members   *members
	relations *relations
	svc       *appgathering.Service
}

func newEnv(t *testing.T, newHarness Factory) *env {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	appgathering.RegisterProjectors(projectors, h.Gathering)
	clock := testkit.NewClock(time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	e := &env{h: h, clock: clock, members: newMembers(), relations: newRelations()}
	e.svc = appgathering.NewService(appgathering.Deps{
		Repository: h.Gathering, UnitOfWork: h.UnitOfWork, Clock: clock, IDs: testkit.NewIDGenerator(clock),
		RateLimiter: kernel.NewRateLimiter(testkit.NewKeyValueStore(clock), clock), Members: e.members, Relations: e.relations,
	})
	return e
}

// start is when the events of the contract start: a day after the clock's start.
func (e *env) schedule() gathering.Schedule {
	start := e.clock.Now().Add(24 * time.Hour).Truncate(time.Minute)
	return gathering.Schedule{StartsAt: start, EndsAt: start.Add(2 * time.Hour)}
}

func settings(title string, vis gathering.Visibility) gathering.Settings {
	return gathering.Settings{
		Title: title, Description: "About " + title, Visibility: vis,
		Places: []gathering.Place{
			{Kind: gathering.PlacePhysical, Name: "Park", Address: "1-1 Chiyoda", URL: "https://maps.example/park"},
			{Kind: gathering.PlaceURL, Name: "Stream", URL: "https://stream.example/live"},
			{Kind: gathering.PlaceVoice},
		},
	}
}

func (e *env) create(t *testing.T, organizer string, s gathering.Settings) appgathering.View {
	t.Helper()
	v, err := e.svc.Create(t.Context(), organizer, appgathering.CreateInput{Settings: s, Schedule: e.schedule()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return v
}

// sees reports whether viewer sees the event id, failing on any error but ErrNotFound.
func (e *env) sees(t *testing.T, viewer, id string) bool {
	t.Helper()
	_, err := e.svc.Get(t.Context(), viewer, id)
	switch {
	case err == nil:
		return true
	case errors.Is(err, appgathering.ErrNotFound):
		return false
	}
	t.Fatalf("Get: %v", err)
	return false
}

func (e *env) answer(t *testing.T, accountID, id string, a gathering.Answer) appgathering.View {
	t.Helper()
	v, err := e.svc.Answer(t.Context(), accountID, id, a)
	if err != nil {
		t.Fatalf("Answer(%s): %v", a, err)
	}
	return v
}

func kinds(places []gathering.Place) []gathering.PlaceKind {
	out := []gathering.PlaceKind{}
	for _, p := range places {
		out = append(out, p.Kind)
	}
	return out
}

func createsAndReads(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Picnic", gathering.VisibilityPublic))
	got, err := e.svc.Get(t.Context(), alice, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := got.Revision.Settings
	if s.Title != "Picnic" || s.Description != "About Picnic" || s.TimeZone != "Asia/Tokyo" ||
		s.PlaceDisclosure != gathering.DisclosureApproved || got.Head.State != gathering.StateScheduled || got.Revisions != 1 {
		t.Errorf("Get = %+v", got)
	}
	// The organizer sees every place; the voice place takes the default speakers.
	if want := []gathering.PlaceKind{gathering.PlacePhysical, gathering.PlaceURL, gathering.PlaceVoice}; !slices.Equal(kinds(got.Places), want) {
		t.Errorf("places = %v, want %v", kinds(got.Places), want)
	}
	if got.Places[2].Speakers != gathering.SpeakersManual || got.Places[0].Address != "1-1 Chiyoda" {
		t.Errorf("places = %+v", got.Places)
	}
	if sc := e.schedule(); !got.Head.Schedule.StartsAt.Equal(sc.StartsAt) || !got.Head.Schedule.EndsAt.Equal(sc.EndsAt) {
		t.Errorf("schedule = %+v, want %+v", got.Head.Schedule, sc)
	}
	if !got.Viewer.Organizer || got.OrganizerAccountID != alice {
		t.Errorf("viewer = %+v", got.Viewer)
	}
	if _, err := e.svc.Get(t.Context(), alice, testkit.NamedID("gathering-unknown")); !errors.Is(err, appgathering.ErrNotFound) {
		t.Errorf("Get(unknown) = %v, want ErrNotFound", err)
	}
}

func visibilities(t *testing.T, e *env) {
	// bob follows alice, alice follows carol, alice and dave follow each other, erin has no relation.
	e.relations.follow(bob, alice)
	e.relations.follow(alice, carol)
	e.relations.follow(alice, dave)
	e.relations.follow(dave, alice)
	want := map[gathering.Visibility][]string{
		gathering.VisibilityPublic:    {alice, bob, carol, dave, erin},
		gathering.VisibilityFollowers: {alice, bob, dave},
		gathering.VisibilityFollowing: {alice, carol, dave},
		gathering.VisibilityMutuals:   {alice, dave},
		gathering.VisibilityInvited:   {alice},
	}
	for vis, viewers := range want {
		v := e.create(t, alice, settings(string(vis), vis))
		for _, id := range []string{alice, bob, carol, dave, erin} {
			if got := e.sees(t, id, v.ID); got != slices.Contains(viewers, id) {
				t.Errorf("%s: sees(%s) = %v", vis, id, got)
			}
		}
	}
	v := e.create(t, alice, settings("Remote", gathering.VisibilityPublic))
	e.members.remote[frank] = true
	if e.sees(t, frank, v.ID) || e.sees(t, "", v.ID) {
		t.Error("a remote or anonymous viewer sees a public event")
	}
}

func blocks(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Blocks", gathering.VisibilityPublic))
	if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, bob); err != nil {
		t.Fatal(err)
	}
	e.relations.block(alice, carol) // the organizer blocks
	e.relations.block(dave, bob)    // a viewer blocks a co-organizer
	for _, id := range []string{carol, dave} {
		if e.sees(t, id, v.ID) {
			t.Errorf("%s sees an event across a block", id)
		}
	}
	if !e.sees(t, erin, v.ID) {
		t.Error("erin does not see a public event")
	}
	// A blocked account is neither invited nor named a co-organizer.
	if _, err := e.svc.Invite(t.Context(), alice, v.ID, carol); !errors.Is(err, appgathering.ErrInvalidInvitee) {
		t.Errorf("Invite(blocked) = %v, want ErrInvalidInvitee", err)
	}
	if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, dave); !errors.Is(err, appgathering.ErrInvalidCoorganizer) {
		t.Errorf("AssignCoorganizer(blocked) = %v, want ErrInvalidCoorganizer", err)
	}
}

func invitedAndAnswered(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Invited", gathering.VisibilityInvited))
	if _, err := e.svc.Invite(t.Context(), alice, v.ID, bob); err != nil {
		t.Fatal(err)
	}
	if !e.sees(t, bob, v.ID) {
		t.Fatal("the invited does not see the event")
	}
	if _, err := e.svc.Invite(t.Context(), alice, v.ID, bob); !errors.Is(err, appgathering.ErrAlreadyInvited) {
		t.Errorf("Invite again = %v, want ErrAlreadyInvited", err)
	}
	invs, more, err := e.svc.Invitations(t.Context(), alice, v.ID, "", 10)
	if err != nil || more || len(invs) != 1 || invs[0].InviteeAccountID != bob {
		t.Errorf("Invitations = %+v, %v, %v", invs, more, err)
	}
	if _, _, err := e.svc.Invitations(t.Context(), bob, v.ID, "", 10); !errors.Is(err, appgathering.ErrNotOrganizer) {
		t.Errorf("Invitations by the invited = %v, want ErrNotOrganizer", err)
	}
	if err := e.svc.RevokeInvitation(t.Context(), alice, v.ID, bob); err != nil {
		t.Fatal(err)
	}
	if e.sees(t, bob, v.ID) {
		t.Error("a revoked invitee sees the event")
	}
	// An answer keeps the event visible when the visibility narrows.
	p := e.create(t, alice, settings("Narrowed", gathering.VisibilityPublic))
	e.answer(t, carol, p.ID, gathering.AnswerDeclined)
	narrow := gathering.VisibilityInvited
	if _, err := e.svc.Update(t.Context(), alice, p.ID, appgathering.Patch{Visibility: &narrow}); err != nil {
		t.Fatal(err)
	}
	if !e.sees(t, carol, p.ID) || e.sees(t, dave, p.ID) {
		t.Error("narrowing the visibility shut out the answered or kept the others")
	}
}

func places(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Places", gathering.VisibilityPublic))
	got, err := e.svc.Get(t.Context(), bob, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(kinds(got.Places), []gathering.PlaceKind{gathering.PlaceVoice}) ||
		!slices.Equal(got.HiddenPlaces, []gathering.PlaceKind{gathering.PlacePhysical, gathering.PlaceURL}) {
		t.Errorf("a viewer sees %v and misses %v", kinds(got.Places), got.HiddenPlaces)
	}
	if _, err := e.svc.Approve(t.Context(), alice, v.ID, bob); !errors.Is(err, appgathering.ErrNotAttending) {
		t.Errorf("Approve before an answer = %v, want ErrNotAttending", err)
	}
	e.answer(t, bob, v.ID, gathering.AnswerMaybe)
	if _, err := e.svc.Approve(t.Context(), bob, v.ID, bob); !errors.Is(err, appgathering.ErrNotOrganizer) {
		t.Errorf("Approve by the attendee = %v, want ErrNotOrganizer", err)
	}
	a, err := e.svc.Approve(t.Context(), alice, v.ID, bob)
	if err != nil || !a.Approved {
		t.Fatalf("Approve = %+v, %v", a, err)
	}
	got, _ = e.svc.Get(t.Context(), bob, v.ID)
	if len(got.Places) != 3 || len(got.HiddenPlaces) != 0 || !got.Viewer.Approved {
		t.Errorf("an approved attendee sees %v", kinds(got.Places))
	}
	// Declining ends the approval, and going again does not bring it back.
	e.answer(t, bob, v.ID, gathering.AnswerDeclined)
	got = e.answer(t, bob, v.ID, gathering.AnswerGoing)
	if len(got.Places) != 1 || got.Viewer.Approved {
		t.Errorf("after declining, the attendee sees %v (approved %v)", kinds(got.Places), got.Viewer.Approved)
	}
	if _, err := e.svc.Approve(t.Context(), alice, v.ID, bob); err != nil {
		t.Fatal(err)
	}
	if a, err := e.svc.RevokeApproval(t.Context(), alice, v.ID, bob); err != nil || a.Approved {
		t.Errorf("RevokeApproval = %+v, %v", a, err)
	}
	// Showing the places to every viewer.
	all := gathering.DisclosureViewers
	if _, err := e.svc.Update(t.Context(), alice, v.ID, appgathering.Patch{PlaceDisclosure: &all}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(t.Context(), carol, v.ID); len(got.Places) != 3 {
		t.Errorf("with the places shown to all, a viewer sees %v", kinds(got.Places))
	}
	as, more, err := e.svc.Attendees(t.Context(), carol, v.ID, "", 10)
	if err != nil || more || len(as) != 1 || as[0].AccountID != bob || as[0].Answer != gathering.AnswerGoing {
		t.Errorf("Attendees = %+v, %v, %v", as, more, err)
	}
}

// attendee is the attendee accountID of the Service's list of the event id as viewer, and whether
// the list has them.
func (e *env) attendee(t *testing.T, viewer, id, accountID string) (appgathering.Attendee, bool) {
	t.Helper()
	as, _, err := e.svc.Attendees(t.Context(), viewer, id, "", 10)
	if err != nil {
		t.Fatalf("Attendees: %v", err)
	}
	for _, a := range as {
		if a.AccountID == accountID {
			return a, true
		}
	}
	return appgathering.Attendee{}, false
}

// organizersApproved checks that the organizer and the co-organizers are never waiting for an
// approval (ADR-0161 §6): they are listed as organizing and approved, their event says they are
// approved, their approval is neither given nor revoked, and a change of the co-organizers shows
// at once, a removed one going back to the approval recorded of them.
func organizersApproved(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Organizers", gathering.VisibilityPublic))
	// bob is named after answering, carol before answering, dave never answers; erin attends.
	e.answer(t, alice, v.ID, gathering.AnswerGoing)
	e.answer(t, bob, v.ID, gathering.AnswerMaybe)
	e.answer(t, erin, v.ID, gathering.AnswerGoing)
	for _, id := range []string{bob, carol, dave} {
		if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	e.answer(t, carol, v.ID, gathering.AnswerGoing)

	rec, err := e.h.Gathering.FindGathering(t.Context(), nil, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{alice: true, bob: true, carol: true, dave: true, erin: false, frank: false} {
		if got := rec.Organizes(id); got != want {
			t.Errorf("Record.Organizes(%s) = %v, want %v", id, got, want)
		}
	}

	for _, id := range []string{alice, bob, carol} {
		a, ok := e.attendee(t, erin, v.ID, id)
		if !ok || !a.Organizes || !a.Approved {
			t.Errorf("the organizing attendee %s is listed as %+v (listed %v), want organizes and approved", id, a, ok)
		}
	}
	if a, ok := e.attendee(t, erin, v.ID, erin); !ok || a.Organizes || a.Approved {
		t.Errorf("the attendee erin is listed as %+v (listed %v), want neither organizing nor approved", a, ok)
	}
	if _, ok := e.attendee(t, erin, v.ID, dave); ok {
		t.Error("a co-organizer who never answered is listed among the attendees")
	}
	for _, id := range []string{alice, bob, carol, dave} {
		got, err := e.svc.Get(t.Context(), id, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Viewer.Approved || len(got.Places) != 3 {
			t.Errorf("the event as %s: viewer %+v and %d places, want approved and every place", id, got.Viewer, len(got.Places))
		}
	}

	// Neither the organizer nor a co-organizer approves or revokes one who organizes, even one who
	// never answered.
	for _, by := range []string{alice, bob} {
		for _, id := range []string{alice, bob, carol, dave} {
			if _, err := e.svc.Approve(t.Context(), by, v.ID, id); !errors.Is(err, appgathering.ErrAttendeeOrganizes) {
				t.Errorf("Approve(%s) by %s = %v, want ErrAttendeeOrganizes", id, by, err)
			}
			if _, err := e.svc.RevokeApproval(t.Context(), by, v.ID, id); !errors.Is(err, appgathering.ErrAttendeeOrganizes) {
				t.Errorf("RevokeApproval(%s) by %s = %v, want ErrAttendeeOrganizes", id, by, err)
			}
		}
	}

	// An ordinary attendee is approved and revoked as before.
	if a, err := e.svc.Approve(t.Context(), bob, v.ID, erin); err != nil || !a.Approved || a.Organizes {
		t.Errorf("Approve(erin) = %+v, %v", a, err)
	}
	if a, ok := e.attendee(t, alice, v.ID, erin); !ok || !a.Approved {
		t.Errorf("an approved attendee is listed as %+v (listed %v)", a, ok)
	}
	if a, err := e.svc.RevokeApproval(t.Context(), alice, v.ID, erin); err != nil || a.Approved {
		t.Errorf("RevokeApproval(erin) = %+v, %v", a, err)
	}
	if got, err := e.svc.Get(t.Context(), erin, v.ID); err != nil || got.Viewer.Approved || len(got.Places) != 1 {
		t.Errorf("the event as a revoked attendee: %+v, %d places, %v", got.Viewer, len(got.Places), err)
	}

	// An attendee approved before being named goes back to that approval once removed; one never
	// approved goes back to waiting.
	e.answer(t, frank, v.ID, gathering.AnswerGoing)
	if _, err := e.svc.RemoveCoorganizer(t.Context(), alice, v.ID, dave); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(t.Context(), alice, v.ID, frank); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, frank); err != nil {
		t.Fatal(err)
	}
	if a, ok := e.attendee(t, erin, v.ID, frank); !ok || !a.Organizes || !a.Approved {
		t.Errorf("an approved attendee named a co-organizer is listed as %+v (listed %v)", a, ok)
	}
	for _, id := range []string{frank, carol} {
		if _, err := e.svc.RemoveCoorganizer(t.Context(), alice, v.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if a, ok := e.attendee(t, erin, v.ID, frank); !ok || a.Organizes || !a.Approved {
		t.Errorf("a removed co-organizer approved before is listed as %+v (listed %v), want approved", a, ok)
	}
	if got, _ := e.svc.Get(t.Context(), frank, v.ID); !got.Viewer.Approved || got.Viewer.Coorganizer || len(got.Places) != 3 {
		t.Errorf("the event as a removed co-organizer approved before: %+v, %d places", got.Viewer, len(got.Places))
	}
	if a, ok := e.attendee(t, erin, v.ID, carol); !ok || a.Organizes || a.Approved {
		t.Errorf("a removed co-organizer never approved is listed as %+v (listed %v), want waiting", a, ok)
	}
	if got, _ := e.svc.Get(t.Context(), carol, v.ID); got.Viewer.Approved || got.Viewer.Coorganizer || len(got.Places) != 1 {
		t.Errorf("the event as a removed co-organizer never approved: %+v, %d places", got.Viewer, len(got.Places))
	}
	// Once removed, their approval is theirs to change again.
	if a, err := e.svc.Approve(t.Context(), alice, v.ID, carol); err != nil || !a.Approved || a.Organizes {
		t.Errorf("Approve(a removed co-organizer) = %+v, %v", a, err)
	}
}

func urls(t *testing.T, e *env) {
	for _, u := range []string{"javascript:alert(1)", "ftp://files.example/x", "data:text/html,hi", "//example.com/x", "file:///etc/passwd"} {
		s := settings("URL", gathering.VisibilityPublic)
		s.Places[1].URL = u
		_, err := e.svc.Create(t.Context(), alice, appgathering.CreateInput{Settings: s, Schedule: e.schedule()})
		var se *gathering.SettingsError
		if !errors.As(err, &se) || se.Field != gathering.FieldPlaces {
			t.Errorf("Create with the URL %q = %v, want a SettingsError of places", u, err)
		}
	}
}

func capacity(t *testing.T, e *env) {
	s := settings("Capacity", gathering.VisibilityPublic)
	s.Capacity = 1
	v := e.create(t, alice, s)
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	if _, err := e.svc.Answer(t.Context(), carol, v.ID, gathering.AnswerGoing); !errors.Is(err, appgathering.ErrFull) {
		t.Errorf("going to a full event = %v, want ErrFull", err)
	}
	// Maybe and declined are not refused, and going again keeps the place of who holds one.
	e.answer(t, carol, v.ID, gathering.AnswerMaybe)
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	got := e.answer(t, bob, v.ID, gathering.AnswerDeclined)
	if got.Head.GoingCount != 0 {
		t.Errorf("going count = %d, want 0", got.Head.GoingCount)
	}
	if got := e.answer(t, carol, v.ID, gathering.AnswerGoing); got.Head.GoingCount != 1 {
		t.Errorf("going count = %d, want 1", got.Head.GoingCount)
	}
	if _, err := e.svc.Answer(t.Context(), bob, v.ID, "attending"); !errors.Is(err, appgathering.ErrInvalidAnswer) {
		t.Errorf("Answer(attending) = %v, want ErrInvalidAnswer", err)
	}
}

func closed(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Closed", gathering.VisibilityPublic))
	if _, err := e.svc.Cancel(t.Context(), bob, v.ID, ""); !errors.Is(err, appgathering.ErrNotOrganizer) {
		t.Errorf("Cancel by a viewer = %v, want ErrNotOrganizer", err)
	}
	got, err := e.svc.Cancel(t.Context(), alice, v.ID, "Rain")
	if err != nil {
		t.Fatal(err)
	}
	if got.Head.State != gathering.StateCancelled || got.Cancellation == nil || got.Cancellation.Note != "Rain" ||
		got.Cancellation.Reason != gathering.CancelOrganizer {
		t.Errorf("Cancel = %+v", got)
	}
	if _, err := e.svc.Answer(t.Context(), bob, v.ID, gathering.AnswerGoing); !errors.Is(err, appgathering.ErrClosed) {
		t.Errorf("Answer after the cancellation = %v, want ErrClosed", err)
	}
	title := "Again"
	if _, err := e.svc.Update(t.Context(), alice, v.ID, appgathering.Patch{Title: &title}); !errors.Is(err, appgathering.ErrClosed) {
		t.Errorf("Update after the cancellation = %v, want ErrClosed", err)
	}
	if !e.sees(t, bob, v.ID) {
		t.Error("a cancelled event is not seen")
	}
	started := e.create(t, alice, settings("Started", gathering.VisibilityPublic))
	e.clock.Advance(25 * time.Hour)
	if _, err := e.svc.Answer(t.Context(), bob, started.ID, gathering.AnswerGoing); !errors.Is(err, appgathering.ErrClosed) {
		t.Errorf("Answer after the start = %v, want ErrClosed", err)
	}
}

func coorganizers(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Coorganizers", gathering.VisibilityPublic))
	for _, id := range []string{bob, carol, dave} {
		if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, erin); !errors.Is(err, appgathering.ErrTooManyCoorganizers) {
		t.Errorf("a fourth co-organizer = %v, want ErrTooManyCoorganizers", err)
	}
	if _, err := e.svc.AssignCoorganizer(t.Context(), bob, v.ID, erin); !errors.Is(err, appgathering.ErrOrganizerOnly) {
		t.Errorf("a co-organizer names one = %v, want ErrOrganizerOnly", err)
	}
	got, err := e.svc.Get(t.Context(), alice, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range got.Coorganizers {
		ids = append(ids, c.AccountID)
	}
	if !slices.Equal(ids, []string{bob, carol, dave}) {
		t.Errorf("co-organizers = %v, want them in the order of their nominations", ids)
	}
	// A co-organizer revises the event and sees every place, but does not change its visibility.
	title := "Renamed"
	if got, err := e.svc.Update(t.Context(), bob, v.ID, appgathering.Patch{Title: &title}); err != nil || got.Revision.Settings.Title != title ||
		len(got.Places) != 3 || !got.Viewer.Coorganizer {
		t.Errorf("Update by a co-organizer = %+v, %v", got, err)
	}
	vis := gathering.VisibilityInvited
	if _, err := e.svc.Update(t.Context(), bob, v.ID, appgathering.Patch{Visibility: &vis}); !errors.Is(err, appgathering.ErrOrganizerOnly) {
		t.Errorf("a co-organizer changes the visibility = %v, want ErrOrganizerOnly", err)
	}
	if _, err := e.svc.RemoveCoorganizer(t.Context(), alice, v.ID, carol); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(t.Context(), carol, v.ID); got.Viewer.Coorganizer || len(got.Places) != 1 {
		t.Errorf("a removed co-organizer = %+v", got.Viewer)
	}
	if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, erin); err != nil {
		t.Errorf("a co-organizer after a removal = %v", err)
	}
}

func eligibility(t *testing.T, e *env) {
	e.members.everyone = false
	if _, err := e.svc.Create(t.Context(), alice, appgathering.CreateInput{Settings: settings("No", gathering.VisibilityPublic), Schedule: e.schedule()}); !errors.Is(err, appgathering.ErrNotEligible) {
		t.Errorf("Create without create_events = %v, want ErrNotEligible", err)
	}
	e.members.permitted[alice] = true
	e.create(t, alice, settings("Permitted", gathering.VisibilityPublic))
	e.members.everyone = true
	e.members.limited[bob] = true
	if _, err := e.svc.Create(t.Context(), bob, appgathering.CreateInput{Settings: settings("Limited", gathering.VisibilityPublic), Schedule: e.schedule()}); !errors.Is(err, appgathering.ErrNotEligible) {
		t.Errorf("Create by a limited account = %v, want ErrNotEligible", err)
	}
	e.members.noVoice[carol] = true
	if _, err := e.svc.Create(t.Context(), carol, appgathering.CreateInput{Settings: settings("Voice", gathering.VisibilityPublic), Schedule: e.schedule()}); !errors.Is(err, appgathering.ErrVoiceUnavailable) {
		t.Errorf("Create with a room without hosting = %v, want ErrVoiceUnavailable", err)
	}
	s := settings("Offline", gathering.VisibilityPublic)
	s.Places = s.Places[:2]
	e.create(t, carol, s)
	past := gathering.Schedule{StartsAt: e.clock.Now().Add(-time.Hour), EndsAt: e.clock.Now().Add(time.Hour)}
	var se *gathering.SettingsError
	if _, err := e.svc.Create(t.Context(), dave, appgathering.CreateInput{Settings: s, Schedule: past}); !errors.As(err, &se) || se.Field != gathering.FieldStartsAt {
		t.Errorf("Create in the past = %v, want a SettingsError of starts_at", err)
	}
}

func reschedule(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Reschedule", gathering.VisibilityPublic))
	sc := e.schedule()
	next := gathering.Schedule{StartsAt: sc.StartsAt.Add(48 * time.Hour), EndsAt: sc.EndsAt.Add(48 * time.Hour)}
	got, err := e.svc.Update(t.Context(), alice, v.ID, appgathering.Patch{Schedule: &next})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Head.Schedule.StartsAt.Equal(next.StartsAt) || got.Confirmation.ID == v.Confirmation.ID || got.Revisions != 1 {
		t.Errorf("Update(schedule) = %+v", got.Head)
	}
	long := gathering.Schedule{StartsAt: next.StartsAt, EndsAt: next.StartsAt.Add(8 * 24 * time.Hour)}
	var se *gathering.SettingsError
	if _, err := e.svc.Update(t.Context(), alice, v.ID, appgathering.Patch{Schedule: &long}); !errors.As(err, &se) || se.Field != gathering.FieldEndsAt {
		t.Errorf("Update(eight days) = %v, want a SettingsError of ends_at", err)
	}
}

func list(t *testing.T, e *env) {
	sc := e.schedule()
	var want []string
	for i, vis := range []gathering.Visibility{gathering.VisibilityPublic, gathering.VisibilityInvited, gathering.VisibilityPublic, gathering.VisibilityPublic} {
		at := gathering.Schedule{StartsAt: sc.StartsAt.Add(time.Duration(3-i) * time.Hour), EndsAt: sc.EndsAt.Add(time.Duration(3-i) * time.Hour)}
		v, err := e.svc.Create(t.Context(), alice, appgathering.CreateInput{Settings: settings("List", vis), Schedule: at})
		if err != nil {
			t.Fatal(err)
		}
		if vis == gathering.VisibilityPublic {
			want = append([]string{v.ID}, want...)
		}
	}
	var got []string
	var after *appgathering.ListKey
	for range 5 {
		page, next, err := e.svc.List(t.Context(), bob, appgathering.ListQuery{After: after, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range page {
			got = append(got, v.ID)
		}
		if next == nil {
			break
		}
		after = next
	}
	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v (by the nearest start, without the invited-only event)", got, want)
	}
	// An event that ended leaves the list.
	e.clock.Advance(30 * 24 * time.Hour)
	if page, _, err := e.svc.List(t.Context(), bob, appgathering.ListQuery{Limit: 10}); err != nil || len(page) != 0 {
		t.Errorf("List after the events = %d events, %v", len(page), err)
	}
}

func erasure(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Erased", gathering.VisibilityPublic))
	eraser := appgathering.NewEraser(e.h.UnitOfWork, e.h.Gathering)
	for range 3 {
		step, err := eraser.EraseAccountData(t.Context(), account.ErasureTarget{AccountID: alice}, "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if step.Done {
			break
		}
	}
	got, err := e.svc.Get(t.Context(), alice, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s := got.Revision.Settings; s.Title != "" || s.Description != "" || len(got.Places) != 0 {
		t.Errorf("after the erasure the revision holds %+v", got.Revision.Settings)
	}
}

func rebuild(t *testing.T, e *env) {
	v := e.create(t, alice, settings("Rebuild", gathering.VisibilityPublic))
	if _, err := e.svc.AssignCoorganizer(t.Context(), alice, v.ID, carol); err != nil {
		t.Fatal(err)
	}
	e.answer(t, bob, v.ID, gathering.AnswerGoing)
	if _, err := e.svc.Approve(t.Context(), alice, v.ID, bob); err != nil {
		t.Fatal(err)
	}
	e.answer(t, dave, v.ID, gathering.AnswerMaybe)
	read := func() (appgathering.Record, []appgathering.Attendee) {
		rec, err := e.h.Gathering.FindGathering(t.Context(), nil, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		as, err := e.h.Gathering.Attendees(t.Context(), nil, v.ID, "", 10)
		if err != nil {
			t.Fatal(err)
		}
		return rec, as
	}
	rec, as := read()
	if err := appgathering.RebuildProjections(t.Context(), e.h.UnitOfWork, e.h.Gathering); err != nil {
		t.Fatal(err)
	}
	rec2, as2 := read()
	if !reflect.DeepEqual(rec, rec2) || !reflect.DeepEqual(as, as2) {
		t.Errorf("the rebuild changed the projections:\n %+v\n %+v\n %+v\n %+v", rec, rec2, as, as2)
	}
	if rec.Head.GoingCount != 1 || len(as) != 2 || !as[0].Approved || as[1].Approved {
		t.Errorf("projections = %+v, %+v", rec.Head, as)
	}
}
