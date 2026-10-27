// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/graph"
)

// FollowHarness is one database under test: an empty database with the product schema, its
// UnitOfWork, and the identity, account, and graph repositories on it.
type FollowHarness struct {
	AccountHarness
	Graph appgraph.Repository
}

// FollowFactory returns a harness on a new, empty database whose UnitOfWork uses opts.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type FollowFactory func(t *testing.T, opts uow.Options) FollowHarness

// FollowRepositoryContract verifies the graph repository of a database adapter through the
// follow use cases between local accounts: following an unlocked account at once, a locked
// account through a request that stays pending until accepted, rejecting and withdrawing
// requests, unfollowing and removing followers, following again as a new follow that keeps the
// earlier one on record, follow preferences, paging, rejecting invalid follows, recording each
// transition once, concurrent follows of one pair, the emitted domain events, and the property
// that follow_edges and follow_counters equal a rebuild from the follows and their events.
func FollowRepositoryContract(t *testing.T, newHarness FollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *followEnv)
	}{
		{"FollowsAnUnlockedAccountAtOnce", followUnlocked},
		{"KeepsARequestToALockedAccountPendingUntilAccepted", followLocked},
		{"RejectsARequest", followRejects},
		{"WithdrawsARequest", followWithdraws},
		{"UnfollowsAndRemovesFollowers", followUnfollows},
		{"FollowsAgainAsANewFollow", followAgain},
		{"UpdatesPreferences", followPreferences},
		{"FollowsWithPreferencesAtomically", followWithPreferences},
		{"PagesFollowersAndFollowing", followPages},
		{"RejectsInvalidFollows", followRejectsInvalid},
		{"RecordsEachTransitionOnce", followRecordsOnce},
		{"AgreesOnConcurrentFollows", followConcurrent},
		{"SettlesAPendingRequestOnce", followSettlesOnce},
		{"EmitsDomainEvents", followEmitsEvents},
		{"IncrementalProjectionEqualsRebuild", followProjectionEqualsRebuild},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newFollowEnv(t, newHarness)) })
	}
}

// followEnv wires the account and graph use cases onto one harness.
type followEnv struct {
	*accountEnv
	repo   appgraph.Repository
	graph  *appgraph.Service
	events *eventLog
	// captured are the harness's projectors, for registering more event types to log.
	captured *uow.Projectors
}

// eventLog records the graph events that commands emit (committed or not).
type eventLog struct {
	mu     sync.Mutex
	events []uow.Event
}

// record is a projector that logs e.
func (l *eventLog) record(_ context.Context, _ uow.Tx, e uow.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, e)
	return nil
}

func (l *eventLog) take() []uow.Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.events
	l.events = nil
	return out
}

func newFollowEnv(t *testing.T, newHarness FollowFactory) *followEnv {
	t.Helper()
	var (
		fh       FollowHarness
		captured uow.Options
	)
	ae := newAccountEnv(t, func(t *testing.T, opts uow.Options) AccountHarness {
		captured = opts
		fh = newHarness(t, opts)
		return fh.AccountHarness
	})
	appgraph.RegisterProjectors(captured.Projectors, fh.Graph)
	log := &eventLog{}
	for _, typ := range []string{
		appgraph.EventFollowed, appgraph.EventFollowRequested, appgraph.EventFollowAccepted,
		appgraph.EventFollowRejected, appgraph.EventFollowTerminated, appgraph.EventFollowPreferencesChanged,
	} {
		captured.Projectors.Register(typ, log.record)
	}
	return &followEnv{
		accountEnv: ae,
		repo:       fh.Graph,
		events:     log,
		captured:   captured.Projectors,
		graph: appgraph.NewService(appgraph.Deps{
			UnitOfWork: fh.UnitOfWork,
			Repository: fh.Graph,
			Accounts:   ae.accounts,
			Clock:      ae.clock,
			IDs:        ae.ids,
			Host:       contractHost,
		}),
	}
}

// user registers a local user and returns its account ID; locked users get a locked profile.
func (e *followEnv) user(t *testing.T, username string, locked bool) string {
	t.Helper()
	id := e.register(t, username).AccountID
	if locked {
		if _, err := e.accounts.ReviseProfile(t.Context(), id, appaccount.ProfileInput{DisplayName: username, Locked: true}); err != nil {
			t.Fatalf("ReviseProfile(%s, locked) = %v", username, err)
		}
	}
	return id
}

