// SPDX-License-Identifier: AGPL-3.0-only

package graphtest

import (
	"cmp"
	"context"
	"errors"
	"maps"
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

// BlockRepositoryContract verifies the blocks and mutes of the graph repository of a database
// adapter by its methods alone, the port contract of ADR-0153 §1: blocks with their retractions,
// each recorded once, found by ID and by URI; the block_edges projection, paged newest first out of
// a blocker and in block order into a blocked account, and rebuilt from the blocks; mutes replaced
// by an upsert, hidden on read once expired, paged newest first, and deleted when expired in
// batches; domain mutes inserted once, paged in the order of their domains, and counted; the
// relations among a set of accounts; and LockPair, which serializes the transactions of one pair
// whichever order they name it in (ADR-0153 §5). The block and mute use cases on top of it are
// BlockFlows, which internal/flowtest/graph runs once on SQLite.
func BlockRepositoryContract(t *testing.T, newHarness FollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *blockRepo)
	}{
		{"FindsBlocksAndRetractions", blockRepoBlocks},
		{"RecordsEachRetractionOnce", blockRepoRetractionOnce},
		{"KeepsOneEdgePerPair", blockRepoEdges},
		{"PagesBlockEdges", blockRepoPages},
		{"RebuildsBlockEdges", blockRepoRebuild},
		{"UpsertsAndExpiresMutes", blockRepoMutes},
		{"PagesMutes", blockRepoMutePages},
		{"DeletesExpiredMutesInBatches", blockRepoDeleteExpired},
		{"KeepsDomainMutes", blockRepoDomainMutes},
		{"ListsRelationsAmongAccounts", blockRepoRelations},
		{"ReadsTheRelationshipPartsOfManyAccounts", blockRepoRelationshipParts},
		{"ReadsTheBlocksAndNotificationRelationsOfOneWithMany", blockRepoOneWithMany},
		{"WritesManyBlocksAndMutes", blockRepoWritesMany},
		{"LockPairSerializesAPair", blockRepoLockPair},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &blockRepo{uow: h.UnitOfWork, repo: h.Graph, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type blockRepo struct {
	uow   uow.UnitOfWork
	repo  appgraph.Repository
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *blockRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.uow.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// accounts returns n account IDs in increasing order; the graph tables do not reference accounts.
func (r *blockRepo) accounts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = r.ids.NewID()
	}
	return out
}

// block inserts a block with its edge, as the use case and its projector do, a second later than
// the previous one.
func (r *blockRepo) block(t *testing.T, blocker, blocked string) graph.Block {
	t.Helper()
	r.clock.Advance(time.Second)
	id := r.ids.NewID()
	b := graph.Block{ID: id, URI: graph.BlockURI(testkit.ContractHost, id), BlockerAccountID: blocker, BlockedAccountID: blocked, CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertBlock(ctx, tx, b); err != nil {
			return err
		}
		inserted, err := r.repo.InsertBlockEdge(ctx, tx, graph.EdgeOf(b))
		if err == nil && !inserted {
			err = errors.New("InsertBlockEdge inserted nothing")
		}
		return err
	})
	return b
}

func (r *blockRepo) retract(t *testing.T, b graph.Block) graph.BlockRetraction {
	t.Helper()
	r.clock.Advance(time.Second)
	ret := graph.BlockRetraction{BlockID: b.ID, RetractedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := r.repo.InsertBlockRetraction(ctx, tx, ret); err != nil {
			return err
		}
		return r.repo.DeleteBlockEdge(ctx, tx, b.BlockerAccountID, b.BlockedAccountID, b.ID)
	})
	return ret
}

func (r *blockRepo) mute(t *testing.T, m graph.Mute) {
	t.Helper()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.UpsertMute(ctx, tx, m) })
}

