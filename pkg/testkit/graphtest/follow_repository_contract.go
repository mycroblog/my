// SPDX-License-Identifier: AGPL-3.0-only

package graphtest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/pkg/testkit"
)

// FollowRepositoryContract verifies the follows of the graph repository of a database adapter by
// its methods alone, the port contract of ADR-0153 §1: follows found by ID and by URI, with a
// taken URI refused; acceptances, rejections, and terminations each recorded once; the resends of
// a Follow, recorded once by URI; preferences replaced by an upsert; the follow_edges projection,
// one edge per pair, updated and deleted only from the state and follow it names, and paged
// newest follow first into and out of an account by state; the counters, added from zero and
// gone at zero, upserted, and found; the histories that rebuild them, in follow order; and one
// edge per pair when transactions insert it concurrently (ADR-0153 §5). LockPair, which
// serializes the commands of one pair, is in BlockRepositoryContract. The follow use cases on top
// of it are FollowFlows, which internal/flowtest/graph runs once on SQLite.
func FollowRepositoryContract(t *testing.T, newHarness FollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *graphFollowRepo)
	}{
		{"FindsFollows", graphFollowRepoFollows},
		{"RecordsEachEventOnce", graphFollowRepoEventsOnce},
		{"RecordsResendsOnce", graphFollowRepoResends},
		{"UpsertsPreferences", graphFollowRepoPreferences},
		{"KeepsOneEdgePerPair", graphFollowRepoEdges},
		{"PagesFollowersAndFollowing", graphFollowRepoPages},
		{"KeepsCounters", graphFollowRepoCounters},
		{"EndsManyFollows", graphFollowRepoEndMany},
		{"ListsHistoriesAndRebuilds", graphFollowRepoHistories},
		{"InsertsOneEdgeUnderConcurrency", graphFollowRepoConcurrentEdges},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &graphFollowRepo{uow: h.UnitOfWork, repo: h.Graph, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type graphFollowRepo struct {
	uow   uow.UnitOfWork
	repo  appgraph.Repository
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *graphFollowRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.uow.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// accounts returns n account IDs in increasing order; the graph tables do not reference accounts.
func (r *graphFollowRepo) accounts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = r.ids.NewID()
	}
	return out
}

// follow inserts a follow a second later than the previous one, with its edge in state (pending,
// or active with an automatic acceptance) and its counters, as the use cases and their projectors
// do.
func (r *graphFollowRepo) follow(t *testing.T, follower, followee string, state graph.State) graph.Follow {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	f := graph.Follow{ID: id, URI: graph.FollowURI(testkit.ContractHost, id), FollowerAccountID: follower, FolloweeAccountID: followee, RequestedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertFollow(ctx, tx, f); err != nil {
			return err
		}
		edge := graph.PendingEdge(f)
		if state == graph.StateActive {
			a := graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByAuto, AcceptedAt: f.RequestedAt}
			if err := r.repo.InsertAcceptance(ctx, tx, a); err != nil {
				return err
			}
			edge = graph.ActiveEdge(f, a)
		}
		inserted, err := r.repo.InsertEdge(ctx, tx, edge)
		if err == nil && !inserted {
			return errors.New("InsertEdge inserted nothing")
		}
		if err != nil || state != graph.StateActive {
			return err
		}
		if err := r.repo.AddCounters(ctx, tx, follower, 0, 1); err != nil {
			return err
		}
		return r.repo.AddCounters(ctx, tx, followee, 1, 0)
	})
	return f
}

