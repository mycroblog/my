// SPDX-License-Identifier: AGPL-3.0-only

package notificationtest

import (
	"reflect"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// runSeverAllowlist runs the enqueued jobs of the allowlist, the fan-out included, and reports how
// many it ran.
func (e *severanceEnv) runSeverAllowlist(t *testing.T) int {
	t.Helper()
	handle, _, ok := e.handlers.Lookup(appmoderation.JobSeverAllowlist)
	if !ok {
		t.Fatalf("no handler for %s", appmoderation.JobSeverAllowlist)
	}
	ran := 0
	for next, more := e.jobs.Pop(); more; next, more = e.jobs.Pop() {
		if next.Type != appmoderation.JobSeverAllowlist {
			t.Errorf("enqueued %s, want only %s", next.Type, appmoderation.JobSeverAllowlist)
			continue
		}
		j, err := next.Build(e.IDs.NewID(), e.Clock.Now())
		if err != nil {
			t.Fatal(err)
		}
		if r := handle(t.Context(), j); !r.IsDone() {
			t.Fatalf("%s = %+v (%v), want Done", next.Type, r, r.Err())
		}
		ran++
	}
	return ran
}

// switchScope changes federation_scope from previous to scope as the change of the setting does,
// in one transaction.
func (e *severanceEnv) switchScope(t *testing.T, previous, scope moderation.FederationScope) {
	t.Helper()
	err := e.H.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return e.mod.FederationScopeSwitchedTx(t.Context(), tx, previous, scope, "")
	})
	if err != nil {
		t.Fatalf("FederationScopeSwitchedTx(%s → %s): %v", previous, scope, err)
	}
	e.scopes.Scope = scope
}