func blockRepoBlocks(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	b := r.block(t, ids[0], ids[1])
	if got, err := r.repo.FindBlock(ctx, b.ID); err != nil || !reflect.DeepEqual(got, b) {
		t.Errorf("FindBlock = %+v, %v; want %+v", got, err, b)
	}
	if _, err := r.repo.FindBlock(ctx, r.ids.NewID()); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindBlock(unknown) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.Block(ctx, tx, b.ID); err != nil || !reflect.DeepEqual(got, b) {
			t.Errorf("Block = %+v, %v; want %+v", got, err, b)
		}
		if _, err := r.repo.Block(ctx, tx, r.ids.NewID()); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("Block(unknown) = %v, want ErrNotFound", err)
		}
		if got, err := r.repo.BlockByURI(ctx, tx, b.URI); err != nil || !reflect.DeepEqual(got, graph.BlockHistory{Block: b}) {
			t.Errorf("BlockByURI = %+v, %v; want the block without a retraction", got, err)
		}
		if _, err := r.repo.BlockByURI(ctx, tx, "https://remote.example/blocks/unknown"); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("BlockByURI(unknown) = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err := r.uow.Tx(ctx, func(tx uow.Tx) error { return r.repo.InsertBlock(ctx, tx, b) }); err == nil {
		t.Error("InsertBlock with a taken ID and URI succeeded")
	}

	// A received block without an activity id has no URI; it is stored as such.
	r.clock.Advance(time.Second)
	anon := graph.Block{ID: r.ids.NewID(), BlockerAccountID: ids[1], BlockedAccountID: ids[0], CreatedAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.InsertBlock(ctx, tx, anon) })
	if got, err := r.repo.FindBlock(ctx, anon.ID); err != nil || got.URI != "" {
		t.Errorf("FindBlock(without a URI) = %+v, %v", got, err)
	}

	ret := r.retract(t, b)
	var histories []graph.BlockHistory
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		got, err := r.repo.BlockByURI(ctx, tx, b.URI)
		if err != nil || got.Retraction == nil || *got.Retraction != ret {
			t.Errorf("BlockByURI after the retraction = %+v, %v; want retraction %+v", got, err, ret)
		}
		histories, err = r.repo.ListBlockHistories(ctx, tx)
		return err
	})
	want := []graph.BlockHistory{{Block: b, Retraction: &ret}, {Block: anon}}
	if !reflect.DeepEqual(histories, want) {
		t.Errorf("ListBlockHistories = %+v, want %+v in block ID order", histories, want)
	}
	// A retracted block is still found by ID.
	if _, err := r.repo.FindBlock(ctx, b.ID); err != nil {
		t.Errorf("FindBlock(retracted) = %v", err)
	}
}

func blockRepoRetractionOnce(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	b := r.block(t, ids[0], ids[1])
	for i := range 2 {
		err := r.uow.Tx(ctx, func(tx uow.Tx) error {
			return r.repo.InsertBlockRetraction(ctx, tx, graph.BlockRetraction{BlockID: b.ID, RetractedAt: r.clock.Now()})
		})
		if dup := i == 1; dup != errors.Is(err, appgraph.ErrAlreadyRecorded) {
			t.Errorf("InsertBlockRetraction #%d = %v; want ErrAlreadyRecorded only for the second", i+1, err)
		}
	}
}

func blockRepoEdges(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	b := r.block(t, ids[0], ids[1])
	want := graph.EdgeOf(b)
	if got, err := r.repo.FindBlockEdge(ctx, ids[0], ids[1]); err != nil || got != want {
		t.Errorf("FindBlockEdge = %+v, %v; want %+v", got, err, want)
	}
	if _, err := r.repo.FindBlockEdge(ctx, ids[1], ids[0]); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindBlockEdge(the other way) = %v, want ErrNotFound", err)
	}
	other := graph.BlockEdge{BlockerAccountID: ids[0], BlockedAccountID: ids[1], BlockID: r.ids.NewID()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.repo.BlockEdge(ctx, tx, ids[0], ids[1]); err != nil || got != want {
			t.Errorf("BlockEdge = %+v, %v; want %+v", got, err, want)
		}
		if inserted, err := r.repo.InsertBlockEdge(ctx, tx, other); err != nil || inserted {
			t.Errorf("InsertBlockEdge(a second edge of the pair) = %v, %v; want false", inserted, err)
		}
		// Deleting the edge of another block leaves the pair's edge alone.
		return r.repo.DeleteBlockEdge(ctx, tx, ids[0], ids[1], other.BlockID)
	})
	if got, err := r.repo.FindBlockEdge(ctx, ids[0], ids[1]); err != nil || got != want {
		t.Errorf("FindBlockEdge after deleting another block's edge = %+v, %v; want %+v", got, err, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return r.repo.DeleteBlockEdge(ctx, tx, ids[0], ids[1], b.ID)
	})
	if _, err := r.repo.FindBlockEdge(ctx, ids[0], ids[1]); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindBlockEdge after DeleteBlockEdge = %v, want ErrNotFound", err)
	}
	if edges, err := r.repo.ListBlockEdges(ctx); err != nil || len(edges) != 0 {
		t.Errorf("ListBlockEdges = %+v, %v; want none", edges, err)
	}
}