func graphFollowRepoFollows(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	f := r.follow(t, ids[0], ids[1], graph.StatePending)
	if got, err := r.repo.FindFollow(ctx, f.ID); err != nil || !reflect.DeepEqual(got, f) {
		t.Errorf("FindFollow = %+v, %v; want %+v", got, err, f)
	}
	if _, err := r.repo.FindFollow(ctx, r.ids.NewID()); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindFollow(unknown) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.Follow(ctx, tx, f.ID); err != nil || !reflect.DeepEqual(got, f) {
			t.Errorf("Follow = %+v, %v; want %+v", got, err, f)
		}
		if _, err := r.repo.Follow(ctx, tx, r.ids.NewID()); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("Follow(unknown) = %v, want ErrNotFound", err)
		}
		if got, err := r.repo.FollowByURI(ctx, tx, f.URI); err != nil || !reflect.DeepEqual(got, f) {
			t.Errorf("FollowByURI = %+v, %v; want %+v", got, err, f)
		}
		if _, err := r.repo.FollowByURI(ctx, tx, "https://remote.example/follows/unknown"); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("FollowByURI(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
	taken := graph.Follow{ID: r.ids.NewID(), URI: f.URI, FollowerAccountID: ids[1], FolloweeAccountID: ids[0], RequestedAt: r.clock.Now()}
	if err := r.uow.Tx(ctx, func(tx uow.Tx) error { return r.repo.InsertFollow(ctx, tx, taken) }); err == nil {
		t.Error("InsertFollow with a taken URI succeeded")
	}
}

func graphFollowRepoEventsOnce(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	f := r.follow(t, ids[0], ids[1], graph.StatePending)
	now := r.clock.Now()
	for name, insert := range map[string]func(tx uow.Tx) error{
		"acceptance": func(tx uow.Tx) error {
			return r.repo.InsertAcceptance(ctx, tx, graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByRemote, ActivityURI: "https://remote.example/accepts/1", AcceptedAt: now})
		},
		"rejection": func(tx uow.Tx) error {
			return r.repo.InsertRejection(ctx, tx, graph.Rejection{FollowID: f.ID, RejectedBy: graph.RejectedByRemote, ActivityURI: "https://remote.example/rejects/1", RejectedAt: now})
		},
		"termination": func(tx uow.Tx) error {
			return r.repo.InsertTermination(ctx, tx, graph.Termination{FollowID: f.ID, Reason: graph.TerminatedModeration, TerminatedAt: now})
		},
	} {
		for i := range 2 {
			if err := r.uow.Tx(ctx, insert); (i == 1) != errors.Is(err, appgraph.ErrAlreadyRecorded) || i == 0 && err != nil {
				t.Errorf("insert %s #%d = %v; want ErrAlreadyRecorded only for the second", name, i+1, err)
			}
		}
	}
	var hs []graph.History
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) { hs, err = r.repo.ListHistories(ctx, tx); return err })
	want := graph.History{
		Follow:      f,
		Acceptance:  &graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByRemote, ActivityURI: "https://remote.example/accepts/1", AcceptedAt: now},
		Rejection:   &graph.Rejection{FollowID: f.ID, RejectedBy: graph.RejectedByRemote, ActivityURI: "https://remote.example/rejects/1", RejectedAt: now},
		Termination: &graph.Termination{FollowID: f.ID, Reason: graph.TerminatedModeration, TerminatedAt: now},
	}
	if len(hs) != 1 || !reflect.DeepEqual(hs[0], want) {
		t.Errorf("ListHistories = %+v, want the follow with its three events", hs)
	}
}

