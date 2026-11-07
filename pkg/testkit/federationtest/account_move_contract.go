// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/pkg/testkit"
)

// AccountMoveContract verifies the repository methods that the moves of accounts use on a database
// adapter, by those methods alone, the port contract of ADR-0153 §1: a move is recorded once per
// account and target, found by ID and as the newest of its account, and canceled once; the moves
// are listed with their cancellations in the order of their accounts, times, and IDs; the moves in
// force are upserted, found, listed, and deleted; the aliases of an account are recorded once,
// counted, listed oldest first, and removed; and the follows, blocks, and mutes that the migration
// moves are listed in their orders and pages. The use cases on top of it are AccountMoveFlows,
// which internal/flowtest/federation runs once on SQLite.
func AccountMoveContract(t *testing.T, newHarness RemoteFollowFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, r *moveRepo)
	}{
		{"RecordsAndCancelsMovesOnce", moveRepoMoves},
		{"KeepsTheMovesInForce", moveRepoStates},
		{"KeepsAliases", moveRepoAliases},
		{"ListsTheRelationshipsToMigrate", moveRepoRelationships},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, uow.Options{})
			clock := testkit.NewClock(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC))
			tc.run(t, &moveRepo{h: h, clock: clock, ids: testkit.NewIDGenerator(clock)})
		})
	}
}

type moveRepo struct {
	h     RemoteFollowHarness
	clock *testkit.Clock
	ids   *testkit.IDGenerator
}

func (r *moveRepo) tx(t *testing.T, fn func(ctx context.Context, tx uow.Tx) error) {
	t.Helper()
	if err := r.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error { return fn(t.Context(), tx) }); err != nil {
		t.Fatal(err)
	}
}

// account inserts a remote account named name and returns its ID.
func (r *moveRepo) account(t *testing.T, name string) string {
	t.Helper()
	a := account.Account{
		ID: r.ids.NewID(), URI: "https://remote.example/users/" + name, Domain: "remote.example",
		ActorType: account.ActorPerson, CreatedAt: r.clock.Now(),
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.InsertAccount(ctx, tx, a) })
	return a.ID
}

// insertMove records a remote move of from to to now and reports whether it recorded one.
func (r *moveRepo) insertMove(t *testing.T, from, to string) (account.Move, bool) {
	t.Helper()
	m := account.Move{
		ID: r.ids.NewID(), AccountID: from, TargetAccountID: to, InitiatedBy: account.MoveInitiatedByRemote,
		ActivityURI: "https://remote.example/moves/" + r.ids.NewID(), OccurredAt: r.clock.Now(),
	}
	var ok bool
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		ok, err = r.h.Accounts.InsertMove(ctx, tx, m)
		return err
	})
	r.clock.Advance(time.Second)
	return m, ok
}

func moveRepoMoves(t *testing.T, r *moveRepo) {
	ctx := t.Context()
	alice, bob, carol, dave := r.account(t, "alice"), r.account(t, "bob"), r.account(t, "carol"), r.account(t, "dave")
	first, ok := r.insertMove(t, alice, bob)
	if !ok {
		t.Fatal("InsertMove = false for a new move")
	}
	if _, ok := r.insertMove(t, alice, bob); ok {
		t.Error("InsertMove to the same target again = true, want false")
	}
	second, _ := r.insertMove(t, alice, carol)
	self := account.Move{ID: r.ids.NewID(), AccountID: dave, TargetAccountID: bob, InitiatedBy: account.MoveInitiatedBySelf, OccurredAt: r.clock.Now()}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		_, err := r.h.Accounts.InsertMove(ctx, tx, self)
		return err
	})

	for _, want := range []account.Move{first, second, self} {
		if got, err := r.h.Accounts.FindMove(ctx, want.ID); err != nil || got != want {
			t.Errorf("FindMove(%s) = %+v, %v; want %+v", want.ID, got, err, want)
		}
	}
	if _, err := r.h.Accounts.FindMove(ctx, r.ids.NewID()); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindMove(unknown) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.FindMoveTx(ctx, tx, second.ID); err != nil || got != second {
			t.Errorf("FindMoveTx = %+v, %v; want %+v", got, err, second)
		}
		if got, err := r.h.Accounts.LatestMove(ctx, tx, alice); err != nil || got != second {
			t.Errorf("LatestMove(alice) = %+v, %v; want the newer move %+v", got, err, second)
		}
		if _, err := r.h.Accounts.LatestMove(ctx, tx, bob); !errors.Is(err, appaccount.ErrNotFound) {
			t.Errorf("LatestMove(an account that never moved) = %v, want ErrNotFound", err)
		}
		return nil
	})

	cancel := account.MoveCancellation{MoveID: first.ID, CanceledAt: r.clock.Now()}
	for i, want := range []bool{true, false} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Accounts.InsertMoveCancellation(ctx, tx, cancel)
			if err == nil && ok != want {
				t.Errorf("InsertMoveCancellation #%d = %v, want %v", i+1, ok, want)
			}
			return err
		})
	}

	var records []account.MoveRecord
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		var err error
		records, err = r.h.Accounts.ListMoveRecords(ctx, tx)
		return err
	})
	want := []account.MoveRecord{{Move: first, Canceled: true}, {Move: second}, {Move: self}}
	if !slices.Equal(records, want) {
		t.Errorf("ListMoveRecords =\n%+v\nwant\n%+v", records, want)
	}
}

