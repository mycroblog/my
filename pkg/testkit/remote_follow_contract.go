// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

// RemoteFollowContract verifies the repository methods that follows with remote accounts use on a
// database adapter, by those methods alone, the port contract of ADR-0153 §1: a follow is found by
// ID and by the URI of its Follow, and takes one acceptance, rejection, and termination each, which
// its history lists; a Follow received again is recorded once and finds its follow; the
// preferences of a follow are upserted; the edge of a pair is inserted once, updated and deleted
// only from the follow and state named, and counted; and the tombstones of unknown Follows are
// recorded once per Undo and stay live, oldest first, until an object tombstone names their Undo.
// The use cases on top of it are RemoteFollowFlows, which internal/flowtest/federation runs once
// on SQLite.
func RemoteFollowContract(t *testing.T, newHarness RemoteFollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *moveRepo)
	}{
		{"RecordsFollowsAndTheirEvents", followRepoEvents},
		{"RecordsFollowsSentAgain", followRepoResends},
		{"KeepsTheEdgesAndCounters", followRepoEdges},
		{"KeepsTheTombstonesOfUnknownFollows", followRepoTombstones},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &moveRepo{h: h, clock: clock, ids: NewIDGenerator(clock)})
		})
	}
}

// insertFollow inserts a follow from follower to followee now.
func (r *moveRepo) insertFollow(t *testing.T, follower, followee string) graph.Follow {
	t.Helper()
	f := graph.Follow{
		ID: r.ids.NewID(), URI: "https://remote.example/follows/" + r.ids.NewID(),
		FollowerAccountID: follower, FolloweeAccountID: followee, RequestedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Graph.InsertFollow(ctx, tx, f) })
	r.clock.Advance(time.Second)
	return f
}

func followRepoEvents(t *testing.T, r *moveRepo) {
	ctx := t.Context()
	alice, bob, carol := r.account(t, "alice"), r.account(t, "bob"), r.account(t, "carol")
	accepted := r.insertFollow(t, alice, bob)
	rejected := r.insertFollow(t, carol, bob)
	ended := r.insertFollow(t, bob, alice)

	if got, err := r.h.Graph.FindFollow(ctx, accepted.ID); err != nil || got != accepted {
		t.Errorf("FindFollow = %+v, %v; want %+v", got, err, accepted)
	}
	if _, err := r.h.Graph.FindFollow(ctx, r.ids.NewID()); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindFollow(unknown) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Graph.Follow(ctx, tx, rejected.ID); err != nil || got != rejected {
			t.Errorf("Follow = %+v, %v; want %+v", got, err, rejected)
		}
		if got, err := r.h.Graph.FollowByURI(ctx, tx, ended.URI); err != nil || got != ended {
			t.Errorf("FollowByURI = %+v, %v; want %+v", got, err, ended)
		}
		if _, err := r.h.Graph.FollowByURI(ctx, tx, "https://remote.example/follows/unknown"); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("FollowByURI(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})

	now := r.clock.Now()
	acceptance := graph.Acceptance{FollowID: accepted.ID, AcceptedBy: graph.AcceptedByRemote, ActivityURI: "https://remote.example/accepts/1", AcceptedAt: now}
	rejection := graph.Rejection{FollowID: rejected.ID, RejectedBy: graph.RejectedByRemote, ActivityURI: "https://remote.example/rejects/1", RejectedAt: now}
	termination := graph.Termination{FollowID: ended.ID, Reason: graph.TerminatedUnfollowed, TerminatedAt: now}
	insertAll := func() error {
		var errs []error
		for _, insert := range []func(ctx context.Context, tx uow.Tx) error{
			func(ctx context.Context, tx uow.Tx) error { return r.h.Graph.InsertAcceptance(ctx, tx, acceptance) },
			func(ctx context.Context, tx uow.Tx) error { return r.h.Graph.InsertRejection(ctx, tx, rejection) },
			func(ctx context.Context, tx uow.Tx) error { return r.h.Graph.InsertTermination(ctx, tx, termination) },
		} {
			errs = append(errs, r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return insert(ctx, tx) }))
		}
		return errors.Join(errs...)
	}
	if err := insertAll(); err != nil {
		t.Fatalf("the first acceptance, rejection, and termination: %v", err)
	}
	for i, err := range []error{
		r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Graph.InsertAcceptance(ctx, tx, acceptance) }),
		r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Graph.InsertRejection(ctx, tx, rejection) }),
		r.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return r.h.Graph.InsertTermination(ctx, tx, termination) }),
	} {
		if !errors.Is(err, appgraph.ErrAlreadyRecorded) {
			t.Errorf("the second event #%d = %v, want ErrAlreadyRecorded", i+1, err)
		}
	}

	var histories []graph.History
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		histories, err = r.h.Graph.ListHistories(ctx, tx)
		return err
	})
	if len(histories) != 3 {
		t.Fatalf("ListHistories = %+v, want the three follows", histories)
	}
	for i, want := range []graph.Follow{accepted, rejected, ended} {
		if histories[i].Follow != want {
			t.Errorf("history %d follow = %+v, want %+v in ID order", i, histories[i].Follow, want)
		}
	}
	if h := histories[0]; h.Acceptance == nil || *h.Acceptance != acceptance || h.Rejection != nil || h.Termination != nil {
		t.Errorf("history of the accepted follow = %+v, want only %+v", h, acceptance)
	}
	if h := histories[1]; h.Rejection == nil || *h.Rejection != rejection || h.Acceptance != nil {
		t.Errorf("history of the rejected follow = %+v, want only %+v", h, rejection)
	}
	if h := histories[2]; h.Termination == nil || *h.Termination != termination || h.Acceptance != nil {
		t.Errorf("history of the ended follow = %+v, want only %+v", h, termination)
	}

	prefs := graph.Preferences{FollowID: accepted.ID, ShowBoosts: true}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Graph.UpsertPreferences(ctx, tx, prefs) })
	prefs.ShowBoosts = false
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Graph.UpsertPreferences(ctx, tx, prefs); err != nil {
			return err
		}
		if got, err := r.h.Graph.Preferences(ctx, tx, accepted.ID); err != nil || got != prefs {
			t.Errorf("Preferences = %+v, %v; want the upserted %+v", got, err, prefs)
		}
		return nil
	})
	if got, err := r.h.Graph.FindPreferences(ctx, accepted.ID); err != nil || got != prefs {
		t.Errorf("FindPreferences = %+v, %v; want %+v", got, err, prefs)
	}
}

