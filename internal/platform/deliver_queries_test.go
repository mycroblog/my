// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"sort"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestDeliveryQueries counts the database round trips of one delivery attempt of the Create of a
// post to a remote follower's server, outside the HTTP request: at most 5 (one query is one round
// trip, one call in pg_stat_statements).
func TestDeliveryQueries(t *testing.T) {
	t.Parallel()
	tracer := testkit.NewTracer()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer, HTTPClient: network}, "alice")
	ctx := t.Context()
	uri := "https://remote.example/users/bob"
	var bob account.Account
	if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		bob, err = c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: uri, Domain: "remote.example", Type: "Person", PreferredUsername: "bob",
			InboxURL: uri + "/inbox", SharedInboxURL: "https://remote.example/inbox", Document: []byte(`{"id":"` + uri + `"}`),
			Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "bob", Domain: "remote.example"},
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.app.Graph.ReceiveFollow(ctx, appgraph.ReceiveFollowInput{URI: "https://remote.example/follows/1", FollowerID: bob.ID, FolloweeID: c.ids["alice"]}); err != nil {
		t.Fatal(err)
	}
	// Every job of the follow and of the post runs but the delivery of the post's Create.
	c.post("alice", map[string]any{"text": "hello", "visibility": "public"})
	done := map[string]bool{}
	for {
		var todo []queue.PendingJob
		for _, j := range mustJobs(t, c, appfederation.JobFanOut) {
			if !done[j.ID] {
				todo = append(todo, j)
			}
		}
		if len(todo) == 0 {
			break
		}
		h, _, _ := c.app.Handlers.Lookup(appfederation.JobFanOut)
		for _, j := range todo {
			done[j.ID] = true
			if res := h(ctx, queue.Job{ID: j.ID, Type: j.Type, Payload: j.Payload}); res.Err() != nil {
				t.Fatal(res.Err())
			}
		}
	}
	deliveries := mustJobs(t, c, appfederation.JobDeliver)
	if len(deliveries) == 0 {
		t.Fatal("no delivery")
	}
	j := deliveries[len(deliveries)-1]
	h, _, _ := c.app.Handlers.Lookup(appfederation.JobDeliver)
	was := ops(tracer)
	if res := h(ctx, queue.Job{ID: j.ID, Type: j.Type, Payload: j.Payload}); res.Err() != nil {
		t.Fatalf("deliver: %v", res.Err())
	}
	count := 0
	var names []string
	for op, n := range ops(tracer) {
		if d := n - was[op]; d > 0 {
			count += d
			names = append(names, op)
		}
	}
	sort.Strings(names)
	t.Logf("one delivery attempt: %d queries: %v", count, names)
	if count > 5 {
		t.Errorf("one delivery attempt makes %d queries, want at most 5", count)
	}
}