func blockRepoPages(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(6)
	blocker, others := ids[0], ids[1:]
	var newestFirst []string
	for _, id := range others {
		r.block(t, blocker, id)
		newestFirst = append([]string{id}, newestFirst...)
	}
	// A retracted block leaves the pages; a block the other way is not blocker's.
	retracted := r.block(t, blocker, r.ids.NewID())
	r.retract(t, retracted)
	r.block(t, others[0], blocker)

	var got []string
	before := ""
	for range 4 {
		page, err := r.repo.ListBlocking(ctx, blocker, before, 2)
		if err != nil {
			t.Fatalf("ListBlocking = %v", err)
		}
		if len(page) > 2 {
			t.Fatalf("ListBlocking page = %d edges, want at most 2", len(page))
		}
		for _, e := range page {
			got = append(got, e.BlockedAccountID)
			before = e.BlockID
		}
	}
	if !reflect.DeepEqual(got, newestFirst) {
		t.Errorf("ListBlocking pages = %v, want %v", got, newestFirst)
	}

	// The blockers of one account, in block order.
	target := r.ids.NewID()
	var blockers []string
	for _, id := range others {
		r.block(t, id, target)
		blockers = append(blockers, id)
	}
	got = nil
	after := ""
	for range 4 {
		page, err := r.repo.ListBlockers(ctx, target, after, 2)
		if err != nil {
			t.Fatalf("ListBlockers = %v", err)
		}
		for _, e := range page {
			got = append(got, e.BlockerAccountID)
			after = e.BlockID
		}
	}
	if !reflect.DeepEqual(got, blockers) {
		t.Errorf("ListBlockers pages = %v, want %v", got, blockers)
	}

	edges, err := r.repo.ListBlockEdges(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2*len(others)+1 {
		t.Errorf("ListBlockEdges = %d edges, want %d", len(edges), 2*len(others)+1)
	}
	if !slices.IsSortedFunc(edges, func(a, b graph.BlockEdge) int {
		if c := cmp.Compare(a.BlockerAccountID, b.BlockerAccountID); c != 0 {
			return c
		}
		return cmp.Compare(a.BlockedAccountID, b.BlockedAccountID)
	}) {
		t.Errorf("ListBlockEdges = %+v, want them by blocker and blocked account", edges)
	}
}

func blockRepoRebuild(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(3)
	kept := r.block(t, ids[0], ids[1])
	r.retract(t, r.block(t, ids[1], ids[2]))
	again := r.block(t, ids[1], ids[2])
	// A stale edge that no block in force has, and a missing edge of a block in force.
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.repo.InsertBlockEdge(ctx, tx, graph.BlockEdge{BlockerAccountID: ids[2], BlockedAccountID: ids[0], BlockID: r.ids.NewID()}); err != nil {
			return err
		}
		return r.repo.DeleteBlockEdge(ctx, tx, again.BlockerAccountID, again.BlockedAccountID, again.ID)
	})
	if err := appgraph.RebuildProjections(ctx, r.uow, r.repo); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	edges, err := r.repo.ListBlockEdges(ctx)
	if want := []graph.BlockEdge{graph.EdgeOf(kept), graph.EdgeOf(again)}; err != nil || !reflect.DeepEqual(edges, want) {
		t.Errorf("the rebuilt block edges = %+v, %v; want %+v", edges, err, want)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.repo.DeleteAllBlockEdges(ctx, tx) })
	if edges, err := r.repo.ListBlockEdges(ctx); err != nil || len(edges) != 0 {
		t.Errorf("ListBlockEdges after DeleteAllBlockEdges = %+v, %v", edges, err)
	}
}

func blockRepoMutes(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(3)
	owner, bob, carol := ids[0], ids[1], ids[2]
	now := r.clock.Now()
	expires := now.Add(time.Hour)
	forBob := graph.Mute{AccountID: owner, TargetAccountID: bob, HideNotifications: true, CreatedAt: now}
	forCarol := graph.Mute{AccountID: owner, TargetAccountID: carol, ExpiresAt: &expires, CreatedAt: now}
	r.mute(t, forBob)
	r.mute(t, forCarol)
	if got, err := r.repo.FindMute(ctx, owner, bob, now); err != nil || !reflect.DeepEqual(got, forBob) {
		t.Errorf("FindMute = %+v, %v; want %+v", got, err, forBob)
	}
	if got, err := r.repo.FindMute(ctx, owner, carol, now); err != nil || !reflect.DeepEqual(got, forCarol) {
		t.Errorf("FindMute(with expiry) = %+v, %v; want %+v", got, err, forCarol)
	}
	if _, err := r.repo.FindMute(ctx, bob, owner, now); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindMute(the other way) = %v, want ErrNotFound", err)
	}
	// At its expiry the mute no longer applies on read, but it is still stored.
	if _, err := r.repo.FindMute(ctx, owner, carol, expires); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindMute at the expiry = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if _, err := r.repo.Mute(ctx, tx, owner, carol, expires); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("Mute at the expiry = %v, want ErrNotFound", err)
		}
		if got, err := r.repo.Mute(ctx, tx, owner, bob, expires); err != nil || !reflect.DeepEqual(got, forBob) {
			t.Errorf("Mute = %+v, %v; want %+v", got, err, forBob)
		}
		all, err := r.repo.ListAllMutes(ctx, tx)
		if want := sortedMutes(forBob, forCarol); err != nil || !reflect.DeepEqual(all, want) {
			t.Errorf("ListAllMutes = %+v, %v; want %+v", all, err, want)
		}
		return nil
	})
	if muters, err := r.repo.ListMuters(ctx, carol, expires, "", 10); err != nil || len(muters) != 0 {
		t.Errorf("ListMuters at the expiry = %+v, %v; want none", muters, err)
	}

	// Muting again replaces the mute.
	r.clock.Advance(time.Minute)
	renewed := graph.Mute{AccountID: owner, TargetAccountID: carol, CreatedAt: r.clock.Now()}
	r.mute(t, renewed)
	if got, err := r.repo.FindMute(ctx, owner, carol, expires); err != nil || !reflect.DeepEqual(got, renewed) {
		t.Errorf("FindMute after a second UpsertMute = %+v, %v; want %+v", got, err, renewed)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			if deleted, err := r.repo.DeleteMute(ctx, tx, owner, bob); err != nil || deleted != want {
				t.Errorf("DeleteMute #%d = %v, %v; want %v", i+1, deleted, err, want)
			}
		}
		return nil
	})
	if _, err := r.repo.FindMute(ctx, owner, bob, now); !errors.Is(err, appgraph.ErrNotFound) {
		t.Errorf("FindMute after DeleteMute = %v, want ErrNotFound", err)
	}
}

