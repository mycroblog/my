// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"strings"
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

// localNoteQueries counts the round trips of the fan-out of a local status of alice that
// mentions n remote accounts, half of which follow her: the fan-out builds its Note, which keeps
// the mentions of the followers alone.
func localNoteQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer, HTTPClient: network}, "alice")
	ctx := t.Context()
	var mentions []string
	for i := range n {
		uri := fmt.Sprintf("https://remote.example/users/r%d", i)
		var r account.Account
		if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			r, err = c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
				URI: uri, Domain: "remote.example", Type: "Person", PreferredUsername: fmt.Sprintf("r%d", i),
				InboxURL: uri + "/inbox", SharedInboxURL: "https://remote.example/inbox", Document: []byte(`{"id":"` + uri + `"}`),
				Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: fmt.Sprintf("r%d", i), Domain: "remote.example"},
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			if err := c.app.Graph.ReceiveFollow(ctx, appgraph.ReceiveFollowInput{URI: fmt.Sprintf("https://remote.example/follows/%d", i), FollowerID: r.ID, FolloweeID: c.ids["alice"]}); err != nil {
				t.Fatal(err)
			}
		}
		mentions = append(mentions, fmt.Sprintf("@r%d@remote.example", i))
	}
	done := map[string]bool{}
	runAll := func() {
		t.Helper()
		for {
			var todo []queue.PendingJob
			for _, j := range mustJobs(t, c, appfederation.JobFanOut) {
				if !done[j.ID] {
					todo = append(todo, j)
				}
			}
			if len(todo) == 0 {
				return
			}
			h, _, _ := c.app.Handlers.Lookup(appfederation.JobFanOut)
			for _, j := range todo {
				done[j.ID] = true
				if res := h(ctx, queue.Job{ID: j.ID, Type: j.Type, Payload: j.Payload}); res.Err() != nil {
					t.Fatal(res.Err())
				}
			}
		}
	}
	runAll() // the Accepts of the follows
	c.post("alice", map[string]any{"text": "hello " + strings.Join(mentions, " "), "visibility": "local"})
	was := ops(tracer)
	runAll()
	count, names := opsSince(tracer, was)
	t.Logf("the fan-out of a local status with %d remote mentions: %d queries: %v", n, count, names)
	return count
}

// TestLocalNoteQueries checks that building the Note of a local status reads which of its
// remote mentions follow the author with a number of queries that does not grow with them.
func TestLocalNoteQueries(t *testing.T) {
	t.Parallel()
	small, large := localNoteQueries(t, 2), localNoteQueries(t, 6)
	if large > small {
		t.Errorf("the fan-out of a local status makes %d queries for 6 remote mentions, more than the %d for 2", large, small)
	}
}
