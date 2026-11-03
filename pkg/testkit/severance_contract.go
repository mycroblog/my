// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	appgraph "github.com/mycroblog/my/internal/app/graph"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/domain/notification"
)

// SeveranceHarness is one database under test: the harness of RemoteFollowContract and the
// moderation repository on the same database.
type SeveranceHarness struct {
	RemoteFollowHarness
	Moderation appmoderation.Repository
}

// SeveranceFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type SeveranceFactory func(t *testing.T, opts uow.Options) SeveranceHarness

// SeveranceContract verifies the follows that suspensions cut (FR-717, ADR-0036) on a database
// adapter, through the real account, graph, moderation, notification, and federation use cases
// and projectors: the suspension of a remote account ends its follows with the local accounts
// in both directions in the action's transaction and sends a Reject of each of its follows and
// an Undo of each local follow of it; the suspension of a domain ends, in its job, the follows
// between the local accounts and the accounts of the domain and its subdomains, and sends
// nothing; each local account that lost an active follow gets one relationships_severed
// notification whose severance counts what it lost, a lost follow request counts nothing, the
// follows stay ended after the lift, and processing the same suspension again, or another
// suspension of the same account, records and notifies nothing more.
func SeveranceContract(t *testing.T, newHarness SeveranceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *severanceEnv)
	}{
		{"RemoteAccountSuspension", severanceOfAccount},
		{"DomainSuspension", severanceOfDomain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newSeveranceEnv(t, newHarness)) })
	}
}

type severanceEnv struct {
	*remoteFollowEnv
	mod *appmoderation.Service
	// jobs holds the moderation jobs, apart from the federation jobs of remoteFollowEnv.
	jobs     *fedJobs
	handlers *queue.JobHandlerRegistry
}

func newSeveranceEnv(t *testing.T, newHarness SeveranceFactory) *severanceEnv {
	t.Helper()
	var (
		mods       appmoderation.Repository
		projectors *uow.Projectors
	)
	re := newRemoteFollowEnv(t, func(t *testing.T, opts uow.Options) RemoteFollowHarness {
		h := newHarness(t, opts)
		mods, projectors = h.Moderation, opts.Projectors
		return h.RemoteFollowHarness
	})
	e := &severanceEnv{remoteFollowEnv: re, jobs: &fedJobs{seen: map[string]bool{}}, handlers: &queue.JobHandlerRegistry{}}
	e.mod = appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: re.h.UnitOfWork, Repository: mods, Accounts: re.accounts, Clock: re.clock, IDs: re.ids,
		Follows: re.graph, Directory: re.accounts, Outbox: queue.AfterCommit(projectors, e.jobs, nil), FanOutSize: 1,
	})
	e.mod.RegisterJobs(e.handlers, queue.NewScheduler(e.jobs, re.clock, nil))
	return e
}

// accepted makes local follow remote, accepted by the remote server.
func (e *severanceEnv) accepted(t *testing.T, local, remote string) graph.Follow {
	t.Helper()
	f := e.requested(t, local, remote)
	err := e.graph.AcceptRemote(t.Context(), appgraph.RemoteResponse{FollowURI: f.URI, FolloweeID: remote, ActivityURI: f.URI + "#accept"})
	if err != nil {
		t.Fatalf("AcceptRemote(%s): %v", f.URI, err)
	}
	return f
}

// requested makes local follow remote, pending until the remote server accepts.
func (e *severanceEnv) requested(t *testing.T, local, remote string) graph.Follow {
	t.Helper()
	if _, err := e.graph.Follow(t.Context(), local, remote); err != nil {
		t.Fatalf("Follow(%s, %s): %v", local, remote, err)
	}
	return e.liveFollow(t, local, remote)
}

// followedBy makes the remote account follow local.
func (e *severanceEnv) followedBy(t *testing.T, remote, local, uri string) graph.Follow {
	t.Helper()
	if err := e.graph.ReceiveFollow(t.Context(), appgraph.ReceiveFollowInput{URI: uri, FollowerID: remote, FolloweeID: local}); err != nil {
		t.Fatalf("ReceiveFollow(%s): %v", uri, err)
	}
	return e.liveFollow(t, remote, local)
}

// severed returns the relationships_severed notifications of accountID with their severances.
func (e *severanceEnv) severed(t *testing.T, accountID string) []moderation.SeveranceRecord {
	t.Helper()
	ns, err := e.notifs.List(t.Context(), accountID, appnotification.Query{Types: []notification.Type{notification.TypeRelationshipsSevered}})
	if err != nil {
		t.Fatalf("notifications of %s: %v", accountID, err)
	}
	out := []moderation.SeveranceRecord{}
	for _, n := range ns {
		if n.SourceAccountID != "" || n.DedupeKey != notification.RelationshipsSeveredKey(n.SeveranceID) {
			t.Errorf("notification = %+v, want one from the instance keyed by its severance", n)
		}
		sv, err := e.mod.SeveranceOf(t.Context(), accountID, n.SeveranceID)
		if err != nil {
			t.Fatalf("SeveranceOf(%s, %s): %v", accountID, n.SeveranceID, err)
		}
		if !sv.OccurredAt.Equal(n.CreatedAt) {
			t.Errorf("notification at %v, want the severance's %v", n.CreatedAt, sv.OccurredAt)
		}
		out = append(out, sv)
	}
	return out
}