// sortedMutes returns ms ordered by account and target, as ListAllMutes returns them.
func sortedMutes(ms ...graph.Mute) []graph.Mute {
	slices.SortFunc(ms, func(a, b graph.Mute) int {
		if c := cmp.Compare(a.AccountID, b.AccountID); c != 0 {
			return c
		}
		return cmp.Compare(a.TargetAccountID, b.TargetAccountID)
	})
	return ms
}

func blockRepoMutePages(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	owner := r.ids.NewID()
	var want []string
	for range 4 {
		r.clock.Advance(time.Second)
		id := r.ids.NewID()
		r.mute(t, graph.Mute{AccountID: owner, TargetAccountID: id, CreatedAt: r.clock.Now()})
		want = append([]string{id}, want...) // newest first
	}
	// Two mutes created at the same instant come by target account ID, the greater first.
	r.clock.Advance(time.Second)
	x, y := r.ids.NewID(), r.ids.NewID()
	for _, id := range []string{x, y} {
		r.mute(t, graph.Mute{AccountID: owner, TargetAccountID: id, CreatedAt: r.clock.Now()})
	}
	want = append([]string{y, x}, want...)
	// An expired mute and another account's mute are left out.
	r.clock.Advance(time.Second)
	expired := r.clock.Now().Add(time.Second)
	r.mute(t, graph.Mute{AccountID: owner, TargetAccountID: r.ids.NewID(), ExpiresAt: &expired, CreatedAt: r.clock.Now()})
	r.mute(t, graph.Mute{AccountID: r.ids.NewID(), TargetAccountID: x, CreatedAt: r.clock.Now()})
	r.clock.Advance(time.Second)

	var got []string
	var before appgraph.MuteCursor
	for range 4 {
		page, err := r.repo.ListMutes(ctx, owner, r.clock.Now(), before, 2)
		if err != nil {
			t.Fatalf("ListMutes = %v", err)
		}
		if len(page) > 2 {
			t.Fatalf("ListMutes page = %d mutes, want at most 2", len(page))
		}
		for _, m := range page {
			got = append(got, m.TargetAccountID)
			before = appgraph.MuteCursorOf(m)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListMutes pages = %v, want %v", got, want)
	}

	// The muters of x, in the order of the muting accounts.
	var muters []string
	after := ""
	for range 3 {
		page, err := r.repo.ListMuters(ctx, x, r.clock.Now(), after, 1)
		if err != nil {
			t.Fatalf("ListMuters = %v", err)
		}
		for _, m := range page {
			muters = append(muters, m.AccountID)
			after = m.AccountID
		}
	}
	if len(muters) != 2 || !slices.IsSorted(muters) || !slices.Contains(muters, owner) {
		t.Errorf("ListMuters pages = %v, want owner and the other account in ID order", muters)
	}
}

func blockRepoDeleteExpired(t *testing.T, r *blockRepo) {
	owner := r.ids.NewID()
	now := r.clock.Now()
	var expiring []string
	for i := range 3 {
		at := now.Add(time.Duration(i+1) * time.Minute)
		id := r.ids.NewID()
		r.mute(t, graph.Mute{AccountID: owner, TargetAccountID: id, ExpiresAt: &at, CreatedAt: now})
		expiring = append(expiring, id)
	}
	later := now.Add(time.Hour)
	forever, notYet := r.ids.NewID(), r.ids.NewID()
	r.mute(t, graph.Mute{AccountID: owner, TargetAccountID: forever, CreatedAt: now})
	r.mute(t, graph.Mute{AccountID: owner, TargetAccountID: notYet, ExpiresAt: &later, CreatedAt: now})

	deleted := func(at time.Time, limit int) []string {
		t.Helper()
		var out []string
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ms, err := r.repo.DeleteExpiredMutes(ctx, tx, at, limit)
			for _, m := range ms {
				out = append(out, m.TargetAccountID)
			}
			return err
		})
		slices.Sort(out)
		return out
	}
	if got := deleted(now, 10); len(got) != 0 {
		t.Errorf("DeleteExpiredMutes before any expiry = %v, want none", got)
	}
	at := now.Add(3 * time.Minute)
	first := deleted(at, 2)
	if want := expiring[:2]; !reflect.DeepEqual(first, want) {
		t.Errorf("DeleteExpiredMutes(limit 2) = %v, want the 2 that expired first %v", first, want)
	}
	if got := deleted(at, 10); !reflect.DeepEqual(got, expiring[2:]) {
		t.Errorf("DeleteExpiredMutes again = %v, want %v", got, expiring[2:])
	}
	var left []string
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		all, err := r.repo.ListAllMutes(ctx, tx)
		for _, m := range all {
			left = append(left, m.TargetAccountID)
		}
		return err
	})
	if want := []string{forever, notYet}; !reflect.DeepEqual(left, want) {
		t.Errorf("mutes left = %v, want %v", left, want)
	}
}