func followRepoResends(t *testing.T, r *moveRepo) {
	alice, bob := r.account(t, "alice"), r.account(t, "bob")
	f := r.insertFollow(t, alice, bob)
	resend := graph.Resend{URI: "https://remote.example/follows/again", FollowID: f.ID, ReceivedAt: r.clock.Now()}
	for i, want := range []bool{true, false} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Graph.InsertResend(ctx, tx, resend)
			if err == nil && ok != want {
				t.Errorf("InsertResend #%d = %v, want %v", i+1, ok, want)
			}
			return err
		})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Graph.FollowByResendURI(ctx, tx, resend.URI); err != nil || got != f {
			t.Errorf("FollowByResendURI = %+v, %v; want %+v", got, err, f)
		}
		if _, err := r.h.Graph.FollowByResendURI(ctx, tx, f.URI); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("FollowByResendURI(the first Follow) = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func followRepoEdges(t *testing.T, r *moveRepo) {
	ctx := t.Context()
	alice, bob := r.account(t, "alice"), r.account(t, "bob")
	f := r.insertFollow(t, alice, bob)
	other := r.insertFollow(t, alice, bob)
	pending := graph.Edge{FollowerAccountID: alice, FolloweeAccountID: bob, FollowID: f.ID, State: graph.StatePending, Since: r.clock.Now()}
	active := pending
	active.State, active.Since = graph.StateActive, r.clock.Now().Add(time.Second)

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.h.Graph.LockPair(ctx, tx, bob, alice); err != nil {
			return err
		}
		for i, want := range []bool{true, false} {
			ok, err := r.h.Graph.InsertEdge(ctx, tx, pending)
			if err != nil {
				return err
			}
			if ok != want {
				t.Errorf("InsertEdge #%d = %v, want %v", i+1, ok, want)
			}
		}
		if ok, err := r.h.Graph.UpdateEdge(ctx, tx, graph.Edge{FollowerAccountID: alice, FolloweeAccountID: bob, FollowID: other.ID, State: graph.StateActive, Since: active.Since}, graph.StatePending); err != nil || ok {
			t.Errorf("UpdateEdge of another follow = %v, %v; want false", ok, err)
		}
		if ok, err := r.h.Graph.UpdateEdge(ctx, tx, active, graph.StateActive); err != nil || ok {
			t.Errorf("UpdateEdge from the wrong state = %v, %v; want false", ok, err)
		}
		if ok, err := r.h.Graph.UpdateEdge(ctx, tx, active, graph.StatePending); err != nil || !ok {
			t.Errorf("UpdateEdge = %v, %v; want true", ok, err)
		}
		if got, err := r.h.Graph.Edge(ctx, tx, alice, bob); err != nil || !equalEdge(got, active) {
			t.Errorf("Edge = %+v, %v; want %+v", got, err, active)
		}
		if _, err := r.h.Graph.Edge(ctx, tx, bob, alice); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("Edge(the other way) = %v, want ErrNotFound", err)
		}
		if err := r.h.Graph.AddCounters(ctx, tx, bob, 1, 0); err != nil {
			return err
		}
		return r.h.Graph.AddCounters(ctx, tx, alice, 0, 1)
	})
	if got, err := r.h.Graph.FindEdge(ctx, alice, bob); err != nil || !equalEdge(got, active) {
		t.Errorf("FindEdge = %+v, %v; want %+v", got, err, active)
	}
	if n, err := r.h.Graph.CountFollowers(ctx, bob, graph.StateActive); err != nil || n != 1 {
		t.Errorf("CountFollowers(active) = %d, %v; want 1", n, err)
	}
	if got, err := r.h.Graph.FindCounters(ctx, bob); err != nil || got != (graph.Counters{AccountID: bob, FollowersCount: 1}) {
		t.Errorf("FindCounters(bob) = %+v, %v; want one follower", got, err)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if ok, err := r.h.Graph.DeleteEdge(ctx, tx, alice, bob, f.ID, graph.StatePending); err != nil || ok {
			t.Errorf("DeleteEdge in the wrong state = %v, %v; want false", ok, err)
		}
		if ok, err := r.h.Graph.DeleteEdge(ctx, tx, alice, bob, other.ID, graph.StateActive); err != nil || ok {
			t.Errorf("DeleteEdge of another follow = %v, %v; want false", ok, err)
		}
		if ok, err := r.h.Graph.DeleteEdge(ctx, tx, alice, bob, f.ID, graph.StateActive); err != nil || !ok {
			t.Errorf("DeleteEdge = %v, %v; want true", ok, err)
		}
		if err := r.h.Graph.AddCounters(ctx, tx, bob, -1, 0); err != nil {
			return err
		}
		return r.h.Graph.AddCounters(ctx, tx, alice, 0, -1)
	})
	if _, err := r.h.Graph.FindEdge(ctx, alice, bob); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindEdge after DeleteEdge = %v, want ErrNotFound", err)
	}
	if counters, err := r.h.Graph.ListCounters(ctx); err != nil || len(counters) != 0 {
		t.Errorf("ListCounters after both reach zero = %+v, %v; want no rows", counters, err)
	}
}

