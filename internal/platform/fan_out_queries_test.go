// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"strings"
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

// fanOutQueries counts the database round trips of the delivery fan-out of one public post by
// alice, and the deliveries it makes: alice has n remote followers on servers and mentions 3
// remote accounts on other servers. Every query is one round trip (one call in
// pg_stat_statements).
func fanOutQueries(t *testing.T, n, servers int) (queries, deliveries int) {
	t.Helper()
	tracer := testkit.NewTracer()
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, "alice")
	ctx := t.Context()
	remote := func(name, domain string) string {
		uri := "https://" + domain + "/users/" + name
		var a account.Account
		if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			a, err = c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
				URI: uri, Domain: domain, Type: "Person", PreferredUsername: name,
				InboxURL: uri + "/inbox", SharedInboxURL: "https://" + domain + "/inbox", Document: []byte(`{"id":"` + uri + `"}`),
				Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: name, Domain: domain},
			})
			return err
		}); err != nil {
			t.Fatalf("UpsertRemoteActor: %v", err)
		}
		return a.ID
	}
	for i := range n {
		id := remote(fmt.Sprintf("f%d", i), fmt.Sprintf("s%d.example", i%servers))
		if err := c.app.Graph.ReceiveFollow(ctx, appgraph.ReceiveFollowInput{URI: fmt.Sprintf("https://x.example/follows/%d", i), FollowerID: id, FolloweeID: c.ids["alice"]}); err != nil {
			t.Fatalf("ReceiveFollow: %v", err)
		}
	}
	var mentions []string
	for i := range 3 {
		remote(fmt.Sprintf("m%d", i), fmt.Sprintf("m%d.example", i))
		mentions = append(mentions, fmt.Sprintf("@m%d@m%d.example", i, i))
	}
	total := func() int {
		count := 0
		for _, sp := range tracer.Spans() {
			if sp.Attrs["db.operation.name"] != "" {
				count++
			}
		}
		return count
	}
	// Only the fan-out jobs count: the post and the other jobs it enqueues are not part of it.
	// Each job is run once (it stays in the queue, which no runner works); the next batch or phase
	// it enqueues is a new job.
	done := map[string]bool{}
	run := func() int {
		count := 0
		h, _, _ := c.app.Handlers.Lookup(appfederation.JobFanOut)
		for {
			var todo []queue.PendingJob
			for _, j := range mustJobs(t, c, appfederation.JobFanOut) {
				if !done[j.ID] {
					todo = append(todo, j)
				}
			}
			if len(todo) == 0 {
				return count
			}
			for _, j := range todo {
				done[j.ID] = true
				before := total()
				if res := h(ctx, queue.Job{ID: j.ID, Type: j.Type, Payload: j.Payload}); res.Err() != nil {
					t.Fatalf("fan-out: %v", res.Err())
				}
				count += total() - before
			}
		}
	}
	// The Accepts of the follows go out first, uncounted.
	run()
	accepts := len(mustJobs(t, c, appfederation.JobDeliver))
	c.post("alice", map[string]any{"text": "hello " + strings.Join(mentions, " "), "visibility": "public"})
	queries = run()
	deliveries = len(mustJobs(t, c, appfederation.JobDeliver)) - accepts
	return queries, deliveries
}

// mustJobs returns the jobs of jobType waiting in the queue of c.
func mustJobs(t *testing.T, c *statusesClient, jobType string) []queue.PendingJob {
	t.Helper()
	jobs, err := c.app.JobInspector.ListJobs(t.Context(), time.Now().Add(time.Hour), queue.JobFilter{Type: jobType}, queue.JobPosition{}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return jobs
}

// TestFanOutQueries checks that the round trips of the fan-out of a post do not grow with its
// recipients and their servers: the inboxes, the relations of the mentioned accounts, and the
// suspensions and health of the servers are read together, and the deliveries and their jobs are
// written together.
func TestFanOutQueries(t *testing.T) {
	t.Parallel()
	smallQ, smallD := fanOutQueries(t, 20, 4)
	largeQ, largeD := fanOutQueries(t, 100, 100)
	t.Logf("%d queries for 20 followers on 4 servers (%d deliveries), %d for 100 on 100 (%d)", smallQ, smallD, largeQ, largeD)
	if smallD != 4+3 || largeD != 100+3 {
		t.Errorf("deliveries = %d and %d, want one per shared inbox: 7 and 103", smallD, largeD)
	}
	if largeQ > smallQ {
		t.Errorf("%d queries for 100 followers, more than the %d for 20", largeQ, smallQ)
	}
}