func blockRepoDomainMutes(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(2)
	alice, bob := ids[0], ids[1]
	now := r.clock.Now()
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, d := range []string{"c.example", "a.example", "b.example"} {
			if inserted, err := r.repo.InsertDomainMute(ctx, tx, graph.DomainMute{AccountID: alice, Domain: d, CreatedAt: now}); err != nil || !inserted {
				t.Errorf("InsertDomainMute(%s) = %v, %v; want true", d, inserted, err)
			}
		}
		if inserted, err := r.repo.InsertDomainMute(ctx, tx, graph.DomainMute{AccountID: alice, Domain: "a.example", CreatedAt: now.Add(time.Second)}); err != nil || inserted {
			t.Errorf("InsertDomainMute(again) = %v, %v; want false", inserted, err)
		}
		if _, err := r.repo.InsertDomainMute(ctx, tx, graph.DomainMute{AccountID: bob, Domain: "a.example", CreatedAt: now}); err != nil {
			return err
		}
		if n, err := r.repo.CountDomainMutes(ctx, tx, alice); err != nil || n != 3 {
			t.Errorf("CountDomainMutes = %d, %v; want 3", n, err)
		}
		return nil
	})

	var got []graph.DomainMute
	after := ""
	for range 3 {
		page, err := r.repo.ListDomainMutes(ctx, alice, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range page {
			got = append(got, m)
			after = m.Domain
		}
	}
	want := []graph.DomainMute{
		{AccountID: alice, Domain: "a.example", CreatedAt: now},
		{AccountID: alice, Domain: "b.example", CreatedAt: now},
		{AccountID: alice, Domain: "c.example", CreatedAt: now},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListDomainMutes pages = %+v, want %+v", got, want)
	}
	for _, tc := range []struct {
		domains []string
		want    bool
	}{{[]string{"x.example", "b.example"}, true}, {[]string{"x.example"}, false}, {nil, false}} {
		if got, err := r.repo.MutesAnyDomain(ctx, alice, tc.domains); err != nil || got != tc.want {
			t.Errorf("MutesAnyDomain(%v) = %v, %v; want %v", tc.domains, got, err, tc.want)
		}
	}
	// DomainMutersAmong tells of many accounts what MutesAnyDomain tells of one.
	stranger := r.ids.NewID()
	for _, tc := range []struct {
		domains []string
		want    map[string]bool
	}{{[]string{"x.example", "b.example"}, map[string]bool{alice: true}}, {[]string{"a.example"}, map[string]bool{alice: true, bob: true}}, {[]string{"x.example"}, map[string]bool{}}, {nil, map[string]bool{}}} {
		if got, err := r.repo.DomainMutersAmong(ctx, []string{alice, bob, stranger}, tc.domains); err != nil || !maps.Equal(got, tc.want) {
			t.Errorf("DomainMutersAmong(%v) = %v, %v; want %v", tc.domains, got, err, tc.want)
		}
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for i, want := range []bool{true, false} {
			if deleted, err := r.repo.DeleteDomainMute(ctx, tx, alice, "b.example"); err != nil || deleted != want {
				t.Errorf("DeleteDomainMute #%d = %v, %v; want %v", i+1, deleted, err, want)
			}
		}
		all, err := r.repo.ListAllDomainMutes(ctx, tx)
		var keys []string
		for _, m := range all {
			keys = append(keys, m.AccountID+" "+m.Domain)
		}
		want := []string{alice + " a.example", alice + " c.example", bob + " a.example"}
		if err != nil || !reflect.DeepEqual(keys, want) {
			t.Errorf("ListAllDomainMutes = %v, %v; want %v", keys, err, want)
		}
		return nil
	})
}