func (e *followEnv) follow(t *testing.T, follower, followee string) appgraph.Relationship {
	t.Helper()
	e.clock.Advance(time.Second)
	rel, err := e.graph.Follow(t.Context(), follower, followee)
	if err != nil {
		t.Fatalf("Follow(%s, %s) = %v", follower, followee, err)
	}
	return rel
}

func checkRelationship(t *testing.T, e *followEnv, account, target string, following, followedBy graph.State) {
	t.Helper()
	rel, err := e.graph.Relationship(t.Context(), account, target)
	if err != nil {
		t.Fatalf("Relationship = %v", err)
	}
	if rel.Following != following || rel.FollowedBy != followedBy {
		t.Errorf("Relationship(%s, %s) = following %s, followed by %s; want %s, %s",
			account, target, rel.Following, rel.FollowedBy, following, followedBy)
	}
}

func checkCounters(t *testing.T, e *followEnv, accountID string, followers, following int64) {
	t.Helper()
	c, err := e.graph.Counters(t.Context(), accountID)
	if err != nil {
		t.Fatalf("Counters = %v", err)
	}
	if c != (graph.Counters{AccountID: accountID, FollowersCount: followers, FollowingCount: following}) {
		t.Errorf("Counters(%s) = %+v, want %d followers and %d following", accountID, c, followers, following)
	}
}

// histories returns the follows from follower to followee with their events, oldest first.
func (e *followEnv) histories(t *testing.T, follower, followee string) []graph.History {
	t.Helper()
	var out []graph.History
	err := e.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		out = nil
		all, err := e.repo.ListHistories(t.Context(), tx)
		for _, h := range all {
			if h.Follow.FollowerAccountID == follower && h.Follow.FolloweeAccountID == followee {
				out = append(out, h)
			}
		}
		return err
	})
	if err != nil {
		t.Fatalf("ListHistories = %v", err)
	}
	return out
}

func followUnlocked(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	rel := e.follow(t, alice, bob)
	if rel.Following != graph.StateActive || rel.FollowedBy != graph.StateNone || !rel.ShowBoosts || rel.Notify {
		t.Errorf("Follow = %+v, want an active follow with the default preferences", rel)
	}
	checkRelationship(t, e, bob, alice, graph.StateNone, graph.StateActive)
	checkCounters(t, e, alice, 0, 1)
	checkCounters(t, e, bob, 1, 0)

	hs := e.histories(t, alice, bob)
	if len(hs) != 1 {
		t.Fatalf("follows = %+v, want one", hs)
	}
	f := hs[0].Follow
	if f.URI != graph.FollowURI(contractHost, f.ID) || f.URI != "https://"+contractHost+"/ap/activities/"+f.ID {
		t.Errorf("follow URI = %q, want /ap/activities/{id}", f.URI)
	}
	if a := hs[0].Acceptance; a == nil || a.AcceptedBy != graph.AcceptedByAuto || !a.AcceptedAt.Equal(f.RequestedAt) {
		t.Errorf("acceptance = %+v, want an automatic acceptance at the request", a)
	}

	// Following again changes nothing.
	e.follow(t, alice, bob)
	if hs := e.histories(t, alice, bob); len(hs) != 1 {
		t.Errorf("follows after following again = %d, want 1", len(hs))
	}
	checkCounters(t, e, bob, 1, 0)

	followers, err := e.graph.Followers(ctx, bob, appgraph.Page{})
	if err != nil || len(followers) != 1 || followers[0].FollowerAccountID != alice || followers[0].FollowID != f.ID {
		t.Errorf("Followers(bob) = %+v, %v; want alice", followers, err)
	}
	following, err := e.graph.Following(ctx, alice, appgraph.Page{})
	if err != nil || len(following) != 1 || following[0].FolloweeAccountID != bob {
		t.Errorf("Following(alice) = %+v, %v; want bob", following, err)
	}
	if reqs, err := e.graph.FollowRequests(ctx, bob, appgraph.Page{}); err != nil || len(reqs) != 0 {
		t.Errorf("FollowRequests(bob) = %+v, %v; want none", reqs, err)
	}
}

