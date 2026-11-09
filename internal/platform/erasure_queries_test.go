// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// erasureQueries counts the round trips of the erasure of alice, deleted with n local accounts
// that follow her and that she follows.
func erasureQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	names := []string{"alice"}
	for i := range n {
		names = append(names, fmt.Sprintf("fan%d", i))
	}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{Tracer: tracer, Clock: clock}, names...)
	ctx := t.Context()
	alice := c.ids["alice"]
	for i := range n {
		fan := c.ids[fmt.Sprintf("fan%d", i)]
		if _, err := c.app.Graph.Follow(ctx, fan, alice); err != nil {
			t.Fatal(err)
		}
		if _, err := c.app.Graph.Follow(ctx, alice, fan); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.app.Identity.DeleteOwnAccount(ctx, c.userIDs["alice"], identity.Confirmation{Password: "correct horse battery staple"}); err != nil {
		t.Fatalf("DeleteOwnAccount: %v", err)
	}
	handler, _, _ := c.app.Handlers.Lookup(appaccount.JobEraseDeletedAccounts)
	clock.ahead.Store(int64(appaccount.ErasureDelay + time.Minute))
	was := ops(tracer)
	if res := handler(ctx, queue.Job{Type: appaccount.JobEraseDeletedAccounts}); res.Err() != nil {
		t.Fatalf("erasure: %v", res.Err())
	}
	if _, err := c.app.Runner.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	count, names := opsSince(tracer, was)
	t.Logf("erasing an account with %d followers and followees: %d queries: %v", n, count, names)
	for i := range n {
		fan := c.ids[fmt.Sprintf("fan%d", i)]
		if got, err := c.app.Graph.Counters(ctx, fan); err != nil || got.FollowersCount != 0 || got.FollowingCount != 0 {
			t.Errorf("the counters of fan%d after the erasure = %+v, %v; want none", i, got, err)
		}
		if r, err := c.app.Graph.Relationship(ctx, fan, alice); err != nil || r.Following != graph.StateNone {
			t.Errorf("fan%d's follow of alice after the erasure = %+v, %v; want none", i, r, err)
		}
	}
	return count
}

// TestErasureQueries checks that the round trips of the erasure of a deleted account do not
// grow with its follows: the live follows end together.
func TestErasureQueries(t *testing.T) {
	t.Parallel()
	small, large := erasureQueries(t, 2), erasureQueries(t, 8)
	if large > small {
		t.Errorf("the erasure makes %d queries for 8 followers, more than the %d for 2", large, small)
	}
}