func blockRepoRelations(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(5)
	viewer, blocked, blocker, muted, expired := ids[0], ids[1], ids[2], ids[3], ids[4]
	r.block(t, viewer, blocked)
	r.block(t, blocker, viewer)
	now := r.clock.Now()
	at := now.Add(time.Minute)
	r.mute(t, graph.Mute{AccountID: viewer, TargetAccountID: muted, CreatedAt: now})
	r.mute(t, graph.Mute{AccountID: viewer, TargetAccountID: expired, ExpiresAt: &at, CreatedAt: now})

	got, err := r.repo.RelationsAmong(ctx, viewer, ids[1:], at)
	if err != nil {
		t.Fatalf("RelationsAmong = %v", err)
	}
	if !reflect.DeepEqual(got.Blocked, []string{blocked}) || !reflect.DeepEqual(got.BlockedBy, []string{blocker}) ||
		!reflect.DeepEqual(got.Muted, []string{muted}) || len(got.Following) != 0 {
		t.Errorf("RelationsAmong = %+v; want blocked %s, blocked by %s, muted %s", got, blocked, blocker, muted)
	}
	if got, err := r.repo.RelationsAmong(ctx, viewer, nil, at); err != nil || !reflect.DeepEqual(got, appgraph.RelationsAmong{}) {
		t.Errorf("RelationsAmong(no accounts) = %+v, %v; want nothing", got, err)
	}
}

// blockRepoLockPair races transactions that each lock a pair, in either order, read whether it
// has a block, and insert one when it has none: the lock makes each see the block of the one
// before it, so exactly one block is inserted. This holds on PostgreSQL only through the lock, on
// read committed, and is what keeps a follow and a block of one pair from missing each other.
func blockRepoLockPair(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	for round := range 3 {
		ids := r.accounts(2)
		a, b := ids[0], ids[1]
		var wg sync.WaitGroup
		for i := range 6 {
			wg.Go(func() {
				first, second := a, b
				if i%2 == 1 {
					first, second = b, a
				}
				err := r.uow.Tx(ctx, func(tx uow.Tx) error {
					if err := r.repo.LockPair(ctx, tx, first, second); err != nil {
						return err
					}
					histories, err := r.repo.ListBlockHistories(ctx, tx)
					if err != nil {
						return err
					}
					for _, h := range histories {
						if h.Block.BlockerAccountID == a && h.Block.BlockedAccountID == b {
							return nil
						}
					}
					id := r.ids.NewID()
					return r.repo.InsertBlock(ctx, tx, graph.Block{
						ID: id, URI: graph.BlockURI(testkit.ContractHost, id), BlockerAccountID: a, BlockedAccountID: b, CreatedAt: r.clock.Now(),
					})
				})
				if err != nil {
					t.Errorf("round %d: transaction = %v", round, err)
				}
			})
		}
		wg.Wait()
		var n int
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			histories, err := r.repo.ListBlockHistories(ctx, tx)
			for _, h := range histories {
				if h.Block.BlockerAccountID == a && h.Block.BlockedAccountID == b {
					n++
				}
			}
			return err
		})
		if n != 1 {
			t.Errorf("round %d: %d blocks of the pair, want 1", round, n)
		}
	}
}

// blockRepoRelationshipParts reads the relationships of one account with several others at once:
// follows both ways in either state with the preferences of the one out, blocks both ways, a mute
// in force and an expired one, and an account with none; each part is what the read of one pair
// finds.
// blockRepoOneWithMany reads, in a transaction, the blocks of one account with many others by
// direction, and its notification relations with many sources, as NotificationRelationsTx reads
// those of many recipients with one source.
func blockRepoOneWithMany(t *testing.T, r *blockRepo) {
	ids := r.accounts(5)
	me, bob, carol, dave, erin := ids[0], ids[1], ids[2], ids[3], ids[4]
	now := r.clock.Now()
	r.block(t, me, carol)
	r.block(t, dave, me)
	r.mute(t, graph.Mute{AccountID: me, TargetAccountID: bob, HideNotifications: true, CreatedAt: now})
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.repo.InsertEdge(ctx, tx, graph.Edge{FollowerAccountID: me, FolloweeAccountID: erin, FollowID: r.ids.NewID(), State: graph.StateActive, Since: now})
		return err
	})
	// The mutes of one target by many accounts, and the preferences of many follows, outside a
	// transaction.
	if got, err := r.repo.FindMutesOn(t.Context(), bob, []string{me, carol}, now); err != nil || len(got) != 1 || !got[me].HideNotifications {
		t.Errorf("FindMutesOn(bob) = %+v, %v; want my mute", got, err)
	}
	if got, err := r.repo.FindPreferencesOf(t.Context(), []string{r.ids.NewID()}); err != nil || len(got) != 0 {
		t.Errorf("FindPreferencesOf(unknown) = %+v, %v; want none", got, err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		blocking, blockedBy, err := r.repo.BlockEdgesWithTx(ctx, tx, me, []string{bob, carol, dave, erin})
		if err != nil || !maps.Equal(blocking, map[string]bool{carol: true}) || !maps.Equal(blockedBy, map[string]bool{dave: true}) {
			t.Errorf("BlockEdgesWithTx = %v, %v, %v; want carol blocked, by dave", blocking, blockedBy, err)
		}
		got, err := r.repo.NotificationRelationsFromTx(ctx, tx, me, []string{bob, carol, dave, erin}, now)
		if err != nil {
			return err
		}
		if !maps.Equal(got.Blocked, map[string]bool{carol: true, dave: true}) {
			t.Errorf("NotificationRelationsFromTx blocked = %v, want carol and dave", got.Blocked)
		}
		if len(got.Mutes) != 1 || !got.Mutes[bob].HideNotifications {
			t.Errorf("NotificationRelationsFromTx mutes = %+v, want my mute of bob", got.Mutes)
		}
		if len(got.Follows) != 1 || got.Follows[erin].State != graph.StateActive {
			t.Errorf("NotificationRelationsFromTx follows = %+v, want my follow of erin", got.Follows)
		}
		// One source seen from the other side reads as NotificationRelationsTx reads it.
		for _, source := range []string{bob, carol, dave, erin} {
			one, err := r.repo.NotificationRelationsTx(ctx, tx, source, []string{me}, now)
			if err != nil {
				return err
			}
			if one.Blocked[me] != got.Blocked[source] || one.Mutes[me] != got.Mutes[source] || one.Follows[me] != got.Follows[source] {
				t.Errorf("the relations of me with %s = %+v, want %+v as NotificationRelationsTx reads them", source, got, one)
			}
		}
		return nil
	})
}