func graphFollowRepoResends(t *testing.T, r *graphFollowRepo) {
	ids := r.accounts(2)
	f := r.follow(t, ids[0], ids[1], graph.StateActive)
	resend := graph.Resend{URI: "https://remote.example/follows/again", FollowID: f.ID, ReceivedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			if inserted, err := r.repo.InsertResend(ctx, tx, resend); err != nil || inserted != want {
				t.Errorf("InsertResend #%d = %t, %v; want %t", i+1, inserted, err, want)
			}
		}
		if got, err := r.repo.FollowByResendURI(ctx, tx, resend.URI); err != nil || got.ID != f.ID {
			t.Errorf("FollowByResendURI = %+v, %v; want the follow", got, err)
		}
		if _, err := r.repo.FollowByResendURI(ctx, tx, f.URI); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("FollowByResendURI(the follow's own URI) = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func graphFollowRepoPreferences(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	f := r.follow(t, ids[0], ids[1], graph.StateActive)
	if _, err := r.repo.FindPreferences(ctx, f.ID); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindPreferences before any = %v, want ErrNotFound", err)
	}
	for _, p := range []graph.Preferences{graph.DefaultPreferences(f.ID), {FollowID: f.ID, ShowBoosts: false}} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.UpsertPreferences(ctx, tx, p) })
		if got, err := r.repo.FindPreferences(ctx, f.ID); err != nil || got != p {
			t.Errorf("FindPreferences = %+v, %v; want %+v", got, err, p)
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if got, err := r.repo.Preferences(ctx, tx, f.ID); err != nil || got != p {
				t.Errorf("Preferences = %+v, %v; want %+v", got, err, p)
			}
			return nil
		})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.repo.Preferences(ctx, tx, r.ids.NewID()); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("Preferences(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func graphFollowRepoEdges(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(3)
	f := r.follow(t, ids[0], ids[1], graph.StatePending)
	pending := graph.PendingEdge(f)
	if got, err := r.repo.FindEdge(ctx, ids[0], ids[1]); err != nil || !reflect.DeepEqual(got, pending) {
		t.Errorf("FindEdge = %+v, %v; want %+v", got, err, pending)
	}
	if _, err := r.repo.FindEdge(ctx, ids[1], ids[0]); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindEdge(reverse) = %v, want ErrNotFound", err)
	}
	r.clock.Advance(time.Second)
	active := graph.ActiveEdge(f, graph.Acceptance{FollowID: f.ID, AcceptedBy: graph.AcceptedByFollowee, AcceptedAt: r.clock.Now()})
	other := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if inserted, err := r.repo.InsertEdge(ctx, tx, graph.Edge{FollowerAccountID: ids[0], FolloweeAccountID: ids[1], FollowID: other, State: graph.StatePending, Since: r.clock.Now()}); err != nil || inserted {
			t.Errorf("InsertEdge for a pair with an edge = %t, %v; want false", inserted, err)
		}
		// UpdateEdge changes the edge only from its state and for its follow.
		if ok, err := r.repo.UpdateEdge(ctx, tx, active, graph.StateActive); err != nil || ok {
			t.Errorf("UpdateEdge from the wrong state = %t, %v; want false", ok, err)
		}
		wrong := active
		wrong.FollowID = other
		if ok, err := r.repo.UpdateEdge(ctx, tx, wrong, graph.StatePending); err != nil || ok {
			t.Errorf("UpdateEdge of another follow = %t, %v; want false", ok, err)
		}
		if ok, err := r.repo.UpdateEdge(ctx, tx, active, graph.StatePending); err != nil || !ok {
			t.Errorf("UpdateEdge = %t, %v; want true", ok, err)
		}
		if got, err := r.repo.Edge(ctx, tx, ids[0], ids[1]); err != nil || !reflect.DeepEqual(got, active) {
			t.Errorf("Edge = %+v, %v; want %+v", got, err, active)
		}
		return nil
	})
	g := r.follow(t, ids[2], ids[0], graph.StateActive)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		edges, err := r.repo.ListEdgesOf(ctx, tx, ids[0])
		want := []graph.Edge{active, graph.ActiveEdge(g, graph.Acceptance{FollowID: g.ID, AcceptedAt: g.RequestedAt})}
		if err != nil || !reflect.DeepEqual(edges, want) {
			t.Errorf("ListEdgesOf = %+v, %v; want %+v", edges, err, want)
		}
		// DeleteEdge deletes the edge only in its state and for its follow.
		for _, c := range []struct {
			follow string
			state  graph.State
			want   bool
		}{{other, graph.StateActive, false}, {f.ID, graph.StatePending, false}, {f.ID, graph.StateActive, true}, {f.ID, graph.StateActive, false}} {
			if ok, err := r.repo.DeleteEdge(ctx, tx, ids[0], ids[1], c.follow, c.state); err != nil || ok != c.want {
				t.Errorf("DeleteEdge(%s, %s) = %t, %v; want %t", c.follow, c.state, ok, err, c.want)
			}
		}
		if _, err := r.repo.Edge(ctx, tx, ids[0], ids[1]); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("Edge after DeleteEdge = %v, want ErrNotFound", err)
		}
		return nil
	})
	if edges, err := r.repo.ListEdges(ctx); err != nil || len(edges) != 1 || edges[0].FollowID != g.ID {
		t.Errorf("ListEdges = %+v, %v; want the other edge", edges, err)
	}
}

