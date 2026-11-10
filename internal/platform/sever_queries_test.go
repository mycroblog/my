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
	"github.com/mycroblog/my/internal/app/queue"
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

// meshScene makes n local accounts and m remote accounts of remote.example, each local account
// following each remote one and followed by it, and returns the client with the remote accounts.
func meshScene(t *testing.T, tracer *testkit.Tracer, n, m int) (*statusesClient, []string, []string) {
	t.Helper()
	names := make([]string, n)
	for i := range n {
		names[i] = fmt.Sprintf("local%d", i)
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, names...)
	ctx := t.Context()
	var remotes []string
	for j := range m {
		uri := fmt.Sprintf("https://remote.example/users/r%d", j)
		var r account.Account
		if err := c.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			r, err = c.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
				URI: uri, Domain: "remote.example", Type: "Person", PreferredUsername: fmt.Sprintf("r%d", j),
				InboxURL: uri + "/inbox", SharedInboxURL: "https://remote.example/inbox", Document: []byte(`{"id":"` + uri + `"}`),
				Handle: appaccount.HandleCheck{Outcome: account.HandleVerified, Username: fmt.Sprintf("r%d", j), Domain: "remote.example"},
			})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		remotes = append(remotes, r.ID)
	}
	var locals []string
	for i, name := range names {
		local := c.ids[name]
		locals = append(locals, local)
		for j, r := range remotes {
			if _, err := c.app.Graph.Follow(ctx, local, r); err != nil {
				t.Fatal(err)
			}
			if err := c.app.Graph.AcceptRemote(ctx, appgraph.RemoteResponse{FolloweeID: r, FollowerID: local, ActivityURI: fmt.Sprintf("https://remote.example/accepts/%d-%d", i, j)}); err != nil {
				t.Fatal(err)
			}
			if err := c.app.Graph.ReceiveFollow(ctx, appgraph.ReceiveFollowInput{URI: fmt.Sprintf("https://remote.example/follows/%d-%d", i, j), FollowerID: r, FolloweeID: local}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return c, locals, remotes
}

// wantSevered checks that no follow is left between the local and the remote accounts.
func wantSevered(t *testing.T, c *statusesClient, locals, remotes []string) {
	t.Helper()
	for _, local := range locals {
		for _, r := range remotes {
			if rel, err := c.app.Graph.Relationship(t.Context(), local, r); err != nil || rel.Following != graph.StateNone || rel.FollowedBy != graph.StateNone {
				t.Errorf("%s and %s after the severance = %+v, %v; want no follow either way", local, r, rel, err)
			}
		}
	}
}

// domainSeverQueries counts the round trips of the job that cuts the follows of a domain
// suspension, over n local accounts that follow and are followed by m accounts of the domain.
func domainSeverQueries(t *testing.T, n, m int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	c, locals, remotes := meshScene(t, tracer, n, m)
	if _, err := c.app.Moderation.IssueDomainAction(t.Context(), appmoderation.DomainActionInput{Domain: "remote.example", Kind: moderation.DomainSuspend}); err != nil {
		t.Fatalf("suspend the domain: %v", err)
	}
	jobs := mustJobs(t, c, appmoderation.JobSeverDomain)
	if len(jobs) != 1 {
		t.Fatalf("%d sever jobs, want 1", len(jobs))
	}
	h, _, _ := c.app.Handlers.Lookup(appmoderation.JobSeverDomain)
	was := ops(tracer)
	if res := h(t.Context(), queue.Job{ID: jobs[0].ID, Type: jobs[0].Type, Payload: jobs[0].Payload}); res.Err() != nil {
		t.Fatalf("sever the domain: %v", res.Err())
	}
	count, names := opsSince(tracer, was)
	t.Logf("cutting the follows of %d local accounts with %d accounts of a suspended domain: %d queries: %v", n, m, count, names)
	wantSevered(t, c, locals, remotes)
	for _, local := range locals[:1] {
		ns, err := c.app.Notifications.List(t.Context(), local, appnotification.Query{Types: []domnotification.Type{domnotification.TypeRelationshipsSevered}})
		if err != nil || len(ns) != 1 {
			t.Errorf("%s has %d relationships_severed notifications, %v; want one", local, len(ns), err)
		} else if sv, err := c.app.Moderation.SeveranceOf(t.Context(), local, ns[0].SeveranceID); err != nil || sv.FollowersLost != m || sv.FollowingLost != m {
			t.Errorf("the severance of %s = %+v, %v; want %d followers and followees lost", local, sv, err, m)
		}
	}
	return count
}

// TestDomainSeverQueries checks that the round trips of the job that cuts the follows of a
// domain suspension do not grow with the follows of a batch: its local accounts' follows end, and
// their severances are recorded, together.
func TestDomainSeverQueries(t *testing.T) {
	t.Parallel()
	small, large := domainSeverQueries(t, 2, 1), domainSeverQueries(t, 6, 3)
	if large > small {
		t.Errorf("cutting the follows of a domain makes %d queries for 6×3 pairs, more than the %d for 2×1", large, small)
	}
}

// federationSeverQueries counts the round trips of `my admin federation sever --direction both`
// over n local accounts that follow and are followed by m remote accounts.
func federationSeverQueries(t *testing.T, n, m int) int {
	t.Helper()
	tracer := testkit.NewTracer()
	c, locals, remotes := meshScene(t, tracer, n, m)
	fanOuts := len(mustJobs(t, c, appfederation.JobFanOut))
	was := ops(tracer)
	got, err := c.app.FederationSever.Sever(t.Context(), appfederation.SeverBothWays)
	if err != nil {
		t.Fatalf("sever: %v", err)
	}
	count, names := opsSince(tracer, was)
	t.Logf("severing the federation of %d local accounts with %d remote accounts: %d queries: %v", n, m, count, names)
	if got.Followers != n*m || got.Following != n*m {
		t.Errorf("Sever = %+v, want %d followers and followees", got, n*m)
	}
	if sent := len(mustJobs(t, c, appfederation.JobFanOut)) - fanOuts; sent != 2*n*m {
		t.Errorf("the severance enqueued %d fan-outs, want a Reject and an Undo for each of the %d pairs", sent, n*m)
	}
	wantSevered(t, c, locals, remotes)
	return count
}

// TestFederationSeverQueries checks that the round trips of severing the federation do not grow
// with the follows of a batch of local accounts.
func TestFederationSeverQueries(t *testing.T) {
	t.Parallel()
	small, large := federationSeverQueries(t, 2, 1), federationSeverQueries(t, 6, 3)
	if large > small {
		t.Errorf("severing the federation makes %d queries for 6×3 pairs, more than the %d for 2×1", large, small)
	}
}