func followLocked(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", true)
	rel := e.follow(t, alice, bob)
	if rel.Following != graph.StatePending {
		t.Fatalf("Follow(locked) = %+v, want pending", rel)
	}
	checkRelationship(t, e, bob, alice, graph.StateNone, graph.StatePending)
	checkCounters(t, e, alice, 0, 0)
	checkCounters(t, e, bob, 0, 0)
	if fs, _ := e.graph.Followers(ctx, bob, appgraph.Page{}); len(fs) != 0 {
		t.Errorf("Followers(bob) = %+v while pending, want none", fs)
	}
	reqs, err := e.graph.FollowRequests(ctx, bob, appgraph.Page{})
	if err != nil || len(reqs) != 1 || reqs[0].FollowerAccountID != alice || reqs[0].State != graph.StatePending {
		t.Fatalf("FollowRequests(bob) = %+v, %v; want alice", reqs, err)
	}

	e.clock.Advance(time.Minute)
	rel, err = e.graph.AcceptFollowRequest(ctx, bob, alice)
	if err != nil || rel.FollowedBy != graph.StateActive {
		t.Fatalf("AcceptFollowRequest = %+v, %v; want followed by alice", rel, err)
	}
	checkRelationship(t, e, alice, bob, graph.StateActive, graph.StateNone)
	checkCounters(t, e, alice, 0, 1)
	checkCounters(t, e, bob, 1, 0)
	hs := e.histories(t, alice, bob)
	if len(hs) != 1 || hs[0].Acceptance == nil || hs[0].Acceptance.AcceptedBy != graph.AcceptedByFollowee {
		t.Fatalf("history = %+v, want one follow accepted by the followee", hs)
	}
	edge, err := e.repo.FindEdge(ctx, alice, bob)
	if err != nil || !edge.Since.Equal(hs[0].Acceptance.AcceptedAt) || !edge.Since.After(hs[0].Follow.RequestedAt) {
		t.Errorf("edge = %+v, %v; want active since the acceptance", edge, err)
	}

	// Accepting again changes nothing; a missing request is not found.
	if _, err := e.graph.AcceptFollowRequest(ctx, bob, alice); err != nil {
		t.Errorf("AcceptFollowRequest again = %v, want no-op", err)
	}
	checkCounters(t, e, bob, 1, 0)
	if _, err := e.graph.AcceptFollowRequest(ctx, alice, bob); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("AcceptFollowRequest without a request = %v, want ErrNotFound", err)
	}
}

func followRejects(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", true)
	e.follow(t, alice, bob)
	rel, err := e.graph.RejectFollowRequest(ctx, bob, alice)
	if err != nil || rel.FollowedBy != graph.StateNone {
		t.Fatalf("RejectFollowRequest = %+v, %v; want no relationship", rel, err)
	}
	checkRelationship(t, e, alice, bob, graph.StateNone, graph.StateNone)
	if _, err := e.graph.RejectFollowRequest(ctx, bob, alice); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("RejectFollowRequest again = %v, want ErrNotFound", err)
	}
	hs := e.histories(t, alice, bob)
	if len(hs) != 1 || hs[0].Rejection == nil || hs[0].Rejection.RejectedBy != graph.RejectedByFollowee || hs[0].Acceptance != nil {
		t.Errorf("history = %+v, want one follow rejected by the followee", hs)
	}

	// An active follow cannot be rejected, only removed.
	carol := e.user(t, "carol", false)
	e.follow(t, carol, alice)
	if _, err := e.graph.RejectFollowRequest(ctx, alice, carol); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("RejectFollowRequest(active) = %v, want ErrNotFound", err)
	}
	checkRelationship(t, e, carol, alice, graph.StateActive, graph.StateNone)
}

func followWithdraws(t *testing.T, e *followEnv) {
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", true)
	e.follow(t, alice, bob)
	rel, err := e.graph.Unfollow(t.Context(), alice, bob)
	if err != nil || rel.Following != graph.StateNone {
		t.Fatalf("Unfollow(pending) = %+v, %v; want no relationship", rel, err)
	}
	hs := e.histories(t, alice, bob)
	if len(hs) != 1 || hs[0].Termination == nil || hs[0].Termination.Reason != graph.TerminatedUnfollowed {
		t.Errorf("history = %+v, want one follow terminated as unfollowed", hs)
	}
	checkCounters(t, e, alice, 0, 0)
	checkCounters(t, e, bob, 0, 0)
	if reqs, _ := e.graph.FollowRequests(t.Context(), bob, appgraph.Page{}); len(reqs) != 0 {
		t.Errorf("FollowRequests(bob) = %+v after withdrawal, want none", reqs)
	}
}

