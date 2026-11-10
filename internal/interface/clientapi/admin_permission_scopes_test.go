// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"

	appwebhook "github.com/mycroblog/my/internal/app/webhook"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/webhook"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// The permission scopes (ADR-0090 §17): a token of admin:<permission> stands in for admin:read
// and admin:write on the operations whose x-permission is that permission, and reaches nothing
// else.

// allPermissionsBut returns every permission except p.
func allPermissionsBut(p identity.Permission) []identity.Permission {
	return slices.DeleteFunc(identity.AllPermissions(), func(q identity.Permission) bool { return q == p })
}

// adminRequest is a request of the operation of pattern, with its path parameters filled in and
// a body or query that the generated code parses, so that the request reaches the handler.
func adminRequest(pattern, token string) *http.Request {
	method, path, _ := strings.Cut(pattern, " ")
	path = regexp.MustCompile(`\{[^}]+\}`).ReplaceAllStringFunc(path, func(p string) string {
		switch {
		case strings.Contains(p, "domain"), strings.Contains(p, "host"):
			return "a.example"
		case strings.Contains(p, "role"), strings.Contains(p, "kind"), strings.Contains(p, "key"), strings.Contains(p, "name"), strings.Contains(p, "shortcode"), strings.Contains(p, "source"):
			return "x"
		}
		return deadJobA
	})
	body, contentType := "{}", "application/json"
	switch {
	case strings.HasSuffix(path, "/cost_totals"), strings.HasSuffix(path, "/income_totals"):
		path += "?granularity=month&from=2026-01-01&to=2026-02-01"
	case strings.Contains(path, "/instance/brand/") && method != http.MethodDelete:
		body, contentType = "--b--\r\n", "multipart/form-data; boundary=b"
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept-Language", "en")
	return req
}

// TestAdminOperationsDeclareTheirPermission checks that the x-permission of every admin operation
// of a permission with a permission scope is the permission its handler checks: a user with
// every other permission gets the 403 of that permission. A wrong declaration would let in the
// token of another permission scope, which requirePermission then refuses, and keep out the
// token it should serve. The declarations of the permissions without a scope admit no token.
func TestAdminOperationsDeclareTheirPermission(t *testing.T) {
	declared := 0
	for _, pattern := range clientapi.GuardedPatterns() {
		op, perm := clientapi.OperationPermission(pattern)
		// getLoginProvidersOnlyImpact also serves manage_login_providers, which has no scope.
		if _, scoped := identity.PermissionScope(perm); !scoped || op == "getLoginProvidersOnlyImpact" {
			continue
		}
		declared++
		t.Run(op, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.Permissions = fakePermissions{userOf(carolAccountID): allPermissionsBut(perm)}
			})
			e.auth.tokens["carol-admin"] = bearer("admin", carolAccountID)
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, adminRequest(pattern, "carol-admin"))
			if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), clientapi.ProblemTypeMissingPermission) ||
				!strings.Contains(rec.Body.String(), string(perm)) {
				t.Errorf("%s without %s = %d %s; want the 403 of %s", pattern, perm, rec.Code, rec.Body, perm)
			}
		})
	}
	if declared < 120 {
		t.Errorf("%d operations declare x-permission, want every admin operation with one permission", declared)
	}
}

// newPermissionScopeEnv is newInvitesEnv with carol, whose roles hold every permission, and her
// third-party tokens of permission scopes.
func newPermissionScopeEnv(t *testing.T, perms []identity.Permission, tweak func(*clientapi.Options)) (*authEnv, *fakeInvites) {
	t.Helper()
	f := &fakeInvites{}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{userOf(carolAccountID): perms}
		o.Invites, o.SignUpPath = f, "/auth/sign_up"
		if tweak != nil {
			tweak(o)
		}
	})
	e.auth.tokens["invites"] = thirdParty(bearer("admin:manage_invites", carolAccountID))
	e.auth.tokens["webhooks"] = thirdParty(bearer("admin:manage_webhooks", carolAccountID))
	e.auth.tokens["webhooks-reports"] = thirdParty(bearer("admin:manage_reports admin:manage_webhooks", carolAccountID))
	return e, f
}