// severanceOfAllowlist checks the cutoffs of the allowlist: the switch to allowlist counts, then
// ends, the follows between the local accounts and the accounts of the domains that the allowlist
// does not allow, without telling them, notifying each local account that lost an accepted follow
// once; the follows with the allowed domains and their subdomains stay; setting allowlist again
// cuts nothing more; removing an allow cuts off the domain and its subdomains, and the
// notification names the domain; under open, nothing is cut; and the follows stay ended after
// switching back.
func severanceOfAllowlist(t *testing.T, e *severanceEnv) {
	ctx := t.Context()
	alice := e.Register(t, "alice").AccountID
	bob := e.Register(t, "bob").AccountID
	carol := e.Register(t, "carol").AccountID
	friend := e.Remote(t, "https://friend.example/users/f")
	sub := e.Remote(t, "https://sub.friend.example/users/s")
	stranger := e.Remote(t, "https://stranger.example/users/x")
	other := e.Remote(t, "https://other.example/users/o")

	outgoing := e.accepted(t, alice, stranger)
	incoming := e.followedBy(t, stranger, alice, "https://stranger.example/follows/1")
	e.accepted(t, alice, friend)
	e.followedBy(t, sub, bob, "https://sub.friend.example/follows/1")
	e.followedBy(t, other, bob, "https://other.example/follows/1")
	e.requested(t, carol, other) // a follow request only
	for _, id := range []string{outgoing.ID, incoming.ID} {
		e.Sent(t, id)
	}
	if _, _, err := e.mod.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "friend.example"}); err != nil {
		t.Fatalf("AllowDomain(friend.example): %v", err)
	}
	// Under open, nothing is enqueued, not even by a removal.
	if _, _, err := e.mod.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "spare.example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mod.RemoveDomainAllow(ctx, "spare.example", ""); err != nil {
		t.Fatal(err)
	}
	if job, ok := e.jobs.Pop(); ok {
		t.Fatalf("enqueued %+v under open, want nothing", job)
	}

	// The impact counts the accepted follows with the domains that the allowlist does not allow.
	impact, err := e.mod.FederationScopeImpact(ctx)
	want := appmoderation.ScopeImpact{
		Scope: moderation.ScopeOpen, Follows: 3, LocalAccounts: 2, Domains: 2,
		TopDomains: []appmoderation.DomainImpact{{Domain: "stranger.example", Follows: 2}, {Domain: "other.example", Follows: 1}},
	}
	if err != nil || !reflect.DeepEqual(impact, want) {
		t.Errorf("FederationScopeImpact = %+v, %v; want %+v", impact, err, want)
	}

	e.Clock.Advance(time.Minute)
	e.switchScope(t, moderation.ScopeOpen, moderation.ScopeAllowlist)
	// Until the job runs, the follows stay.
	if r := e.Relationship(t, alice, stranger); r.Following != graph.StateActive {
		t.Fatalf("alice's follow before the job = %q, want active", r.Following)
	}
	if n := e.runSeverAllowlist(t); n == 0 {
		t.Fatal("the switch enqueued no job")
	}
	e.wantNoFollows(t, alice, stranger, "after the switch")
	e.wantNoFollows(t, bob, other, "after the switch")
	e.wantNoFollows(t, carol, other, "after the switch")
	if r := e.Relationship(t, alice, friend); r.Following != graph.StateActive {
		t.Errorf("alice's follow of friend.example = %q, want active", r.Following)
	}
	if r := e.Relationship(t, bob, sub); r.FollowedBy != graph.StateActive {
		t.Errorf("sub.friend.example's follow of bob = %q, want active", r.FollowedBy)
	}
	// Nothing goes to the servers cut off.
	if got := e.lastSent(t, outgoing.ID); got != federation.ActivityFollow {
		t.Errorf("last sent about alice's follow = %q, want still its Follow", got)
	}
	if got := e.lastSent(t, incoming.ID); got != federation.ActivityAccept {
		t.Errorf("last sent about the follow of alice = %q, want still its Accept", got)
	}
	type counts struct{ followers, following int }
	check := func(id string, w []counts, domain string, when string) []moderation.SeveranceRecord {
		t.Helper()
		got := e.severed(t, id)
		if len(got) != len(w) {
			t.Fatalf("severances of %s %s = %+v, want %d", id, when, got, len(w))
		}
		for i, sv := range got {
			if sv.AllowlistCutoffID == "" || sv.DomainActionID != "" || sv.ActionID != "" ||
				sv.FollowersLost != w[i].followers || sv.FollowingLost != w[i].following {
				t.Errorf("severance %d of %s %s = %+v, want one of a cutoff with %+v", i, id, when, sv, w[i])
			}
		}
		if len(got) > 0 && got[0].Domain != domain {
			t.Errorf("newest severance of %s %s names %q, want %q", id, when, got[0].Domain, domain)
		}
		return got
	}
	check(alice, []counts{{1, 1}}, "", "after the switch")
	check(bob, []counts{{1, 0}}, "", "after the switch")
	check(carol, nil, "", "after the switch")
	if impact, err := e.mod.FederationScopeImpact(ctx); err != nil || impact.Follows != 0 || impact.Scope != moderation.ScopeAllowlist {
		t.Errorf("FederationScopeImpact after the switch = %+v, %v; want nothing left", impact, err)
	}

	// Setting allowlist again records nothing.
	e.switchScope(t, moderation.ScopeAllowlist, moderation.ScopeAllowlist)
	if job, ok := e.jobs.Pop(); ok {
		t.Errorf("setting allowlist again enqueued %+v, want nothing", job)
	}

	// Removing the allow of friend.example cuts it and its subdomains off.
	e.Clock.Advance(time.Minute)
	if _, err := e.mod.RemoveDomainAllow(ctx, "friend.example", ""); err != nil {
		t.Fatalf("RemoveDomainAllow: %v", err)
	}
	if n := e.runSeverAllowlist(t); n == 0 {
		t.Fatal("the removal enqueued no job")
	}
	e.wantNoFollows(t, alice, friend, "after the removal")
	e.wantNoFollows(t, bob, sub, "after the removal")
	check(alice, []counts{{0, 1}, {1, 1}}, "friend.example", "after the removal")
	check(bob, []counts{{1, 0}, {1, 0}}, "friend.example", "after the removal")

	// Switching back to open restores nothing.
	e.switchScope(t, moderation.ScopeAllowlist, moderation.ScopeOpen)
	e.wantNoFollows(t, alice, stranger, "after switching back")
	if job, ok := e.jobs.Pop(); ok {
		t.Errorf("switching back enqueued %+v, want nothing", job)
	}
}