func followUnfollows(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice", false), e.user(t, "bob", false), e.user(t, "carol", false)
	e.follow(t, alice, bob)
	e.follow(t, carol, bob)
	e.follow(t, bob, alice)
	checkCounters(t, e, bob, 2, 1)

	if _, err := e.graph.Unfollow(ctx, alice, bob); err != nil {
		t.Fatalf("Unfollow = %v", err)
	}
	checkRelationship(t, e, alice, bob, graph.StateNone, graph.StateActive)
	checkCounters(t, e, alice, 1, 0)
	checkCounters(t, e, bob, 1, 1)
	if _, err := e.graph.Unfollow(ctx, alice, bob); err != nil {
		t.Errorf("Unfollow again = %v, want no-op", err)
	}

	rel, err := e.graph.RemoveFollower(ctx, bob, carol)
	if err != nil || rel.FollowedBy != graph.StateNone {
		t.Fatalf("RemoveFollower = %+v, %v; want carol removed", rel, err)
	}
	if hs := e.histories(t, carol, bob); len(hs) != 1 || hs[0].Termination == nil || hs[0].Termination.Reason != graph.TerminatedRemovedByFollowee {
		t.Errorf("history = %+v, want one follow removed by the followee", hs)
	}
	checkCounters(t, e, bob, 0, 1)
	checkCounters(t, e, carol, 0, 0)
	if _, err := e.graph.RemoveFollower(ctx, bob, carol); err != nil {
		t.Errorf("RemoveFollower again = %v, want no-op", err)
	}
}

func followAgain(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", true)
	e.follow(t, alice, bob)
	if _, err := e.graph.RejectFollowRequest(ctx, bob, alice); err != nil {
		t.Fatalf("RejectFollowRequest = %v", err)
	}
	e.follow(t, alice, bob)
	if _, err := e.graph.AcceptFollowRequest(ctx, bob, alice); err != nil {
		t.Fatalf("AcceptFollowRequest = %v", err)
	}
	if _, err := e.graph.Unfollow(ctx, alice, bob); err != nil {
		t.Fatalf("Unfollow = %v", err)
	}
	rel := e.follow(t, alice, bob)
	if rel.Following != graph.StatePending {
		t.Fatalf("Follow again = %+v, want pending", rel)
	}

	hs := e.histories(t, alice, bob)
	if len(hs) != 3 {
		t.Fatalf("follows = %d, want 3 (each follow is a new row)", len(hs))
	}
	if hs[0].Rejection == nil || hs[1].Acceptance == nil || hs[1].Termination == nil ||
		hs[2].Acceptance != nil || hs[2].Rejection != nil || hs[2].Termination != nil {
		t.Errorf("histories = %+v, want rejected, accepted then terminated, and pending", hs)
	}
	uris := map[string]bool{}
	for _, h := range hs {
		uris[h.Follow.URI] = true
	}
	if len(uris) != 3 {
		t.Errorf("follow URIs = %v, want three distinct", uris)
	}
	edge, err := e.repo.FindEdge(ctx, alice, bob)
	if err != nil || edge.FollowID != hs[2].Follow.ID || edge.State != graph.StatePending {
		t.Errorf("edge = %+v, %v; want the newest follow, pending", edge, err)
	}
	checkCounters(t, e, bob, 0, 0)
}

func followPreferences(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	if _, err := e.graph.UpdatePreferences(ctx, alice, bob, appgraph.PreferencesInput{}); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("UpdatePreferences without a follow = %v, want ErrNotFound", err)
	}
	e.follow(t, alice, bob)
	rel, err := e.graph.UpdatePreferences(ctx, alice, bob, appgraph.PreferencesInput{ShowBoosts: false, Notify: true})
	if err != nil || rel.ShowBoosts || !rel.Notify {
		t.Fatalf("UpdatePreferences = %+v, %v; want boosts hidden and notifications on", rel, err)
	}
	e.follow(t, alice, bob)
	if rel, _ := e.graph.Relationship(ctx, alice, bob); rel.ShowBoosts || !rel.Notify {
		t.Errorf("Relationship after following again = %+v, want the preferences kept", rel)
	}
	if _, err := e.graph.Unfollow(ctx, alice, bob); err != nil {
		t.Fatalf("Unfollow = %v", err)
	}
	rel = e.follow(t, alice, bob)
	if !rel.ShowBoosts || rel.Notify {
		t.Errorf("new follow = %+v, want the default preferences", rel)
	}
}