// blockRepoWritesMany inserts many blocks and their edges, and upserts many mutes of one target,
// each read as the writes of one record it.
func blockRepoWritesMany(t *testing.T, r *blockRepo) {
	ids := r.accounts(4)
	target, a, b, c := ids[0], ids[1], ids[2], ids[3]
	now := r.clock.Now()
	var bs []graph.Block
	for _, blocker := range []string{a, b} {
		id := r.ids.NewID()
		bs = append(bs, graph.Block{ID: id, URI: graph.BlockURI(testkit.ContractHost, id), BlockerAccountID: blocker, BlockedAccountID: target, CreatedAt: now})
	}
	later := now.Add(time.Hour)
	ms := []graph.Mute{
		{AccountID: a, TargetAccountID: target, HideNotifications: true, CreatedAt: now},
		{AccountID: c, TargetAccountID: target, ExpiresAt: &later, CreatedAt: now},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if err := errors.Join(r.repo.InsertBlocks(ctx, tx, bs), r.repo.InsertBlocks(ctx, tx, nil)); err != nil {
			return err
		}
		if n, err := r.repo.InsertBlockEdges(ctx, tx, bs); err != nil || n != 2 {
			t.Errorf("InsertBlockEdges = %d, %v; want 2", n, err)
		}
		if n, err := r.repo.InsertBlockEdges(ctx, tx, bs[:1]); err != nil || n != 0 {
			t.Errorf("InsertBlockEdges of a pair with an edge = %d, %v; want 0", n, err)
		}
		for _, want := range bs {
			if got, err := r.repo.Block(ctx, tx, want.ID); err != nil || got.URI != want.URI || got.BlockerAccountID != want.BlockerAccountID || !got.CreatedAt.Equal(now) {
				t.Errorf("Block(%s) = %+v, %v; want %+v", want.ID, got, err, want)
			}
			if e, err := r.repo.BlockEdge(ctx, tx, want.BlockerAccountID, target); err != nil || e.BlockID != want.ID {
				t.Errorf("BlockEdge(%s) = %+v, %v", want.BlockerAccountID, e, err)
			}
		}
		if err := errors.Join(r.repo.UpsertMutes(ctx, tx, ms), r.repo.UpsertMutes(ctx, tx, nil)); err != nil {
			return err
		}
		got, err := r.repo.ActiveMutesOfTargetTx(ctx, tx, target, []string{a, b, c}, now)
		if err != nil || len(got) != 2 {
			t.Errorf("ActiveMutesOfTargetTx = %+v, %v; want the mutes of a and c", got, err)
		}
		for _, want := range ms {
			one, err := r.repo.Mute(ctx, tx, want.AccountID, target, now)
			if err != nil || one.HideNotifications != want.HideNotifications || (one.ExpiresAt == nil) != (want.ExpiresAt == nil) ||
				(want.ExpiresAt != nil && !one.ExpiresAt.Equal(*want.ExpiresAt)) || !reflect.DeepEqual(got[want.AccountID], one) {
				t.Errorf("the mute of %s = %+v / %+v, %v; want %+v", want.AccountID, one, got[want.AccountID], err, want)
			}
		}
		if got, err := r.repo.ActiveMutesOfTargetTx(ctx, tx, target, []string{c}, later.Add(time.Second)); err != nil || len(got) != 0 {
			t.Errorf("ActiveMutesOfTargetTx after the expiry = %+v, %v; want none", got, err)
		}
		return nil
	})
}

