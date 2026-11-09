// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/graph"
	domnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// migrateQueries counts the round trips of the first page of the migration of the follows of
// alice, who moved to bob, with n local followers.
func migrateQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	names := []string{"alice", "bob"}
	for i := range n {
		names = append(names, fmt.Sprintf("fan%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, names...)
	ctx := t.Context()
	alice, bob := c.ids["alice"], c.ids["bob"]
	for i := range n {
		if _, err := c.app.Graph.Follow(ctx, c.ids[fmt.Sprintf("fan%d", i)], alice); err != nil {
			t.Fatal(err)
		}
	}
	move, err := c.app.Accounts.RecordLocalMove(ctx, appaccount.LocalMoveInput{AccountID: alice, TargetAccountID: bob})
	if err != nil {
		t.Fatal(err)
	}
	was := ops(tracer)
	if _, _, err := c.app.Graph.MigrateStep(ctx, appgraph.MigrationStep{MoveID: move.ID, SourceAccountID: alice, TargetAccountID: bob}); err != nil {
		t.Fatalf("MigrateStep: %v", err)
	}
	count, names := opsSince(tracer, was)
	t.Logf("migrating %d local followers: %d queries: %v", n, count, names)
	for i := range n {
		fan := c.ids[fmt.Sprintf("fan%d", i)]
		if r, err := c.app.Graph.Relationship(ctx, fan, bob); err != nil || r.Following != graph.StateActive {
			t.Errorf("fan%d's follow of bob = %+v, %v; want active", i, r, err)
		}
		if r, err := c.app.Graph.Relationship(ctx, fan, alice); err != nil || r.Following != graph.StateNone {
			t.Errorf("fan%d's follow of alice = %+v, %v; want none", i, r, err)
		}
	}
	// Each follower is told of the move, and bob of each new follower; the counters follow.
	for i := range n {
		fan := c.ids[fmt.Sprintf("fan%d", i)]
		ns, err := c.app.Notifications.List(ctx, fan, appnotification.Query{Types: []domnotification.Type{domnotification.TypeAccountMoved}})
		if err != nil || len(ns) != 1 || ns[0].AccountMoveOutcome != domnotification.MoveOutcome(appgraph.MigrationFollowed) {
			t.Errorf("fan%d's account_moved notifications = %+v, %v; want one, followed", i, ns, err)
		}
	}
	if ns, err := c.app.Notifications.List(ctx, bob, appnotification.Query{Types: []domnotification.Type{domnotification.TypeFollow}}); err != nil || len(ns) != n {
		t.Errorf("bob has %d follow notifications, %v; want %d", len(ns), err, n)
	}
	for id, want := range map[string]int64{alice: 0, bob: int64(n)} {
		if got, err := c.app.Graph.Counters(ctx, id); err != nil || got.FollowersCount != want {
			t.Errorf("the followers of %s after the migration = %+v, %v; want %d", id, got, err, want)
		}
	}
	return count
}

// TestMigrateQueries checks that the round trips of a page of the migration of a moved account's
// followers do not grow with them.
func TestMigrateQueries(t *testing.T) {
	t.Parallel()
	small, large := migrateQueries(t, 2), migrateQueries(t, 8)
	if large > small {
		t.Errorf("a page of the migration makes %d queries for 8 followers, more than the %d for 2", large, small)
	}
}

// migrateBlocksQueries counts the round trips of the blocks phase and of the mutes phase of the
// migration of alice, who moved to bob, with n local accounts that block her and n that mute her.
func migrateBlocksQueries(t *testing.T, n int) (blocks, mutes int) {
	t.Helper()
	tracer := testkit.NewTracer()
	names := []string{"alice", "bob"}
	for i := range n {
		names = append(names, fmt.Sprintf("blocker%d", i), fmt.Sprintf("muter%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, names...)
	ctx := t.Context()
	alice, bob := c.ids["alice"], c.ids["bob"]
	for i := range n {
		blocker, muter := c.ids[fmt.Sprintf("blocker%d", i)], c.ids[fmt.Sprintf("muter%d", i)]
		if _, err := c.app.Graph.Follow(ctx, blocker, bob); err != nil {
			t.Fatal(err)
		}
		if _, err := c.app.Graph.Block(ctx, blocker, alice); err != nil {
			t.Fatal(err)
		}
		if _, err := c.app.Graph.Mute(ctx, muter, alice, appgraph.MuteInput{HideNotifications: i%2 == 0}); err != nil {
			t.Fatal(err)
		}
	}
	move, err := c.app.Accounts.RecordLocalMove(ctx, appaccount.LocalMoveInput{AccountID: alice, TargetAccountID: bob})
	if err != nil {
		t.Fatal(err)
	}
	run := func(phase appgraph.MigrationPhase) int {
		t.Helper()
		was := ops(tracer)
		if _, _, err := c.app.Graph.MigrateStep(ctx, appgraph.MigrationStep{MoveID: move.ID, SourceAccountID: alice, TargetAccountID: bob, Phase: phase}); err != nil {
			t.Fatalf("MigrateStep(%s): %v", phase, err)
		}
		count, names := opsSince(tracer, was)
		t.Logf("the %s phase with %d accounts: %d queries: %v", phase, n, count, names)
		return count
	}
	blocks, mutes = run(appgraph.MigrateBlocksPhase), run(appgraph.MigrateMutesPhase)
	for i := range n {
		blocker, muter := c.ids[fmt.Sprintf("blocker%d", i)], c.ids[fmt.Sprintf("muter%d", i)]
		if r, err := c.app.Graph.Relationship(ctx, blocker, bob); err != nil || !r.Blocking || r.Following != graph.StateNone {
			t.Errorf("blocker%d and bob = %+v, %v; want a block and no follow", i, r, err)
		}
		if r, err := c.app.Graph.Relationship(ctx, muter, bob); err != nil || !r.Muting {
			t.Errorf("muter%d and bob = %+v, %v; want a mute", i, r, err)
		}
	}
	return blocks, mutes
}

// TestMigrateBlocksQueries checks that the round trips of a page of the blocks and of the mutes
// phases of a migration do not grow with their accounts.
func TestMigrateBlocksQueries(t *testing.T) {
	t.Parallel()
	smallB, smallM := migrateBlocksQueries(t, 2)
	largeB, largeM := migrateBlocksQueries(t, 6)
	if largeB > smallB {
		t.Errorf("a page of the blocks phase makes %d queries for 6 blockers, more than the %d for 2", largeB, smallB)
	}
	if largeM > smallM {
		t.Errorf("a page of the mutes phase makes %d queries for 6 muters, more than the %d for 2", largeM, smallM)
	}
}