func followPages(t *testing.T, e *followEnv) {
	ctx := t.Context()
	star := e.user(t, "star", false)
	var fans []string
	for i := range 5 {
		fan := e.user(t, fmt.Sprintf("fan%d", i), false)
		fans = append(fans, fan)
		e.follow(t, fan, star)
		e.follow(t, star, fan)
	}
	slices.Reverse(fans) // newest follow first
	for name, list := range map[string]func(context.Context, string, appgraph.Page) ([]graph.Edge, error){
		"Followers": e.graph.Followers, "Following": e.graph.Following,
	} {
		var got []string
		before := ""
		for range 3 {
			page, err := list(ctx, star, appgraph.Page{Before: before, Limit: 2})
			if err != nil {
				t.Fatalf("%s = %v", name, err)
			}
			if len(page) > 2 {
				t.Fatalf("%s page = %d edges, want at most 2", name, len(page))
			}
			for _, edge := range page {
				other := edge.FollowerAccountID
				if name == "Following" {
					other = edge.FolloweeAccountID
				}
				got = append(got, other)
				before = edge.FollowID
			}
		}
		if !slices.Equal(got, fans) {
			t.Errorf("%s pages = %v, want %v", name, got, fans)
		}
	}

	// The batches that jobs walk all followers in end with an empty cursor.
	var got []string
	before := ""
	for range 4 {
		ids, next, err := e.graph.ActiveFollowerIDs(ctx, star, before, 2)
		if err != nil {
			t.Fatalf("ActiveFollowerIDs = %v", err)
		}
		got = append(got, ids...)
		if before = next; next == "" {
			break
		}
	}
	if !slices.Equal(got, fans) || before != "" {
		t.Errorf("ActiveFollowerIDs batches = %v (cursor %q), want %v", got, before, fans)
	}
}

func followRejectsInvalid(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	if _, err := e.graph.Follow(ctx, alice, alice); !errors.Is(err, graph.ErrSelfFollow) {
		t.Errorf("Follow(self) = %v, want ErrSelfFollow", err)
	}
	missing := "00000000-0000-7000-8000-000000000000"
	if _, err := e.graph.Follow(ctx, alice, missing); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Follow(missing) = %v, want ErrAccountUnavailable", err)
	}
	if _, err := e.graph.Follow(ctx, missing, alice); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Follow(from missing) = %v, want ErrAccountUnavailable", err)
	}
	_, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: bob, Kind: account.LifecycleDeactivated, InitiatedBy: account.InitiatedBySelf,
	})
	if err != nil {
		t.Fatalf("RecordLifecycleEvent = %v", err)
	}
	if _, err := e.graph.Follow(ctx, alice, bob); !errors.Is(err, appgraph.ErrAccountUnavailable) {
		t.Errorf("Follow(deactivated) = %v, want ErrAccountUnavailable", err)
	}
	// A follower whose moderation restrictions forbid interacting may not follow.
	carol := e.user(t, "carol", false)
	restricted := appgraph.NewService(appgraph.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.repo, Accounts: e.accounts, Clock: e.clock, IDs: e.ids, Host: contractHost,
		Interactions: interactionPolicy{alice: true},
	})
	if _, err := restricted.Follow(ctx, alice, carol); !errors.Is(err, appgraph.ErrInteractionRestricted) {
		t.Errorf("Follow under the interact restriction = %v, want ErrInteractionRestricted", err)
	}
	if edges, _ := e.repo.ListEdges(ctx); len(edges) != 0 {
		t.Errorf("edges = %+v, want none", edges)
	}
	if _, err := restricted.Follow(ctx, carol, alice); err != nil {
		t.Errorf("Follow by an account without restrictions = %v", err)
	}
}

// interactionPolicy is a graph.InteractionPolicy that forbids the accounts in it to interact.
type interactionPolicy map[string]bool

func (p interactionPolicy) InteractionRestricted(_ context.Context, accountID string) (bool, error) {
	return p[accountID], nil
}