func blockRepoRelationshipParts(t *testing.T, r *blockRepo) {
	ctx := t.Context()
	ids := r.accounts(5)
	me, bob, carol, dave, erin := ids[0], ids[1], ids[2], ids[3], ids[4]
	now := r.clock.Now()
	past := now.Add(-time.Hour)
	edges := []graph.Edge{
		{FollowerAccountID: me, FolloweeAccountID: bob, FollowID: r.ids.NewID(), State: graph.StateActive, Since: now},
		{FollowerAccountID: me, FolloweeAccountID: carol, FollowID: r.ids.NewID(), State: graph.StatePending, Since: now},
		{FollowerAccountID: bob, FolloweeAccountID: me, FollowID: r.ids.NewID(), State: graph.StateActive, Since: now},
		{FollowerAccountID: carol, FolloweeAccountID: bob, FollowID: r.ids.NewID(), State: graph.StateActive, Since: now}, // not mine
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, e := range edges {
			if _, err := r.repo.InsertEdge(ctx, tx, e); err != nil {
				return err
			}
		}
		return r.repo.UpsertPreferences(ctx, tx, graph.Preferences{FollowID: edges[0].FollowID, ShowBoosts: false})
	})
	r.block(t, me, carol)
	r.block(t, dave, me)
	r.mute(t, graph.Mute{AccountID: me, TargetAccountID: bob, HideNotifications: true, CreatedAt: now})
	r.mute(t, graph.Mute{AccountID: me, TargetAccountID: dave, ExpiresAt: &past, CreatedAt: past.Add(-time.Hour)})
	r.mute(t, graph.Mute{AccountID: bob, TargetAccountID: me, HideNotifications: true, CreatedAt: now})

	parts, err := r.repo.FindRelationshipParts(ctx, me, []string{bob, carol, dave, erin}, now)
	if err != nil {
		t.Fatalf("FindRelationshipParts = %v", err)
	}
	for _, other := range []string{bob, carol, dave, erin} {
		check := func(part string, got any, ok bool, want any, err error) {
			t.Helper()
			switch {
			case errors.Is(err, appgraph.ErrNotFound):
				if ok {
					t.Errorf("FindRelationshipParts: %s with %s = %+v, want none", part, other, got)
				}
			case err != nil:
				t.Fatal(err)
			case !ok || !reflect.DeepEqual(got, want):
				t.Errorf("FindRelationshipParts: %s with %s = %+v (%t), want %+v", part, other, got, ok, want)
			}
		}
		e, ok := parts.Following[other]
		want, err := r.repo.FindEdge(ctx, me, other)
		check("follow out", e, ok, want, err)
		if err == nil {
			p, ok := parts.Preferences[want.FollowID]
			wantP, err := r.repo.FindPreferences(ctx, want.FollowID)
			check("preferences", p, ok, wantP, err)
		}
		e, ok = parts.FollowedBy[other]
		want, err = r.repo.FindEdge(ctx, other, me)
		check("follow in", e, ok, want, err)
		b, ok := parts.Blocking[other]
		wantB, err := r.repo.FindBlockEdge(ctx, me, other)
		check("block out", b, ok, wantB, err)
		b, ok = parts.BlockedBy[other]
		wantB, err = r.repo.FindBlockEdge(ctx, other, me)
		check("block in", b, ok, wantB, err)
		m, ok := parts.Mutes[other]
		wantM, err := r.repo.FindMute(ctx, me, other, now)
		check("mute", m, ok, wantM, err)
	}
	if _, ok := parts.Mutes[dave]; ok {
		t.Errorf("FindRelationshipParts: the expired mute of dave is in force")
	}
	if empty, err := r.repo.FindRelationshipParts(ctx, me, nil, now); err != nil || len(empty.Following) != 0 {
		t.Errorf("FindRelationshipParts(none) = %+v, %v; want nothing", empty, err)
	}

	// The relations of the recipients of a notification from me, read in a transaction.
	var rels appgraph.NotificationRelations
	r.tx(t, func(ctx context.Context, tx uow.Tx) (err error) {
		rels, err = r.repo.NotificationRelationsTx(ctx, tx, me, []string{bob, carol, dave, erin}, now)
		return err
	})
	for _, other := range []string{bob, carol, dave, erin} {
		_, out := r.repo.FindBlockEdge(ctx, me, other)
		_, in := r.repo.FindBlockEdge(ctx, other, me)
		if blocked := out == nil || in == nil; rels.Blocked[other] != blocked {
			t.Errorf("NotificationRelationsTx: blocked with %s = %v, want %v", other, rels.Blocked[other], blocked)
		}
		m, ok := rels.Mutes[other]
		wantM, err := r.repo.FindMute(ctx, other, me, now)
		if (err == nil) != ok || ok && !reflect.DeepEqual(m, wantM) {
			t.Errorf("NotificationRelationsTx: mute of %s = %+v (%t), want %+v (%v)", other, m, ok, wantM, err)
		}
		e, ok := rels.Follows[other]
		wantE, err := r.repo.FindEdge(ctx, other, me)
		if (err == nil) != ok || ok && e != wantE {
			t.Errorf("NotificationRelationsTx: follow of %s = %+v (%t), want %+v (%v)", other, e, ok, wantE, err)
		}
	}
	if !rels.Blocked[carol] || !rels.Blocked[dave] || rels.Mutes[bob].AccountID != bob || rels.Follows[bob].State != graph.StateActive {
		t.Errorf("NotificationRelationsTx = %+v; want carol and dave blocked, bob muting and following me", rels)
	}
}
