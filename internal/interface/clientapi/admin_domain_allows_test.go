// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeDomainAllows keeps the allows in memory and records the removals.
type fakeDomainAllows struct {
	allows  []moderation.DomainAllow
	removed []string
	by      []string
}

func (f *fakeDomainAllows) DomainAllows(context.Context) ([]moderation.DomainAllow, error) {
	return f.allows, nil
}

func (f *fakeDomainAllows) AllowDomain(_ context.Context, in appmoderation.AllowDomainInput) (moderation.DomainAllow, bool, error) {
	f.by = append(f.by, in.ByUserID)
	if in.Domain == "self.example" {
		return moderation.DomainAllow{}, false, moderation.ErrOwnDomain
	}
	if len(in.Comment) > 10 {
		return moderation.DomainAllow{}, false, moderation.ErrInvalidDomainAllow
	}
	for _, a := range f.allows {
		if a.Domain == in.Domain {
			return a, false, nil
		}
	}
	a := moderation.DomainAllow{Domain: in.Domain, Comment: in.Comment, CreatedByUserID: in.ByUserID, CreatedAt: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)}
	f.allows = append(f.allows, a)
	return a, true, nil
}

func (f *fakeDomainAllows) RemoveDomainAllow(_ context.Context, domain, byUserID string) (bool, error) {
	f.removed, f.by = append(f.removed, domain), append(f.by, byUserID)
	return true, nil
}

func (f *fakeDomainAllows) FederationScopeImpact(context.Context) (appmoderation.ScopeImpact, error) {
	return appmoderation.ScopeImpact{
		Scope: moderation.ScopeOpen, Follows: 3, LocalAccounts: 2, Domains: 1,
		TopDomains: []appmoderation.DomainImpact{{Domain: "stranger.example", Follows: 3}},
	}, nil
}

// TestDomainAllowsPermissions lets the holders of manage_domains manage the allowlist and those
// of manage_settings read the impact of the switch, and nobody else.
func TestDomainAllowsPermissions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		have          []identity.Permission
		allows, scope int
	}{
		{"manage_domains", []identity.Permission{identity.PermManageDomains}, http.StatusOK, http.StatusForbidden},
		{"manage_settings", []identity.Permission{identity.PermManageSettings}, http.StatusForbidden, http.StatusOK},
		{"neither", []identity.Permission{identity.PermManageJobs}, http.StatusForbidden, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.DomainAllows = &fakeDomainAllows{}
				o.Permissions = fakePermissions{userOf(aliceAccountID): tc.have}
			})
			e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
			if res := e.send(t, http.MethodGet, "/admin/domain_allows", "admin-read", ""); res.StatusCode != tc.allows {
				t.Errorf("GET /admin/domain_allows = %d, want %d", res.StatusCode, tc.allows)
			}
			if res := e.send(t, http.MethodGet, "/admin/settings/federation_scope/impact", "admin-read", ""); res.StatusCode != tc.scope {
				t.Errorf("GET impact = %d, want %d", res.StatusCode, tc.scope)
			}
		})
	}
}

// TestDomainAllows adds a domain in canonical form (201, then 200 when it is already allowed),
// refuses a domain that is not a host (400), this instance's domain and a long comment (422),
// lists the allows, removes one as the viewer, and presents the impact of the switch.
func TestDomainAllows(t *testing.T) {
	allows := &fakeDomainAllows{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.DomainAllows = allows
		o.Permissions = fakePermissions{userOf(aliceAccountID): identity.AllPermissions()}
	})
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	e.auth.tokens["admin-write"] = bearer("admin:write", aliceAccountID)

	res := e.send(t, http.MethodPost, "/admin/domain_allows", "admin-write", `{"domain":"Friend.EXAMPLE","comment":"partner"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST = %d, want 201", res.StatusCode)
	}
	if got := decode[clientapi.DomainAllow](t, res); got.Domain != "friend.example" || got.Comment != "partner" {
		t.Errorf("POST body = %+v, want friend.example with its comment", got)
	}
	if res := e.send(t, http.MethodPost, "/admin/domain_allows", "admin-write", `{"domain":"friend.example"}`); res.StatusCode != http.StatusOK {
		t.Errorf("POST again = %d, want 200", res.StatusCode)
	}
	for body, want := range map[string]int{
		`{"domain":"not a host/x"}`:                         http.StatusBadRequest,
		`{"domain":"self.example"}`:                         http.StatusUnprocessableEntity,
		`{"domain":"long.example","comment":"0123456789x"}`: http.StatusUnprocessableEntity,
	} {
		if res := e.send(t, http.MethodPost, "/admin/domain_allows", "admin-write", body); res.StatusCode != want {
			t.Errorf("POST %s = %d, want %d", body, res.StatusCode, want)
		}
	}
	res = e.send(t, http.MethodGet, "/admin/domain_allows", "admin-read", "")
	if list := decode[clientapi.DomainAllowList](t, res); len(list.Items) != 1 || list.Items[0].Domain != "friend.example" {
		t.Errorf("GET = %+v, want friend.example", list)
	}
	if res := e.send(t, http.MethodDelete, "/admin/domain_allows/friend.example", "admin-write", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("DELETE = %d, want 204", res.StatusCode)
	}
	if len(allows.removed) != 1 || allows.removed[0] != "friend.example" || allows.by[len(allows.by)-1] != userOf(aliceAccountID) {
		t.Errorf("removed %v by %v, want friend.example by alice's user", allows.removed, allows.by)
	}
	res = e.send(t, http.MethodGet, "/admin/settings/federation_scope/impact", "admin-read", "")
	impact := decode[clientapi.FederationScopeImpact](t, res)
	if impact.Follows != 3 || impact.LocalAccounts != 2 || impact.Domains != 1 || len(impact.TopDomains) != 1 || impact.TopDomains[0].Domain != "stranger.example" {
		t.Errorf("impact = %+v", impact)
	}
}