func followRecordsOnce(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	e.follow(t, alice, bob)
	f := e.histories(t, alice, bob)[0].Follow
	now := e.clock.Now()
	for name, insert := range map[string]func(tx uow.Tx) error{
		"acceptance": func(tx uow.Tx) error {
			return e.repo.InsertAcceptance(ctx, tx, graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByFollowee, AcceptedAt: now})
		},
		"rejection": func(tx uow.Tx) error {
			return e.repo.InsertRejection(ctx, tx, graph.Rejection{FollowID: f.ID, RejectedBy: graph.RejectedBySystem, RejectedAt: now})
		},
		"termination": func(tx uow.Tx) error {
			return e.repo.InsertTermination(ctx, tx, graph.Termination{FollowID: f.ID, Reason: graph.TerminatedModeration, TerminatedAt: now})
		},
	} {
		first := name == "acceptance" // the automatic acceptance already exists
		for i := range 2 {
			err := e.h.UnitOfWork.Tx(ctx, insert)
			if dup := first || i == 1; dup != errors.Is(err, appgraph.ErrAlreadyRecorded) {
				t.Errorf("insert %s #%d = %v; want ErrAlreadyRecorded only for a second event", name, i+1, err)
			}
		}
	}
	if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.repo.InsertFollow(ctx, tx, f) }); err == nil {
		t.Error("InsertFollow with a taken ID and URI succeeded")
	}
}

func followConcurrent(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob := e.user(t, "alice", false), e.user(t, "bob", false)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := e.graph.Follow(ctx, alice, bob); err != nil {
				t.Errorf("Follow = %v", err)
			}
		})
	}
	wg.Wait()
	live := 0
	for _, h := range e.histories(t, alice, bob) {
		if _, ok := h.Edge(); ok {
			live++
		}
	}
	if live != 1 {
		t.Errorf("live follows = %d, want 1", live)
	}
	checkCounters(t, e, alice, 0, 1)
	checkCounters(t, e, bob, 1, 0)
}

// followSettlesOnce accepts a pending request while it is rejected or withdrawn, in transactions
// that each read the request as pending before the other commits, when the database lets them:
// one outcome wins, and the edges and counters equal a rebuild.
func followSettlesOnce(t *testing.T, e *followEnv) {
	ctx := t.Context()
	var meet *rendezvous
	settle := func(context.Context, uow.Tx, uow.Event) error {
		if meet != nil {
			meet.wait()
		}
		return nil
	}
	for _, typ := range []string{appgraph.EventFollowAccepted, appgraph.EventFollowRejected, appgraph.EventFollowTerminated} {
		e.captured.Register(typ, settle)
	}
	followee := e.user(t, "bob", true)
	for i, other := range []struct {
		name string
		run  func(follower string) error
	}{
		{"reject", func(follower string) error {
			_, err := e.graph.RejectFollowRequest(ctx, followee, follower)
			return err
		}},
		{"withdraw", func(follower string) error { _, err := e.graph.Unfollow(ctx, follower, followee); return err }},
	} {
		follower := e.user(t, fmt.Sprintf("alice%d", i), false)
		e.follow(t, follower, followee)
		meet = newRendezvous(2)
		var accepted, settled error
		var wg sync.WaitGroup
		wg.Go(func() { _, accepted = e.graph.AcceptFollowRequest(ctx, followee, follower) })
		wg.Go(func() { settled = other.run(follower) })
		wg.Wait()
		meet = nil
		if accepted != nil && !errors.Is(accepted, appgraph.ErrNotFound) || settled != nil && !errors.Is(settled, appgraph.ErrNotFound) {
			t.Fatalf("accept and %s = %v, %v", other.name, accepted, settled)
		}
		hs := e.histories(t, follower, followee)
		if len(hs) != 1 || hs[0].Acceptance != nil && hs[0].Rejection != nil {
			t.Errorf("accept and %s: histories = %+v, want one follow accepted or rejected, not both", other.name, hs)
		}
		incremental := snapshotFollowProjections(t, e.repo)
		if err := appgraph.RebuildProjections(ctx, e.h.UnitOfWork, e.repo); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		if rebuilt := snapshotFollowProjections(t, e.repo); !reflect.DeepEqual(incremental, rebuilt) {
			t.Errorf("accept and %s: incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", other.name, incremental, rebuilt)
		}
	}
}