func graphFollowRepoPages(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	star := r.ids.NewID()
	var fans []string
	for range 5 {
		fan := r.ids.NewID()
		r.follow(t, fan, star, graph.StateActive)
		r.follow(t, star, fan, graph.StateActive)
		r.follow(t, r.ids.NewID(), star, graph.StatePending)
		fans = append([]string{fan}, fans...) // newest follow first
	}
	if n, err := r.repo.CountFollowers(ctx, star, graph.StateActive); err != nil || n != 5 {
		t.Errorf("CountFollowers(active) = %d, %v; want 5", n, err)
	}
	if n, err := r.repo.CountFollowers(ctx, star, graph.StatePending); err != nil || n != 5 {
		t.Errorf("CountFollowers(pending) = %d, %v; want 5", n, err)
	}
	for name, list := range map[string]func(context.Context, string, graph.State, string, int) ([]graph.Edge, error){
		"ListFollowers": r.repo.ListFollowers, "ListFollowing": r.repo.ListFollowing,
	} {
		var got []string
		before := ""
		for range 3 {
			page, err := list(ctx, star, graph.StateActive, before, 2)
			if err != nil || len(page) > 2 {
				t.Fatalf("%s = %d edges, %v; want at most 2", name, len(page), err)
			}
			for _, e := range page {
				other := e.FollowerAccountID
				if name == "ListFollowing" {
					other = e.FolloweeAccountID
				}
				got = append(got, other)
				before = e.FollowID
			}
		}
		if !slices.Equal(got, fans) {
			t.Errorf("%s pages = %v, want %v", name, got, fans)
		}
	}
	page, err := r.repo.ListFollowers(ctx, star, graph.StatePending, "", 10)
	if err != nil || len(page) != 5 || page[0].State != graph.StatePending {
		t.Errorf("ListFollowers(pending) = %+v, %v; want the five requests", page, err)
	}
	if page, err := r.repo.ListFollowing(ctx, star, graph.StatePending, "", 10); err != nil || len(page) != 0 {
		t.Errorf("ListFollowing(pending) = %+v, %v; want none", page, err)
	}
}

