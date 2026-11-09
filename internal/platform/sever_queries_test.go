// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"fmt"
	"sort"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/moderation"
	domnotification "github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// severScene makes n local accounts that follow a remote account and that it follows, and
// returns the client with the remote account.
func severScene(t *testing.T, tracer *testkit.Tracer, n int) (*statusesClient, string) {
	t.Helper()
	names := make([]string, n)
	for i := range n {
		names[i] = fmt.Sprintf("local%d", i)
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, names...)
	ctx := t.Context()
	uri := "https://remote.example/users/star"
	var star account.Account
	if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		star, err = c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: uri, Domain: "remote.example", Type: "Person", PreferredUsername: "star",
			InboxURL: uri + "/inbox", SharedInboxURL: "https://remote.example/inbox", Document: []byte(`{"id":"` + uri + `"}`),
			Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "star", Domain: "remote.example"},
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for i, name := range names {
		local := c.ids[name]
		if _, err := c.app.Graph.Follow(ctx, local, star.ID); err != nil {
			t.Fatal(err)
		}
		if err := c.app.Graph.AcceptRemote(ctx, appgraph.RemoteResponse{FolloweeID: star.ID, FollowerID: local, ActivityURI: fmt.Sprintf("https://remote.example/accepts/%d", i)}); err != nil {
			t.Fatal(err)
		}
		if err := c.app.Graph.ReceiveFollow(ctx, appgraph.ReceiveFollowInput{URI: fmt.Sprintf("https://remote.example/follows/%d", i), FollowerID: star.ID, FolloweeID: local}); err != nil {
			t.Fatal(err)
		}
	}
	return c, star.ID
}

// opsSince returns the queries made since was, by operation, and their number.
func opsSince(tracer *testkit.Tracer, was map[string]int) (int, []string) {
	count := 0
	var names []string
	for op, n := range ops(tracer) {
		if d := n - was[op]; d > 0 {
			count += d
			names = append(names, fmt.Sprintf("%s×%d", op, d))
		}
	}
	sort.Strings(names)
	return count, names
}

// suspendRemoteQueries counts the round trips of the suspension of a remote account that n
// local accounts follow and that follows them.
func suspendRemoteQueries(t *testing.T, n int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	c, star := severScene(t, tracer, n)
	fanOuts := len(mustJobs(t, c, appfederation.JobFanOut))
	was := ops(tracer)
	if _, err := c.app.Moderation.IssueAccountAction(t.Context(), appmoderation.AccountActionInput{
		TargetAccountID: star, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	}); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	count, names := opsSince(tracer, was)
	t.Logf("suspending a remote account with %d local followers: %d queries: %v", n, count, names)
	// Each local account gets an Undo of its follow and a Reject of the remote one, and a
	// notification of what it lost.
	if got := len(mustJobs(t, c, appfederation.JobFanOut)) - fanOuts; got != 2*n {
		t.Errorf("the suspension enqueued %d fan-outs, want an Undo and a Reject for each of the %d local accounts", got, n)
	}
	for _, name := range []string{"local0", fmt.Sprintf("local%d", n-1)} {
		ns, err := c.app.Notifications.List(t.Context(), c.ids[name], appnotification.Query{Types: []domnotification.Type{domnotification.TypeRelationshipsSevered}})
		if err != nil || len(ns) != 1 {
			t.Errorf("%s has %d relationships_severed notifications, %v; want one", name, len(ns), err)
		} else if sv, err := c.app.Moderation.SeveranceOf(t.Context(), c.ids[name], ns[0].SeveranceID); err != nil || sv.FollowersLost != 1 || sv.FollowingLost != 1 {
			t.Errorf("the severance of %s = %+v, %v; want one follower and one followee lost", name, sv, err)
		}
		if r, err := c.app.Graph.Relationship(t.Context(), c.ids[name], star); err != nil || r.Following != graph.StateNone || r.FollowedBy != graph.StateNone {
			t.Errorf("%s after the suspension = %+v, %v; want no follow either way", name, r, err)
		}
	}
	for _, id := range []string{c.ids["local0"], star} {
		if got, err := c.app.Graph.Counters(t.Context(), id); err != nil || got.FollowersCount != 0 || got.FollowingCount != 0 {
			t.Errorf("the counters of %s after the suspension = %+v, %v; want none", id, got, err)
		}
	}
	return count
}

// TestSuspendRemoteQueries checks that the round trips of the suspension of a remote account do
// not grow with its local followers: it ends the follows and records the severances together.
func TestSuspendRemoteQueries(t *testing.T) {
	t.Parallel()
	small, large := suspendRemoteQueries(t, 2), suspendRemoteQueries(t, 8)
	if large > small {
		t.Errorf("suspending a remote account makes %d queries for 8 local followers, more than the %d for 2", large, small)
	}
}