// followWithPreferences checks FollowWith: a new follow starts with the requested preferences in
// its one event, a failure records nothing, and following again changes only the preferences that
// it names.
func followWithPreferences(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice", false), e.user(t, "bob", false), e.user(t, "carol", true)
	no, yes := false, true
	injected := errors.New("injected failure")
	var fail bool
	for _, typ := range []string{appgraph.EventFollowed, appgraph.EventFollowRequested} {
		e.captured.Register(typ, func(context.Context, uow.Tx, uow.Event) error {
			if fail {
				return injected
			}
			return nil
		})
	}
	e.events.take()

	fail = true
	if _, err := e.graph.FollowWith(ctx, alice, bob, appgraph.FollowOptions{ShowBoosts: &no, Notify: &yes}); !errors.Is(err, injected) {
		t.Fatalf("FollowWith with a failing projector = %v, want the injected failure", err)
	}
	fail = false
	if hs := e.histories(t, alice, bob); len(hs) != 0 {
		t.Errorf("follows after a failed FollowWith = %+v, want none", hs)
	}
	checkRelationship(t, e, alice, bob, graph.StateNone, graph.StateNone)
	e.events.take()

	for _, followee := range []string{bob, carol} {
		rel, err := e.graph.FollowWith(ctx, alice, followee, appgraph.FollowOptions{ShowBoosts: &no, Notify: &yes})
		if err != nil || rel.ShowBoosts || !rel.Notify {
			t.Errorf("FollowWith(%s) = %+v, %v; want the requested preferences", followee, rel, err)
		}
		events := e.events.take()
		if len(events) != 1 {
			t.Fatalf("FollowWith(%s) events = %+v, want one", followee, events)
		}
		var prefs graph.Preferences
		switch ev := events[0].(type) {
		case appgraph.Followed:
			prefs = ev.Preferences
		case appgraph.FollowRequested:
			prefs = ev.Preferences
		}
		if prefs.ShowBoosts || !prefs.Notify {
			t.Errorf("FollowWith(%s) event = %+v, want the requested preferences", followee, events[0])
		}
	}

	// Following again changes only the named preferences; without any, nothing.
	rel, err := e.graph.FollowWith(ctx, alice, bob, appgraph.FollowOptions{ShowBoosts: &yes})
	if err != nil || !rel.ShowBoosts || !rel.Notify || rel.Following != graph.StateActive {
		t.Errorf("FollowWith(show boosts) again = %+v, %v; want boosts shown and notify kept", rel, err)
	}
	if events := e.events.take(); len(events) != 1 || events[0].EventType() != appgraph.EventFollowPreferencesChanged {
		t.Errorf("events of following again = %+v, want one FollowPreferencesChanged", events)
	}
	if _, err := e.graph.FollowWith(ctx, alice, bob, appgraph.FollowOptions{}); err != nil {
		t.Errorf("FollowWith without preferences again = %v", err)
	}
	if events := e.events.take(); len(events) != 0 {
		t.Errorf("events of following again without preferences = %+v, want none", events)
	}
	if hs := e.histories(t, alice, bob); len(hs) != 1 {
		t.Errorf("follows of alice to bob = %d, want 1", len(hs))
	}
}