func TestPermissionScopeCallsTheOperationsOfItsPermission(t *testing.T) {
	e, f := newPermissionScopeEnv(t, identity.AllPermissions(), nil)
	res := e.sendJSON(t, http.MethodPost, "/admin/invites", "invites", `{"max_uses":1,"expires_in":604800}`)
	created := decodeJSON[clientapi.AdminInvite](t, res)
	if res.StatusCode != http.StatusCreated || len(f.creates) != 1 || f.creates[0].CreatedByUserID != userOf(carolAccountID) {
		t.Fatalf("create with admin:manage_invites = %d %+v", res.StatusCode, f.creates)
	}
	if res := e.admin(t, http.MethodGet, "/admin/invites", "invites", nil); res.StatusCode != http.StatusOK {
		t.Errorf("list with admin:manage_invites = %d, want 200", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/admin/invites/"+created.Id+"/revoke", "invites", ""); res.StatusCode != http.StatusOK {
		t.Errorf("revoke with admin:manage_invites = %d, want 200", res.StatusCode)
	}

	// Another permission's operation, and an operation that names none, are out of reach,
	// although carol's roles hold every permission.
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/accounts"},
		{http.MethodGet, "/admin/counts"},
		{http.MethodGet, "/admin/cost_items"},
	} {
		res := e.admin(t, op.method, op.path, "invites", nil)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope {
			t.Errorf("%s %s with admin:manage_invites = %d %+v, want 403 insufficient-scope", op.method, op.path, res.StatusCode, p)
		}
	}
}

func TestPermissionScopeNeedsThePermissionOnEveryCall(t *testing.T) {
	e, f := newPermissionScopeEnv(t, allPermissionsBut(identity.PermManageInvites), nil)
	res := e.sendJSON(t, http.MethodPost, "/admin/invites", "invites", `{}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeMissingPermission {
		t.Errorf("create after losing manage_invites = %d %+v, want 403 missing-permission", res.StatusCode, p)
	}
	if len(f.creates) != 0 {
		t.Error("a refused request created an invite")
	}
}

// fakeWebhooks records the endpoints created; it implements only Create.
type fakeWebhooks struct {
	clientapi.Webhooks
	created []webhook.EndpointSettings
}

func (f *fakeWebhooks) Create(_ context.Context, s webhook.EndpointSettings, _ string) (appwebhook.Created, error) {
	f.created = append(f.created, s)
	e := webhook.Endpoint{ID: deadJobA, Settings: s, CreatedAt: adminNow, RevisedAt: adminNow, State: webhook.StateEnabled, StateReason: webhook.ReasonAdmin, StateChangedAt: adminNow}
	return appwebhook.Created{Endpoint: e, Secret: "secret"}, nil
}

// TestPermissionScopeWebhooksShowOnlyItsPermissions checks that a token of admin:manage_webhooks
// subscribes an endpoint only to the events that its permission scopes show.
func TestPermissionScopeWebhooksShowOnlyItsPermissions(t *testing.T) {
	hooks := &fakeWebhooks{}
	e, _ := newPermissionScopeEnv(t, identity.AllPermissions(), func(o *clientapi.Options) { o.Webhooks = hooks })
	report := `{"url":"https://hooks.example/in","events":["report.created"]}`
	res := e.sendJSON(t, http.MethodPost, "/admin/webhooks", "webhooks", report)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope ||
		p.Detail == nil || !strings.Contains(*p.Detail, "admin:manage_reports") {
		t.Errorf("report events with admin:manage_webhooks = %d %+v, want 403 for admin:manage_reports", res.StatusCode, p)
	}
	if res := e.sendJSON(t, http.MethodPost, "/admin/webhooks", "webhooks", `{"url":"https://hooks.example/in","events":["status.created"]}`); res.StatusCode != http.StatusCreated {
		t.Errorf("status events with admin:manage_webhooks = %d, want 201", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, "/admin/webhooks", "webhooks-reports", report); res.StatusCode != http.StatusCreated {
		t.Errorf("report events with admin:manage_reports admin:manage_webhooks = %d, want 201", res.StatusCode)
	}
	if len(hooks.created) != 2 {
		t.Errorf("created = %+v, want the two allowed endpoints", hooks.created)
	}
}
