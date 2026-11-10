// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// refreshActorsQueries counts the database round trips of one run of JobRefreshActors over n
// stale remote actors, half of them followed by alice, and returns them with the refresh jobs the
// run enqueued.
func refreshActorsQueries(t *testing.T, n int) (int, int) {
	t.Helper()
	tracer := testkit.NewTracer()
	clock := testkit.NewClock(time.Now().UTC())
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer, Clock: clock}, "alice")
	ctx := t.Context()
	for i := range n {
		uri := fmt.Sprintf("https://remote.example/users/u%d", i)
		var actor account.Account
		if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			actor, err = c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
				URI: uri, Domain: "remote.example", Type: "Person", PreferredUsername: fmt.Sprintf("u%d", i),
				InboxURL: uri + "/inbox", Document: []byte(`{"id":"` + uri + `"}`),
				Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: fmt.Sprintf("u%d", i), Domain: "remote.example"},
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			if _, err := c.app.Graph.Follow(ctx, c.ids["alice"], actor.ID); err != nil {
				t.Fatal(err)
			}
			if err := c.app.Graph.AcceptRemote(ctx, appgraph.RemoteResponse{
				FolloweeID: actor.ID, FollowerID: c.ids["alice"], ActivityURI: fmt.Sprintf("https://remote.example/accepts/%d", i),
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	clock.Advance(2 * account.RemoteActorTTL)
	before := len(mustJobs(t, c, appfederation.JobRefreshActor))
	h, _, _ := c.app.Handlers.Lookup(appfederation.JobRefreshActors)
	was := ops(tracer)
	if res := h(ctx, queue.Job{ID: "refresh", Type: appfederation.JobRefreshActors}); res.Err() != nil {
		t.Fatalf("refresh actors: %v", res.Err())
	}
	count := 0
	for op, k := range ops(tracer) {
		count += k - was[op]
	}
	return count, len(mustJobs(t, c, appfederation.JobRefreshActor)) - before
}

// TestRefreshActorsQueries checks that the round trips of one run of JobRefreshActors do not grow
// with the stale actors of its batch: it reads which of them local accounts follow together.
func TestRefreshActorsQueries(t *testing.T) {
	t.Parallel()
	small, smallJobs := refreshActorsQueries(t, 4)
	large, largeJobs := refreshActorsQueries(t, 20)
	t.Logf("JobRefreshActors: %d queries for 4 stale actors, %d for 20", small, large)
	if smallJobs != 2 || largeJobs != 10 {
		t.Errorf("refresh jobs = %d and %d, want those of the followed actors, 2 and 10", smallJobs, largeJobs)
	}
	if large > small {
		t.Errorf("JobRefreshActors makes %d queries for 20 stale actors, more than the %d for 4", large, small)
	}
}