// wantNoFollows checks that neither account follows the other.
func (e *severanceEnv) wantNoFollows(t *testing.T, local, remote, when string) {
	t.Helper()
	if r := e.relationship(t, local, remote); r.Following != graph.StateNone || r.FollowedBy != graph.StateNone {
		t.Errorf("relationship of %s and %s %s = %+v, want none either way", local, remote, when, r)
	}
}

// lastSent returns the type of the last activity recorded about a follow.
func (e *severanceEnv) lastSent(t *testing.T, followID string) federation.ActivityType {
	t.Helper()
	sent := e.sent(t, followID)
	if len(sent) == 0 {
		return ""
	}
	return sent[len(sent)-1].Type
}

func severanceOfAccount(t *testing.T, e *severanceEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").AccountID
	bob := e.register(t, "bob").AccountID
	carol := e.register(t, "carol").AccountID
	spammer := e.remote(t, "https://spam.example/users/spammer")
	other := e.remote(t, "https://spam.example/users/other")

	outgoing := e.accepted(t, alice, spammer)
	incoming := e.followedBy(t, spammer, alice, "https://spam.example/follows/1")
	request := e.requested(t, bob, spammer) // a follow request only
	kept := e.accepted(t, carol, other)
	e.followedBy(t, spammer, carol, "https://spam.example/follows/2")
	for _, id := range []string{outgoing.ID, incoming.ID, request.ID, kept.ID} {
		e.sent(t, id) // drain the Follows and Accepts
	}

	e.clock.Advance(time.Minute)
	a, err := e.mod.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: spammer, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	})
	if err != nil {
		t.Fatalf("IssueAccountAction(suspend): %v", err)
	}
	e.wantNoFollows(t, alice, spammer, "after the suspension")
	e.wantNoFollows(t, bob, spammer, "after the suspension")
	e.wantNoFollows(t, carol, spammer, "after the suspension")
	if r := e.relationship(t, carol, other); r.Following != graph.StateActive {
		t.Errorf("carol's follow of another account of the server = %q, want active", r.Following)
	}

	// The suspended account's server hears of each follow.
	if got := e.lastSent(t, incoming.ID); got != federation.ActivityReject {
		t.Errorf("last sent about the spammer's follow = %q, want a Reject", got)
	}
	if got := e.lastSent(t, outgoing.ID); got != federation.ActivityUndo {
		t.Errorf("last sent about alice's follow = %q, want an Undo", got)
	}
	if got := e.lastSent(t, request.ID); got != federation.ActivityUndo {
		t.Errorf("last sent about bob's follow request = %q, want an Undo", got)
	}

	// One notification per local account that lost an accepted follow, with the counts.
	got := e.severed(t, alice)
	if len(got) != 1 || got[0].ActionID != a.ID || got[0].DomainActionID != "" || got[0].TargetAccountID != spammer ||
		got[0].FollowersLost != 1 || got[0].FollowingLost != 1 || got[0].AccountID != alice {
		t.Errorf("severances of alice = %+v, want one of the action %s on the spammer, 1 follower and 1 following", got, a.ID)
	}
	if got := e.severed(t, carol); len(got) != 1 || got[0].FollowersLost != 1 || got[0].FollowingLost != 0 {
		t.Errorf("severances of carol = %+v, want one with 1 follower", got)
	}
	if got := e.severed(t, bob); len(got) != 0 {
		t.Errorf("severances of bob = %+v, want none for a follow request", got)
	}
	// Another account's severance is not found.
	if _, err := e.mod.SeveranceOf(ctx, bob, e.severed(t, alice)[0].ID); !errors.Is(err, appmoderation.ErrNotFound) {
		t.Errorf("SeveranceOf(bob, alice's) = %v, want ErrNotFound", err)
	}

	// Lifting the suspension restores nothing, and suspending again cuts and notifies nothing.
	e.clock.Advance(time.Minute)
	if err := e.mod.LiftAccountAction(ctx, a.ID, "", ""); err != nil {
		t.Fatalf("LiftAccountAction: %v", err)
	}
	e.wantNoFollows(t, alice, spammer, "after the lift")
	if _, err := e.mod.IssueAccountAction(ctx, appmoderation.AccountActionInput{
		TargetAccountID: spammer, Kind: moderation.ActionSuspend, ReasonCategory: moderation.CategorySpam,
	}); err != nil {
		t.Fatalf("IssueAccountAction(suspend again): %v", err)
	}
	if got := e.severed(t, alice); len(got) != 1 {
		t.Errorf("severances of alice after suspending again = %+v, want still one", got)
	}
}

