// SPDX-License-Identifier: AGPL-3.0-only

package identity_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/domain/identity"
)

func TestKnownScopesAreSortedAndParse(t *testing.T) {
	known := identity.KnownScopes()
	if !slices.IsSorted(known) {
		t.Errorf("KnownScopes = %v, want sorted", known)
	}
	for _, s := range known {
		if got, err := identity.ParseScope(s); err != nil || string(got) != s {
			t.Errorf("ParseScope(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := identity.ParseScope("read:everything"); !errors.Is(err, identity.ErrInvalidScope) {
		t.Errorf("ParseScope(read:everything) = %v, want ErrInvalidScope", err)
	}
}

func TestScopeGrants(t *testing.T) {
	tests := []struct {
		scope identity.Scope
		want  string
		ok    bool
	}{
		{"read", identity.ScopeReadAccounts, true},
		{"read", identity.ScopeRead, true},
		{"read", identity.ScopeWriteStatuses, false},
		{"write", identity.ScopeWriteMedia, true},
		{"write", identity.ScopeReadStatuses, false},
		{"follow", identity.ScopeReadFollows, true},
		{"follow", identity.ScopeWriteMutes, true},
		{"follow", identity.ScopeWriteStatuses, false},
		{"follow", identity.ScopeReadAccounts, false},
		{"read:accounts", identity.ScopeReadAccounts, true},
		{"read:accounts", identity.ScopeRead, false},
		{"read:accounts", identity.ScopeReadStatuses, false},
		{"read", identity.ScopeReadDirect, true},
		{"read:statuses", identity.ScopeReadDirect, false},
		{"read:direct", identity.ScopeReadStatuses, false},
		{"write", identity.ScopeReadDirect, false},
		{"push", identity.ScopeReadNotifications, false},
		{"", identity.ScopeReadAccounts, false},
	}
	for _, tt := range tests {
		if got := tt.scope.Grants(tt.want); got != tt.ok {
			t.Errorf("Scope(%q).Grants(%q) = %v, want %v", tt.scope, tt.want, got, tt.ok)
		}
	}
}

func TestScopeWithinFollowsBroadScopes(t *testing.T) {
	if !identity.Scope("read:accounts write:statuses").Within(identity.AllScopes()) {
		t.Error("fine-grained scopes are not within AllScopes")
	}
	if identity.Scope("read write").Within("read:accounts write") {
		t.Error("read is within read:accounts")
	}
	for _, s := range identity.KnownScopes() {
		if !identity.Scope(s).Within(identity.AllScopes()) {
			t.Errorf("%s is not within AllScopes", s)
		}
	}
}

func TestFirstPartyOnlyScopes(t *testing.T) {
	for _, s := range []string{"admin", "admin:read", "admin:write", "write:supporters"} {
		if !identity.FirstPartyOnlyScope(s) {
			t.Errorf("FirstPartyOnlyScope(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"read", "write", "write:statuses", "read:supporters", "read:direct", "follow"} {
		if identity.FirstPartyOnlyScope(s) {
			t.Errorf("FirstPartyOnlyScope(%q) = true, want false", s)
		}
	}
	if identity.Scope("read write").HasFirstPartyOnly() {
		t.Error("read write names no first-party-only scope itself")
	}
	if !identity.Scope("read admin:read").HasFirstPartyOnly() {
		t.Error("read admin:read names a first-party-only scope")
	}
}

// TestPermissionScopes checks the permission scopes (ADR-0090 §17): one per moderation and
// server administration permission but the unscoped ones, known to ParseScope, granted by the
// broad admin alone, and only for self-issued tokens.
func TestPermissionScopes(t *testing.T) {
	for _, p := range identity.AllPermissions() {
		scope, ok := identity.PermissionScope(p)
		unscoped := p.Category() == identity.CategoryMember || slices.Contains([]identity.Permission{
			identity.PermManageRoles, identity.PermManageOAuthApps, identity.PermManageLoginRecovery,
			identity.PermManageLoginProviders, identity.PermManageServiceClosure,
		}, p)
		if ok == unscoped {
			t.Errorf("PermissionScope(%s) = %q, %v; want a scope %v", p, scope, ok, !unscoped)
			continue
		}
		if !ok {
			if _, err := identity.ParseScope("admin:" + string(p)); err == nil {
				t.Errorf("ParseScope(admin:%s) accepted a permission without a scope", p)
			}
			continue
		}
		if scope != "admin:"+string(p) || !slices.Contains(identity.KnownScopes(), scope) || !identity.SelfIssuedOnlyScope(scope) || identity.FirstPartyOnlyScope(scope) {
			t.Errorf("scope of %s = %q: want admin:%s, known, self-issued only, not first-party only", p, scope, p)
		}
		if got, ok := identity.ScopePermission(scope); !ok || got != p {
			t.Errorf("ScopePermission(%s) = %s, %v", scope, got, ok)
		}
		if !identity.Scope("admin").Grants(scope) || identity.Scope("admin:read admin:write read write").Grants(scope) {
			t.Errorf("%s: want it granted by admin alone", scope)
		}
	}
	for _, t2 := range []string{"admin:read", "admin:write", "admin", "read:accounts", "admin:nothing"} {
		if identity.SelfIssuedOnlyScope(t2) {
			t.Errorf("SelfIssuedOnlyScope(%s) = true", t2)
		}
	}
	s := identity.Scope("admin:manage_invites read")
	if !s.HasSelfIssuedOnly() || s.WithoutSelfIssuedOnly() != "read" || identity.Scope("read").HasSelfIssuedOnly() {
		t.Errorf("HasSelfIssuedOnly / WithoutSelfIssuedOnly of %q = %v, %q", s, s.HasSelfIssuedOnly(), s.WithoutSelfIssuedOnly())
	}
}