// graphFollowRepoEndMany ends many follows with the writes of a batch, as the writes of one
// follow end each: their follows found by ID, their terminations recorded once, their edges
// deleted only in the state given, and the counters of many accounts added, from zero and gone
// at zero.
func graphFollowRepoEndMany(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(4)
	star, fans := ids[0], ids[1:]
	var follows []graph.Follow
	for _, fan := range fans {
		follows = append(follows, r.follow(t, fan, star, graph.StateActive))
	}
	pending := r.follow(t, star, fans[0], graph.StatePending)
	all := append(append([]graph.Follow{}, follows...), pending)
	byID := make([]string, len(all))
	for i, f := range all {
		byID[i] = f.ID
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.LockPairs(ctx, tx, []string{star, star, star}, fans); err != nil {
			return err
		}
		// The edges of many accounts, each once, as ListEdgesOf reads those of one; and the edges
		// between the pairs named, in either direction.
		of, err := r.repo.ListEdgesOfAccounts(ctx, tx, []string{star, fans[0], r.ids.NewID()})
		if want, werr := r.repo.ListEdgesOf(ctx, tx, star); err != nil || werr != nil || !reflect.DeepEqual(of, want) {
			t.Errorf("ListEdgesOfAccounts(star, a fan of it) = %+v, %v; want star's edges once each, %+v", of, err, want)
		}
		between, err := r.repo.ListEdgesBetween(ctx, tx, []string{fans[0], star}, []string{star, fans[1]})
		if err != nil || len(between) != 3 {
			t.Errorf("ListEdgesBetween(fan 0 and star, star and fan 1) = %+v, %v; want the three edges of the two pairs", between, err)
		}
		if got, err := r.repo.ListEdgesBetween(ctx, tx, []string{fans[0]}, []string{fans[1]}); err != nil || len(got) != 0 {
			t.Errorf("ListEdgesBetween(two fans) = %+v, %v; want none", got, err)
		}
		got, err := r.repo.FollowsByIDs(ctx, tx, append(byID, r.ids.NewID()))
		if err != nil {
			return err
		}
		if len(got) != len(all) {
			t.Errorf("FollowsByIDs = %d follows, want the %d that exist", len(got), len(all))
		}
		for _, f := range all {
			if one, err := r.repo.Follow(ctx, tx, f.ID); err != nil || got[f.ID] != one {
				t.Errorf("FollowsByIDs[%s] = %+v, want %+v as Follow finds it (%v)", f.ID, got[f.ID], one, err)
			}
		}
		return r.repo.InsertTerminations(ctx, tx, byID, graph.TerminatedModeration, r.clock.Now())
	})
	err := r.uow.Tx(ctx, func(tx uow.Tx) error {
		return r.repo.InsertTerminations(ctx, tx, byID[:1], graph.TerminatedModeration, r.clock.Now())
	})
	if !errors.Is(err, appgraph.ErrAlreadyRecorded) {
		t.Errorf("InsertTerminations of an ended follow = %v, want ErrAlreadyRecorded", err)
	}
	fresh := r.ids.NewID()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		// The active edges and the pending one, and one named in a state it is not in.
		edges := []graph.Edge{{FollowID: pending.ID, State: graph.StateActive}}
		deleted, err := r.repo.DeleteEdgesOfFollows(ctx, tx, edges)
		if err != nil || deleted != 0 {
			t.Errorf("DeleteEdgesOfFollows in another state = %d, %v; want none", deleted, err)
		}
		edges = []graph.Edge{{FollowID: pending.ID, State: graph.StatePending}}
		for _, f := range follows {
			edges = append(edges, graph.Edge{FollowerAccountID: f.FollowerAccountID, FolloweeAccountID: f.FolloweeAccountID, FollowID: f.ID, State: graph.StateActive})
		}
		if deleted, err = r.repo.DeleteEdgesOfFollows(ctx, tx, edges); err != nil || deleted != len(edges) {
			t.Errorf("DeleteEdgesOfFollows = %d, %v; want %d", deleted, err, len(edges))
		}
		deltas := map[string]appgraph.CounterDelta{star: {Followers: -3}, fresh: {Followers: 2, Following: 1}}
		for _, fan := range fans {
			deltas[fan] = appgraph.CounterDelta{Following: -1}
		}
		return r.repo.AddManyCounters(ctx, tx, deltas)
	})
	if edges, err := r.repo.ListEdges(ctx); err != nil || len(edges) != 0 {
		t.Errorf("ListEdges after DeleteEdgesOfFollows = %+v, %v; want none", edges, err)
	}
	for _, id := range ids {
		if _, err := r.repo.FindCounters(ctx, id); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("FindCounters(%s) after AddManyCounters = %v, want none at zero", id, err)
		}
	}
	if got, err := r.repo.FindCounters(ctx, fresh); err != nil || got != (graph.Counters{AccountID: fresh, FollowersCount: 2, FollowingCount: 1}) {
		t.Errorf("FindCounters of an account without a row after AddManyCounters = %+v, %v; want them from zero", got, err)
	}
	if n, err := r.repo.CountFollowers(ctx, star, graph.StateActive); err != nil || n != 0 {
		t.Errorf("CountFollowers after the end = %d, %v", n, err)
	}
}

func graphFollowRepoCounters(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	if _, err := r.repo.FindCounters(ctx, ids[0]); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindCounters before any = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.AddCounters(ctx, tx, ids[0], 2, 1) })
	if got, err := r.repo.FindCounters(ctx, ids[0]); err != nil || got != (graph.Counters{AccountID: ids[0], FollowersCount: 2, FollowingCount: 1}) {
		t.Errorf("FindCounters after AddCounters = %+v, %v", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.AddCounters(ctx, tx, ids[0], -1, 0) })
	if got, err := r.repo.FindCounters(ctx, ids[0]); err != nil || got.FollowersCount != 1 || got.FollowingCount != 1 {
		t.Errorf("FindCounters after a decrement = %+v, %v", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.AddCounters(ctx, tx, ids[0], -1, -1) })
	if _, err := r.repo.FindCounters(ctx, ids[0]); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindCounters at zero = %v, want ErrNotFound (no row)", err)
	}
	c := graph.Counters{AccountID: ids[1], FollowersCount: 7, FollowingCount: 3}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.UpsertCounters(ctx, tx, c) })
	c.FollowersCount = 8
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.UpsertCounters(ctx, tx, c) })
	if got, err := r.repo.ListCounters(ctx); err != nil || !reflect.DeepEqual(got, []graph.Counters{c}) {
		t.Errorf("ListCounters = %+v, %v; want %+v", got, err, c)
	}
}