// equalEdge compares edges by their fields and the instants of their times.
func equalEdge(a, b graph.Edge) bool {
	return a.FollowerAccountID == b.FollowerAccountID && a.FolloweeAccountID == b.FolloweeAccountID &&
		a.FollowID == b.FollowID && a.State == b.State && a.Since.Equal(b.Since)
}

func followRepoTombstones(t *testing.T, r *moveRepo) {
	const (
		actor    = federation.IRI("https://remote.example/users/alice")
		followee = federation.IRI("https://local.example/users/bob")
	)
	first := federation.RemoteFollowTombstone{ActivityURI: actor + "/undo/1", ActorURI: actor, ObjectURI: followee, ReceivedAt: r.clock.Now()}
	second := federation.RemoteFollowTombstone{ActivityURI: actor + "/undo/2", ActorURI: actor, ObjectURI: followee, ReceivedAt: r.clock.Now().Add(time.Second)}
	insert := func(ts federation.RemoteFollowTombstone, want bool) {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Inbox.InsertRemoteFollowTombstone(ctx, tx, ts)
			if err == nil && ok != want {
				t.Errorf("InsertRemoteFollowTombstone(%s) = %v, want %v", ts.ActivityURI, ok, want)
			}
			return err
		})
	}
	live := func() (federation.RemoteFollowTombstone, error) {
		var got federation.RemoteFollowTombstone
		err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			var err error
			got, err = r.h.Inbox.LiveRemoteFollowTombstone(t.Context(), tx, actor, followee)
			return err
		})
		return got, err
	}
	if _, err := live(); !errors.Is(err, appfederation.ErrNotFound) {
		t.Errorf("LiveRemoteFollowTombstone with none = %v, want ErrNotFound", err)
	}
	insert(second, true)
	insert(first, true)
	insert(first, false)
	if got, err := live(); err != nil || !equalFollowTombstone(got, first) {
		t.Errorf("LiveRemoteFollowTombstone = %+v, %v; want the oldest %+v", got, err, first)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.h.Inbox.LiveRemoteFollowTombstone(ctx, tx, followee, actor); !errors.Is(err, appfederation.ErrNotFound) {
			t.Errorf("LiveRemoteFollowTombstone of the other way = %v, want ErrNotFound", err)
		}
		// Dropping a Follow leaves an object tombstone of the Follow that names the Undo.
		_, err := r.h.Inbox.InsertRemoteObjectTombstone(ctx, tx, federation.RemoteObjectTombstone{
			URI: actor + "/follows/1", ActorURI: actor, ActivityURI: first.ActivityURI, ReceivedAt: r.clock.Now(),
		})
		return err
	})
	if got, err := live(); err != nil || !equalFollowTombstone(got, second) {
		t.Errorf("LiveRemoteFollowTombstone after the first dropped a Follow = %+v, %v; want %+v", got, err, second)
	}
}

func equalFollowTombstone(a, b federation.RemoteFollowTombstone) bool {
	return a.ActivityURI == b.ActivityURI && a.ActorURI == b.ActorURI && a.ObjectURI == b.ObjectURI && a.ReceivedAt.Equal(b.ReceivedAt)
}