func moveRepoStates(t *testing.T, r *moveRepo) {
	ctx := t.Context()
	alice, bob, carol := r.account(t, "alice"), r.account(t, "bob"), r.account(t, "carol")
	toBob, _ := r.insertMove(t, alice, bob)
	toCarol, _ := r.insertMove(t, alice, carol)
	bobMove, _ := r.insertMove(t, bob, carol)

	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		return errors.Join(
			r.h.Accounts.UpsertMoveState(ctx, tx, account.StateOf(toBob)),
			r.h.Accounts.UpsertMoveState(ctx, tx, account.StateOf(toCarol)),
			r.h.Accounts.UpsertMoveState(ctx, tx, account.StateOf(bobMove)),
		)
	})
	if got, err := r.h.Accounts.FindMoveState(ctx, alice); err != nil || got != account.StateOf(toCarol) {
		t.Errorf("FindMoveState(alice) = %+v, %v; want the upserted %+v", got, err, account.StateOf(toCarol))
	}
	if _, err := r.h.Accounts.FindMoveState(ctx, carol); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindMoveState(an account that never moved) = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Accounts.FindMoveStateTx(ctx, tx, bob); err != nil || got != account.StateOf(bobMove) {
			t.Errorf("FindMoveStateTx(bob) = %+v, %v; want %+v", got, err, account.StateOf(bobMove))
		}
		return nil
	})
	states, err := r.h.Accounts.ListMoveStates(ctx)
	if want := []account.MoveState{account.StateOf(toCarol), account.StateOf(bobMove)}; err != nil || !slices.Equal(states, want) {
		t.Errorf("ListMoveStates = %+v, %v; want %+v in account order", states, err, want)
	}

	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.DeleteMoveState(ctx, tx, alice) })
	if _, err := r.h.Accounts.FindMoveState(ctx, alice); !errors.Is(err, appaccount.ErrNotFound) {
		t.Errorf("FindMoveState after DeleteMoveState = %v, want ErrNotFound", err)
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error { return r.h.Accounts.DeleteAllMoveStates(ctx, tx) })
	if states, err := r.h.Accounts.ListMoveStates(ctx); err != nil || len(states) != 0 {
		t.Errorf("ListMoveStates after DeleteAllMoveStates = %+v, %v; want none", states, err)
	}
	if _, err := r.h.Accounts.FindMove(ctx, toBob.ID); err != nil {
		t.Errorf("FindMove after DeleteAllMoveStates = %v, want the move kept", err)
	}
}