func graphFollowRepoHistories(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(3)
	f1 := r.follow(t, ids[0], ids[1], graph.StateActive)
	f2 := r.follow(t, ids[1], ids[2], graph.StatePending)
	f3 := r.follow(t, ids[2], ids[0], graph.StateActive)
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertTermination(ctx, tx, graph.Termination{FollowID: f3.ID, Reason: graph.TerminatedUnfollowed, TerminatedAt: r.clock.Now()}); err != nil {
			return err
		}
		if _, err := r.repo.DeleteEdge(ctx, tx, ids[2], ids[0], f3.ID, graph.StateActive); err != nil {
			return err
		}
		if err := r.repo.AddCounters(ctx, tx, ids[2], 0, -1); err != nil {
			return err
		}
		return r.repo.AddCounters(ctx, tx, ids[0], -1, 0)
	})
	var hs []graph.History
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) { hs, err = r.repo.ListHistories(ctx, tx); return err })
	if len(hs) != 3 || hs[0].Follow != f1 || hs[1].Follow != f2 || hs[2].Follow != f3 ||
		hs[0].Acceptance == nil || hs[1].Acceptance != nil || hs[2].Termination == nil {
		t.Fatalf("ListHistories = %+v, want the three follows in ID order with their events", hs)
	}
	edges, err := r.repo.ListEdges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counters, err := r.repo.ListCounters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.DeleteAllEdges(ctx, tx); err != nil {
			return err
		}
		return r.repo.DeleteAllCounters(ctx, tx)
	})
	if got, err := r.repo.ListEdges(ctx); err != nil || len(got) != 0 {
		t.Errorf("ListEdges after DeleteAllEdges = %+v, %v; want none", got, err)
	}
	if got, err := r.repo.ListCounters(ctx); err != nil || len(got) != 0 {
		t.Errorf("ListCounters after DeleteAllCounters = %+v, %v; want none", got, err)
	}
	// The rebuild from the histories gives back the projections.
	if err := appgraph.RebuildProjections(ctx, r.uow, r.repo); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	if got, _ := r.repo.ListEdges(ctx); !reflect.DeepEqual(got, edges) {
		t.Errorf("rebuilt edges = %+v, want %+v", got, edges)
	}
	if got, _ := r.repo.ListCounters(ctx); !reflect.DeepEqual(got, counters) {
		t.Errorf("rebuilt counters = %+v, want %+v", got, counters)
	}
}

// graphFollowRepoConcurrentEdges inserts the edge of one pair from several transactions at once: one
// inserts it, and the others report false.
func graphFollowRepoConcurrentEdges(t *testing.T, r *graphFollowRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	var (
		mu       sync.Mutex
		inserted int
		wg       sync.WaitGroup
	)
	for range 4 {
		wg.Go(func() {
			id := r.ids.NewID()
			var ok bool
			err := r.uow.Tx(ctx, func(tx uow.Tx) (err error) {
				ok, err = r.repo.InsertEdge(ctx, tx, graph.Edge{FollowerAccountID: ids[0], FolloweeAccountID: ids[1], FollowID: id, State: graph.StatePending, Since: r.clock.Now()})
				return err
			})
			if err != nil {
				t.Errorf("InsertEdge = %v", err)
			}
			if ok {
				mu.Lock()
				inserted++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if edges, err := r.repo.ListEdges(ctx); err != nil || len(edges) != 1 || inserted != 1 {
		t.Errorf("edges = %+v, %v after %d reported inserts; want one", edges, err, inserted)
	}
}