func severanceOfDomain(t *testing.T, e *severanceEnv) {
	ctx := t.Context()
	alice := e.register(t, "alice").AccountID
	bob := e.register(t, "bob").AccountID
	carol := e.register(t, "carol").AccountID
	bad := e.remote(t, "https://bad.example/users/x")
	sub := e.remote(t, "https://sub.bad.example/users/y")
	good := e.remote(t, "https://notbad.example/users/z")

	outgoing := e.accepted(t, alice, bad)
	incoming := e.followedBy(t, sub, alice, "https://sub.bad.example/follows/1")
	e.accepted(t, alice, good)
	e.followedBy(t, bad, bob, "https://bad.example/follows/1")
	e.followedBy(t, sub, bob, "https://sub.bad.example/follows/2")
	e.accepted(t, carol, good)
	for _, id := range []string{outgoing.ID, incoming.ID} {
		e.sent(t, id)
	}

	e.clock.Advance(time.Minute)
	a, err := e.mod.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainSuspend})
	if err != nil {
		t.Fatalf("IssueDomainAction(suspend): %v", err)
	}
	// Another kind of domain action enqueues nothing.
	if _, err := e.mod.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "bad.example", Kind: moderation.DomainRejectMedia}); err != nil {
		t.Fatalf("IssueDomainAction(reject_media): %v", err)
	}
	job, ok := e.jobs.pop()
	if !ok || job.Type != appmoderation.JobSeverDomain {
		t.Fatalf("enqueued %+v (%v), want %s", job, ok, appmoderation.JobSeverDomain)
	}
	if extra, ok := e.jobs.pop(); ok {
		t.Errorf("enqueued %+v too, want only the job of the suspension", extra)
	}
	run := func() {
		t.Helper()
		handle, _, ok := e.handlers.Lookup(job.Type)
		if !ok {
			t.Fatalf("no handler for %s", job.Type)
		}
		// The job, and the jobs of the fan-out it splits into, as each covers one local account.
		for next, more := job, true; more; next, more = e.jobs.pop() {
			if next.Type != appmoderation.JobSeverDomain {
				continue
			}
			j, err := next.Build(e.ids.NewID(), e.clock.Now())
			if err != nil {
				t.Fatal(err)
			}
			if r := handle(ctx, j); !r.IsDone() {
				t.Fatalf("%s = %+v (%v), want Done", next.Type, r, r.Err())
			}
		}
	}
	// Until the job runs, the follows stay.
	if r := e.relationship(t, alice, bad); r.Following != graph.StateActive {
		t.Fatalf("alice's follow before the job = %q, want active", r.Following)
	}
	e.clock.Advance(time.Minute)
	run()

	e.wantNoFollows(t, alice, bad, "after the domain suspension")
	e.wantNoFollows(t, alice, sub, "after the domain suspension")
	e.wantNoFollows(t, bob, bad, "after the domain suspension")
	e.wantNoFollows(t, bob, sub, "after the domain suspension")
	if r := e.relationship(t, alice, good); r.Following != graph.StateActive {
		t.Errorf("alice's follow of notbad.example = %q, want active", r.Following)
	}
	// Nothing goes to the suspended servers.
	if got := e.lastSent(t, outgoing.ID); got != federation.ActivityFollow {
		t.Errorf("last sent about alice's follow = %q, want still its Follow", got)
	}
	if got := e.lastSent(t, incoming.ID); got != federation.ActivityAccept {
		t.Errorf("last sent about the follow of alice = %q, want still its Accept", got)
	}

	type counts struct{ followers, following int }
	want := map[string]counts{alice: {1, 1}, bob: {2, 0}}
	check := func(when string) {
		t.Helper()
		for id, w := range want {
			got := e.severed(t, id)
			if len(got) != 1 || got[0].DomainActionID != a.ID || got[0].ActionID != "" || got[0].Domain != "bad.example" ||
				got[0].FollowersLost != w.followers || got[0].FollowingLost != w.following {
				t.Errorf("severances of %s %s = %+v, want one of %s with %+v", id, when, got, a.ID, w)
			}
		}
		if got := e.severed(t, carol); len(got) != 0 {
			t.Errorf("severances of carol %s = %+v, want none", when, got)
		}
	}
	check("after the job")

	// The job again records and notifies nothing more.
	e.clock.Advance(time.Minute)
	run()
	if n, err := e.mod.SeverDomain(ctx, a.ID); err != nil || n != 0 {
		t.Errorf("SeverDomain again = %d, %v; want 0", n, err)
	}
	check("after the job again")

	// Lifting the suspension restores nothing.
	if err := e.mod.LiftDomainAction(ctx, a.ID, ""); err != nil {
		t.Fatalf("LiftDomainAction: %v", err)
	}
	e.wantNoFollows(t, alice, bad, "after the lift")

	// A payload without an action fails the job.
	handle, _, _ := e.handlers.Lookup(appmoderation.JobSeverDomain)
	payload, _ := json.Marshal(map[string]string{"domain_action_id": NamedID("missing")})
	if r := handle(ctx, queue.Job{ID: NamedID("j"), Type: appmoderation.JobSeverDomain, Payload: payload}); r.IsDone() || r.Err() == nil {
		t.Errorf("job of a missing action = %+v, want a failure", r)
	}
}