func moveRepoAliases(t *testing.T, r *moveRepo) {
	ctx := t.Context()
	alice, bob := r.account(t, "alice"), r.account(t, "bob")
	const (
		oldURI   = "https://old.example/users/alice"
		otherURI = "https://other.example/users/alice"
	)
	insert := func(a account.Alias) bool {
		var ok bool
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			var err error
			ok, err = r.h.Accounts.InsertAlias(ctx, tx, a)
			return err
		})
		r.clock.Advance(time.Second)
		return ok
	}
	first := account.Alias{AccountID: alice, URI: otherURI, AddedAt: r.clock.Now()}
	if !insert(first) {
		t.Fatal("InsertAlias = false for a new alias")
	}
	second := account.Alias{AccountID: alice, URI: oldURI, AddedAt: r.clock.Now()}
	insert(second)
	if insert(account.Alias{AccountID: alice, URI: oldURI, AddedAt: r.clock.Now()}) {
		t.Error("InsertAlias of an alias the account has = true, want false")
	}
	insert(account.Alias{AccountID: bob, URI: oldURI, AddedAt: r.clock.Now()})

	if got, err := r.h.Accounts.ListAliases(ctx, alice); err != nil || !slices.Equal(got, []account.Alias{first, second}) {
		t.Errorf("ListAliases(alice) = %+v, %v; want %+v oldest first", got, err, []account.Alias{first, second})
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if n, err := r.h.Accounts.CountAliases(ctx, tx, alice); err != nil || n != 2 {
			t.Errorf("CountAliases(alice) = %d, %v; want 2", n, err)
		}
		if ok, err := r.h.Accounts.HasAliasTx(ctx, tx, alice, oldURI); err != nil || !ok {
			t.Errorf("HasAliasTx(alice, old) = %v, %v; want true", ok, err)
		}
		return nil
	})
	if ok, err := r.h.Accounts.HasAlias(ctx, alice, "https://nowhere.example/users/alice"); err != nil || ok {
		t.Errorf("HasAlias(an unknown URI) = %v, %v; want false", ok, err)
	}

	for i, want := range []bool{true, false} {
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			ok, err := r.h.Accounts.DeleteAlias(ctx, tx, alice, oldURI)
			if err == nil && ok != want {
				t.Errorf("DeleteAlias #%d = %v, want %v", i+1, ok, want)
			}
			return err
		})
	}
	if ok, err := r.h.Accounts.HasAlias(ctx, alice, oldURI); err != nil || ok {
		t.Errorf("HasAlias after DeleteAlias = %v, %v; want false", ok, err)
	}
	if ok, err := r.h.Accounts.HasAlias(ctx, bob, oldURI); err != nil || !ok {
		t.Errorf("HasAlias of another account with the URI = %v, %v; want true", ok, err)
	}
}