func followEmitsEvents(t *testing.T, e *followEnv) {
	ctx := t.Context()
	alice, bob, carol := e.user(t, "alice", false), e.user(t, "bob", true), e.user(t, "carol", false)
	e.events.take()

	step := func(want string, run func() error) uow.Event {
		t.Helper()
		if err := run(); err != nil {
			t.Fatalf("%s: %v", want, err)
		}
		got := e.events.take()
		if len(got) != 1 || got[0].EventType() != want {
			t.Fatalf("events = %+v, want one %s", got, want)
		}
		return got[0]
	}
	followed := step(appgraph.EventFollowed, func() error { _, err := e.graph.Follow(ctx, alice, carol); return err }).(appgraph.Followed)
	if followed.Follow.FollowerAccountID != alice || followed.Follow.FolloweeAccountID != carol ||
		followed.Acceptance.FollowID != followed.Follow.ID || followed.Preferences != graph.DefaultPreferences(followed.Follow.ID) {
		t.Errorf("Followed = %+v", followed)
	}
	requested := step(appgraph.EventFollowRequested, func() error { _, err := e.graph.Follow(ctx, alice, bob); return err }).(appgraph.FollowRequested)
	if requested.Follow.FolloweeAccountID != bob {
		t.Errorf("FollowRequested = %+v", requested)
	}
	accepted := step(appgraph.EventFollowAccepted, func() error { _, err := e.graph.AcceptFollowRequest(ctx, bob, alice); return err }).(appgraph.FollowAccepted)
	if accepted.Follow != requested.Follow || accepted.Acceptance.AcceptedBy != graph.AcceptedByFollowee {
		t.Errorf("FollowAccepted = %+v", accepted)
	}
	changed := step(appgraph.EventFollowPreferencesChanged, func() error {
		_, err := e.graph.UpdatePreferences(ctx, alice, bob, appgraph.PreferencesInput{Notify: true})
		return err
	}).(appgraph.FollowPreferencesChanged)
	if changed.State != graph.StateActive || !changed.Preferences.Notify || changed.Preferences.FollowID != requested.Follow.ID {
		t.Errorf("FollowPreferencesChanged = %+v", changed)
	}
	terminated := step(appgraph.EventFollowTerminated, func() error { _, err := e.graph.Unfollow(ctx, alice, bob); return err }).(appgraph.FollowTerminated)
	if terminated.PreviousState != graph.StateActive || terminated.Termination.Reason != graph.TerminatedUnfollowed {
		t.Errorf("FollowTerminated = %+v", terminated)
	}
	step(appgraph.EventFollowRequested, func() error { _, err := e.graph.Follow(ctx, carol, bob); return err })
	rejected := step(appgraph.EventFollowRejected, func() error { _, err := e.graph.RejectFollowRequest(ctx, bob, carol); return err }).(appgraph.FollowRejected)
	if rejected.Follow.FollowerAccountID != carol || rejected.Rejection.RejectedBy != graph.RejectedByFollowee {
		t.Errorf("FollowRejected = %+v", rejected)
	}

	// Commands that change nothing emit nothing.
	if _, err := e.graph.Follow(ctx, alice, carol); err != nil {
		t.Fatal(err)
	}
	if _, err := e.graph.Unfollow(ctx, carol, alice); err != nil {
		t.Fatal(err)
	}
	if got := e.events.take(); len(got) != 0 {
		t.Errorf("events of no-op commands = %+v, want none", got)
	}
}

func followProjectionEqualsRebuild(t *testing.T, e *followEnv) {
	ctx := t.Context()
	rng := rand.New(rand.NewPCG(95, 20261016)) //nolint:gosec // deterministic test input, not a secret
	var ids []string
	for i := range 6 {
		ids = append(ids, e.user(t, fmt.Sprintf("u%d", i), i%2 == 0))
	}
	pick := func() (string, string) {
		a := ids[rng.IntN(len(ids))]
		for {
			if b := ids[rng.IntN(len(ids))]; b != a {
				return a, b
			}
		}
	}
	for round := range 3 {
		for range 60 {
			e.clock.Advance(time.Duration(rng.IntN(5000)+1) * time.Millisecond)
			a, b := pick()
			var err error
			switch rng.IntN(6) {
			case 0, 1:
				_, err = e.graph.Follow(ctx, a, b)
			case 2:
				_, err = e.graph.AcceptFollowRequest(ctx, b, a)
			case 3:
				_, err = e.graph.RejectFollowRequest(ctx, b, a)
			case 4:
				_, err = e.graph.Unfollow(ctx, a, b)
			default:
				_, err = e.graph.RemoveFollower(ctx, b, a)
			}
			if err != nil && !errors.Is(err, appgraph.ErrNotFound) {
				t.Fatalf("command = %v", err)
			}
		}
		incremental := snapshotFollowProjections(t, e.repo)
		if len(incremental.edges) == 0 || len(incremental.counters) == 0 {
			t.Fatalf("round %d: projections are empty: %+v", round, incremental)
		}
		if err := appgraph.RebuildProjections(ctx, e.h.UnitOfWork, e.repo); err != nil {
			t.Fatalf("RebuildProjections = %v", err)
		}
		rebuilt := snapshotFollowProjections(t, e.repo)
		if !reflect.DeepEqual(incremental, rebuilt) {
			t.Fatalf("round %d: incremental projections differ from a rebuild:\nincremental %+v\nrebuilt     %+v", round, incremental, rebuilt)
		}
	}
}

type followProjections struct {
	edges    []graph.Edge
	counters []graph.Counters
}

func snapshotFollowProjections(t *testing.T, repo appgraph.Repository) followProjections {
	t.Helper()
	edges, err := repo.ListEdges(t.Context())
	if err != nil {
		t.Fatalf("ListEdges = %v", err)
	}
	counters, err := repo.ListCounters(t.Context())
	if err != nil {
		t.Fatalf("ListCounters = %v", err)
	}
	return followProjections{edges: edges, counters: counters}
}
