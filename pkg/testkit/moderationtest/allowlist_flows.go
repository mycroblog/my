// SPDX-License-Identifier: AGPL-3.0-only

package moderationtest

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// StubScopes is a moderation.FederationScopes that answers Scope, open when it is "".
type StubScopes struct{ Scope moderation.FederationScope }

// FederationScope implements moderation.FederationScopes.
func (s *StubScopes) FederationScope(context.Context) (moderation.FederationScope, error) {
	return cmpScope(s.Scope), nil
}

// FederationScopeTx implements moderation.FederationScopes.
func (s *StubScopes) FederationScopeTx(context.Context, uow.Tx) (moderation.FederationScope, error) {
	return cmpScope(s.Scope), nil
}

func cmpScope(s moderation.FederationScope) moderation.FederationScope {
	if s == "" {
		return moderation.ScopeOpen
	}
	return s
}

// moderationAllowlist checks the allowlist of federation (ADR-0159 §1-§3) through the use cases
// and the scoped FederationPolicy: under open the allowlist changes nothing; under allowlist a
// domain that neither it nor a parent allows has the restrictions of a suspension, an allowed
// domain and its subdomains federate, a domain action on an allowed domain still applies, and the
// unscoped policy reads the actions only; an allow is added once, audited, and refused for this
// instance's domain, an invalid domain, or a long comment; a removal is audited, and under
// allowlist records a cutoff that the audit entry points at, unless a parent domain is still
// allowed; and a switch of the scope is taken whichever way it goes (the cutoffs that a switch
// records and what they end are notificationtest.SeveranceFlows).
func moderationAllowlist(t *testing.T, e *moderationEnv) {
	ctx := t.Context()
	scopes := &StubScopes{}
	svc := appmoderation.NewService(appmoderation.Deps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Moderation, Accounts: e.actors, Clock: e.clock, IDs: e.ids,
		LocalDomains: []string{"self.example"}, Scopes: scopes,
	})
	policy := appmoderation.NewFederationPolicy(e.h.Moderation, e.clock).Scoped(scopes)
	moderator := e.ids.NewID()

	suspended := func(domain string) bool {
		t.Helper()
		got, err := policy.FederationSuspended(ctx, domain)
		if err != nil {
			t.Fatalf("FederationSuspended(%s): %v", domain, err)
		}
		return got
	}

	if _, created, err := svc.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "friend.example", Comment: "partner", ByUserID: moderator}); err != nil || !created {
		t.Fatalf("AllowDomain(friend.example) = %v, %v; want created", created, err)
	}
	if a, created, err := svc.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "friend.example", Comment: "other"}); err != nil || created || a.Comment != "partner" {
		t.Errorf("AllowDomain(friend.example again) = %+v, %v, %v; want the first allow kept", a, created, err)
	}
	if _, _, err := svc.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "blocked.example"}); err != nil {
		t.Fatalf("AllowDomain(blocked.example): %v", err)
	}
	for _, tc := range []struct {
		in   appmoderation.AllowDomainInput
		want error
	}{
		{appmoderation.AllowDomainInput{Domain: "self.example"}, moderation.ErrOwnDomain},
		{appmoderation.AllowDomainInput{Domain: "Not A Host"}, moderation.ErrInvalidDomainAllow},
		{appmoderation.AllowDomainInput{Domain: "long.example", Comment: strings.Repeat("x", moderation.MaxDomainAllowComment+1)}, moderation.ErrInvalidDomainAllow},
	} {
		if _, _, err := svc.AllowDomain(ctx, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("AllowDomain(%q) = %v, want %v", tc.in.Domain, err, tc.want)
		}
	}
	allows, err := svc.DomainAllows(ctx)
	if err != nil || len(allows) != 2 || allows[0].Domain != "blocked.example" || allows[1].Domain != "friend.example" || allows[1].CreatedByUserID != moderator {
		t.Errorf("DomainAllows = %+v, %v; want blocked.example then friend.example by the moderator", allows, err)
	}
	e.domainAction(t, "blocked.example", moderation.DomainSuspend, 0)

	// Under open, the allowlist changes nothing.
	if suspended("stranger.example") || suspended("friend.example") || !suspended("blocked.example") {
		t.Errorf("under open: stranger %v, friend %v, blocked %v; want only the suspended domain suspended",
			suspended("stranger.example"), suspended("friend.example"), suspended("blocked.example"))
	}

	// Under allowlist, a domain without an allow is suspended; an allowed one and its subdomains
	// federate, and its actions still apply.
	scopes.Scope = moderation.ScopeAllowlist
	for domain, want := range map[string]bool{
		"stranger.example": true, "example": true, "friend.example": false, "sub.friend.example": false,
		"blocked.example": true, "notfriend.example": true,
	} {
		if got := suspended(domain); got != want {
			t.Errorf("under allowlist, FederationSuspended(%s) = %v, want %v", domain, got, want)
		}
	}
	if got, err := policy.FederationSuspended(ctx, ""); err != nil || got {
		t.Errorf("FederationSuspended(local) = %v, %v; want false", got, err)
	}
	rs, err := policy.DomainRestrictions(ctx, "stranger.example")
	want := moderation.CutOffRestrictions()
	slices.Sort(want)
	if err != nil || !reflect.DeepEqual(rs, want) {
		t.Errorf("DomainRestrictions(stranger.example) = %v, %v; want those of a suspension", rs, err)
	}
	among, err := policy.FederationSuspendedAmong(ctx, []string{"stranger.example", "sub.friend.example", "blocked.example"})
	if err != nil || !reflect.DeepEqual(among, map[string]bool{"stranger.example": true, "blocked.example": true}) {
		t.Errorf("FederationSuspendedAmong = %v, %v; want stranger and blocked", among, err)
	}
	all, err := policy.DomainRestrictionsAmong(ctx, []string{"stranger.example", "friend.example"})
	if err != nil || !slices.Contains(all["stranger.example"], moderation.RestrictFederation) || len(all["friend.example"]) != 0 {
		t.Errorf("DomainRestrictionsAmong = %v, %v; want stranger cut off and friend free", all, err)
	}
	if got, err := policy.DiscoveryRestricted(ctx, "stranger.example", ""); err != nil || !got {
		t.Errorf("DiscoveryRestricted(stranger.example) = %v, %v; want true", got, err)
	}
	if got, err := policy.Unscoped().FederationSuspended(ctx, "stranger.example"); err != nil || got {
		t.Errorf("unscoped FederationSuspended(stranger.example) = %v, %v; want false", got, err)
	}

	audit := func(action moderation.AuditAction) []moderation.AuditEntry {
		t.Helper()
		got, err := svc.AuditLog(ctx, appmoderation.AuditQuery{Action: action}, 10)
		if err != nil {
			t.Fatalf("AuditLog(%s): %v", action, err)
		}
		return got
	}
	if got := audit(moderation.AuditDomainAllowAdded); len(got) != 2 || got[1].TargetID != "friend.example" ||
		got[1].TargetKind != moderation.AuditTargetDomainAllow || got[1].ActorUserID != moderator || got[0].ActorKind != moderation.AuditActorCLI {
		t.Errorf("domain_allow_added entries = %+v, want blocked.example by the CLI and friend.example by the moderator", got)
	}

	// Removing the allow of a subdomain whose parent is allowed cuts nothing off.
	if _, _, err := svc.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: "sub.friend.example"}); err != nil {
		t.Fatal(err)
	}
	if removed, err := svc.RemoveDomainAllow(ctx, "sub.friend.example", moderator); err != nil || !removed {
		t.Fatalf("RemoveDomainAllow(sub.friend.example) = %v, %v; want removed", removed, err)
	}
	if got := audit(moderation.AuditDomainAllowRemoved); len(got) != 1 || got[0].SourceEventID != "" {
		t.Errorf("removal of a covered subdomain = %+v, want an entry without a cutoff", got)
	}
	// Removing the allow of friend.example cuts it off at once, recorded as a cutoff.
	if removed, err := svc.RemoveDomainAllow(ctx, "friend.example", moderator); err != nil || !removed {
		t.Fatalf("RemoveDomainAllow(friend.example) = %v, %v; want removed", removed, err)
	}
	if removed, err := svc.RemoveDomainAllow(ctx, "friend.example", moderator); err != nil || removed {
		t.Errorf("RemoveDomainAllow(friend.example again) = %v, %v; want nothing removed", removed, err)
	}
	if !suspended("friend.example") {
		t.Error("friend.example after its allow is removed is not suspended")
	}
	entries := audit(moderation.AuditDomainAllowRemoved)
	if len(entries) != 2 || entries[0].TargetID != "friend.example" || entries[0].SourceEventID == "" {
		t.Fatalf("domain_allow_removed entries = %+v, want friend.example's with its cutoff", entries)
	}
	c, err := e.h.Moderation.FindAllowlistCutoff(ctx, entries[0].SourceEventID)
	if err != nil || c.Cause != moderation.CutoffAllowRemoved || c.Domain != "friend.example" || c.IssuedByUserID != moderator {
		t.Errorf("cutoff of the removal = %+v, %v; want allow_removed of friend.example by the moderator", c, err)
	}
	gone, err := svc.GoneAuditTargets(ctx, entries)
	if err != nil || !gone[appmoderation.AuditTarget{Kind: moderation.AuditTargetDomainAllow, ID: "friend.example"}] {
		t.Errorf("GoneAuditTargets = %v, %v; want the removed allow gone", gone, err)
	}

	// Under open, a removal cuts nothing off.
	scopes.Scope = moderation.ScopeOpen
	if removed, err := svc.RemoveDomainAllow(ctx, "blocked.example", ""); err != nil || !removed {
		t.Fatalf("RemoveDomainAllow(blocked.example) = %v, %v", removed, err)
	}
	if got := audit(moderation.AuditDomainAllowRemoved); len(got) != 3 || got[0].SourceEventID != "" {
		t.Errorf("removal under open = %+v, want an entry without a cutoff", got[0])
	}

	// A switch to allowlist records a cutoff; setting allowlist again or open records none.
	switchTo := func(previous, scope moderation.FederationScope) {
		t.Helper()
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return svc.FederationScopeSwitchedTx(ctx, tx, previous, scope, moderator)
		})
		if err != nil {
			t.Fatalf("FederationScopeSwitchedTx(%s → %s): %v", previous, scope, err)
		}
	}
	switchTo(moderation.ScopeOpen, moderation.ScopeAllowlist)
	switchTo(moderation.ScopeAllowlist, moderation.ScopeAllowlist)
	switchTo(moderation.ScopeAllowlist, moderation.ScopeOpen)
	if impact, err := svc.FederationScopeImpact(ctx); err != nil || impact.Follows != 0 || impact.Scope != moderation.ScopeOpen {
		t.Errorf("FederationScopeImpact without the graph = %+v, %v; want nothing counted under open", impact, err)
	}
}