func moveRepoRelationships(t *testing.T, r *moveRepo) {
	ctx := t.Context()
	source := r.account(t, "source")
	followers := []string{r.account(t, "f1"), r.account(t, "f2"), r.account(t, "f3"), r.account(t, "pending")}
	var edges []graph.Edge
	for i, f := range followers {
		follow := graph.Follow{
			ID: r.ids.NewID(), URI: "https://remote.example/follows/" + r.ids.NewID(),
			FollowerAccountID: f, FolloweeAccountID: source, RequestedAt: r.clock.Now(),
		}
		edge := graph.Edge{FollowerAccountID: f, FolloweeAccountID: source, FollowID: follow.ID, State: graph.StateActive, Since: r.clock.Now()}
		if i == len(followers)-1 {
			edge.State = graph.StatePending
		}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := r.h.Graph.LockPair(ctx, tx, f, source); err != nil {
				return err
			}
			if err := r.h.Graph.InsertFollow(ctx, tx, follow); err != nil {
				return err
			}
			_, err := r.h.Graph.InsertEdge(ctx, tx, edge)
			return err
		})
		edges = append(edges, edge)
		r.clock.Advance(time.Second)
	}
	got, err := r.h.Graph.ListFollowers(ctx, source, graph.StateActive, "", 2)
	if want := []graph.Edge{edges[2], edges[1]}; err != nil || !slices.Equal(got, want) {
		t.Errorf("ListFollowers(active, first page) = %+v, %v; want %+v newest first", got, err, want)
	}
	got, err = r.h.Graph.ListFollowers(ctx, source, graph.StateActive, edges[1].FollowID, 2)
	if want := []graph.Edge{edges[0]}; err != nil || !slices.Equal(got, want) {
		t.Errorf("ListFollowers(active, after the second) = %+v, %v; want %+v", got, err, want)
	}
	got, err = r.h.Graph.ListFollowers(ctx, source, graph.StatePending, "", 10)
	if want := []graph.Edge{edges[3]}; err != nil || !slices.Equal(got, want) {
		t.Errorf("ListFollowers(pending) = %+v, %v; want %+v", got, err, want)
	}

	var blocks []graph.BlockEdge
	for _, blocker := range followers[:3] {
		b := graph.Block{ID: r.ids.NewID(), BlockerAccountID: blocker, BlockedAccountID: source, CreatedAt: r.clock.Now()}
		be := graph.BlockEdge{BlockerAccountID: blocker, BlockedAccountID: source, BlockID: b.ID}
		r.tx(t, func(ctx context.Context, tx uow.Tx) error {
			if err := r.h.Graph.InsertBlock(ctx, tx, b); err != nil {
				return err
			}
			_, err := r.h.Graph.InsertBlockEdge(ctx, tx, be)
			return err
		})
		blocks = append(blocks, be)
		r.clock.Advance(time.Second)
	}
	gotBlocks, err := r.h.Graph.ListBlockers(ctx, source, "", 2)
	if want := blocks[:2]; err != nil || !slices.Equal(gotBlocks, want) {
		t.Errorf("ListBlockers(first page) = %+v, %v; want %+v in block order", gotBlocks, err, want)
	}
	gotBlocks, err = r.h.Graph.ListBlockers(ctx, source, blocks[1].BlockID, 2)
	if want := blocks[2:]; err != nil || !slices.Equal(gotBlocks, want) {
		t.Errorf("ListBlockers(after the second) = %+v, %v; want %+v", gotBlocks, err, want)
	}

	now := r.clock.Now()
	expired := now.Add(-time.Second)
	later := now.Add(time.Hour)
	mutes := []graph.Mute{
		{AccountID: followers[0], TargetAccountID: source, HideNotifications: true, CreatedAt: now},
		{AccountID: followers[1], TargetAccountID: source, ExpiresAt: &later, CreatedAt: now},
		{AccountID: followers[2], TargetAccountID: source, ExpiresAt: &expired, CreatedAt: now.Add(-time.Hour)},
		{AccountID: followers[3], TargetAccountID: source, CreatedAt: now},
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		for _, m := range mutes {
			if err := r.h.Graph.UpsertMute(ctx, tx, m); err != nil {
				return err
			}
		}
		return nil
	})
	sortedMuters := []graph.Mute{mutes[0], mutes[1], mutes[3]}
	slices.SortFunc(sortedMuters, func(a, b graph.Mute) int { return strings.Compare(a.AccountID, b.AccountID) })
	gotMutes, err := r.h.Graph.ListMuters(ctx, source, now, "", 2)
	if err != nil || !equalMutes(gotMutes, sortedMuters[:2]) {
		t.Errorf("ListMuters(first page) = %+v, %v; want %+v, without the expired mute", gotMutes, err, sortedMuters[:2])
	}
	gotMutes, err = r.h.Graph.ListMuters(ctx, source, now, sortedMuters[1].AccountID, 2)
	if err != nil || !equalMutes(gotMutes, sortedMuters[2:]) {
		t.Errorf("ListMuters(after the second) = %+v, %v; want %+v", gotMutes, err, sortedMuters[2:])
	}
	r.tx(t, func(ctx context.Context, tx uow.Tx) error {
		if got, err := r.h.Graph.Mute(ctx, tx, followers[1], source, now); err != nil || !equalMutes([]graph.Mute{got}, mutes[1:2]) {
			t.Errorf("Mute(in force) = %+v, %v; want %+v", got, err, mutes[1])
		}
		if _, err := r.h.Graph.Mute(ctx, tx, followers[2], source, now); !errors.Is(err, appgraph.ErrNotFound) {
			t.Errorf("Mute(expired) = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// equalMutes compares mutes by their fields and the instants of their expiries.
func equalMutes(got, want []graph.Mute) bool {
	return slices.EqualFunc(got, want, func(a, b graph.Mute) bool {
		if (a.ExpiresAt == nil) != (b.ExpiresAt == nil) || (a.ExpiresAt != nil && !a.ExpiresAt.Equal(*b.ExpiresAt)) {
			return false
		}
		return a.AccountID == b.AccountID && a.TargetAccountID == b.TargetAccountID &&
			a.HideNotifications == b.HideNotifications && a.CreatedAt.Equal(b.CreatedAt)
	})
}
