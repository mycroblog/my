// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	domidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestAdminLoginProviders runs the admin API of the external login providers through the
// composition root on SQLite: the operations need manage_login_providers; settings and secrets
// that break a rule are 422s with their reason; no answer carries the client secret, which is
// sealed so that it opens with its row's AAD; the answers carry the callback URL and the number
// of linked users; and every change is audited, a change to nothing not.
func TestAdminLoginProviders(t *testing.T) {
	t.Parallel()
	dbPath := newDatabase(t, true)
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": dbPath, "MY_WEB_ORIGIN": webOrigin, "MY_QUEUE": "inline"})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h, spec, ctx := app.Handler(), apitest.Load(t), t.Context()
	admin, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	adminToken := issueAccessToken(t, app, admin.User.ID, "read write admin")
	bob, err := app.Identity.RegisterLocalUser(ctx, identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bobToken := issueAccessToken(t, app, bob.User.ID, "read write admin")

	const secret, rotated = "first-client-secret-value", "second-client-secret-value"
	api := func(token, method, path string, body any, out any) int {
		t.Helper()
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, clientapi.BasePath+path, r)
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res := spec.Serve(t, h, req)
		raw, _ := io.ReadAll(res.Body)
		if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte(rotated)) {
			t.Errorf("%s %s answered with the client secret: %s", method, path, raw)
		}
		if out != nil {
			if err := json.Unmarshal(raw, out); err != nil {
				t.Fatalf("%s %s: %v: %s", method, path, err, raw)
			}
		}
		return res.StatusCode
	}

	// Without manage_login_providers, nothing.
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/login_providers"},
		{http.MethodPost, "/admin/login_providers"},
		{http.MethodPost, "/admin/login_providers/01890a5d-ac96-774b-bcce-b302099a0000/disable"},
	} {
		var body any
		if op.method == http.MethodPost && op.path == "/admin/login_providers" {
			body = map[string]any{"kind": "github", "display_name": "GitHub", "client_id": "c", "client_secret": secret}
		}
		if code := api(bobToken, op.method, op.path, body, nil); code != http.StatusForbidden {
			t.Errorf("%s %s without manage_login_providers = %d, want 403", op.method, op.path, code)
		}
	}

	// Settings and secrets that break a rule.
	for _, tc := range []struct {
		body map[string]any
		want clientapi.LoginProviderProblemReason
	}{
		{map[string]any{"kind": "oidc", "display_name": "SSO", "issuer": "http://sso.example", "client_id": "c", "client_secret": secret}, clientapi.LoginProviderProblemReasonIssuer},
		{map[string]any{"kind": "github", "display_name": "", "client_id": "c", "client_secret": secret}, clientapi.LoginProviderProblemReasonDisplayName},
		{map[string]any{"kind": "github", "display_name": "GitHub", "client_id": "c", "client_secret": " "}, clientapi.LoginProviderProblemReasonClientSecret},
		{map[string]any{"kind": "mastodon", "display_name": "M", "issuer": "mastodon.example", "client_id": "c", "client_secret": secret, "scopes": []string{"read"}}, clientapi.LoginProviderProblemReasonScopes},
		{map[string]any{"kind": "github", "display_name": "GitHub", "client_id": "c", "client_secret": secret, "claims": []map[string]any{{"claim": "hd", "values": []string{"example.com"}}}}, clientapi.LoginProviderProblemReasonConditions},
	} {
		var problem clientapi.LoginProviderProblem
		if code := api(adminToken, http.MethodPost, "/admin/login_providers", tc.body, &problem); code != http.StatusUnprocessableEntity ||
			problem.Reason == nil || *problem.Reason != tc.want || problem.Type != clientapi.ProblemTypeInvalidLoginProvider {
			t.Errorf("POST %v = %d %+v, want reason %s", tc.body, code, problem, tc.want)
		}
	}

	var created clientapi.AdminLoginProvider
	if code := api(adminToken, http.MethodPost, "/admin/login_providers", map[string]any{
		"kind": "oidc", "display_name": "Example SSO", "issuer": "https://sso.example", "client_id": "my-client",
		"client_secret": secret, "claims": []map[string]any{{"claim": "hd", "values": []string{"example.org", "example.com"}}},
	}, &created); code != http.StatusOK {
		t.Fatalf("POST /admin/login_providers = %d", code)
	}
	id := created.Id
	if created.Kind != clientapi.LoginProviderKindOidc || created.State != clientapi.AdminLoginProviderStateEnabled ||
		created.CallbackUrl != "https://example.test/auth/external/"+id+"/callback" || created.LinkedUsers != 0 ||
		created.AllowSignup || !created.AllowLink || len(created.Claims) != 1 || len(created.Claims[0].Values) != 2 ||
		created.Claims[0].Values[0] != "example.com" || created.Issuer == nil || *created.Issuer != "https://sso.example" {
		t.Errorf("created = %+v", created)
	}

	// The secret is sealed with the AAD of its row and column.
	openSecret := func() string {
		t.Helper()
		db, err := sqlite.Open(ctx, dbPath, uow.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		s, err := db.IdentityRepository().FindLoginProviderSecret(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := app.Sealer.Open(ctx, s.Sealed, kernel.AAD(s.ID, domidentity.LoginProviderSecretColumn))
		if err != nil {
			t.Fatalf("open the client secret: %v", err)
		}
		return string(plain)
	}
	if got := openSecret(); got != secret {
		t.Errorf("sealed secret = %q, want %q", got, secret)
	}

	var got clientapi.AdminLoginProvider
	if code := api(adminToken, http.MethodPatch, "/admin/login_providers/"+id, map[string]any{"kind": "github"}, &got); code != http.StatusOK || got.DisplayName != "Example SSO" {
		t.Errorf("PATCH with only an unknown field = %d %+v", code, got)
	}
	if code := api(adminToken, http.MethodPatch, "/admin/login_providers/"+id, map[string]any{
		"display_name": "Corp SSO", "allow_signup": true, "claims": []any{},
	}, &got); code != http.StatusOK || got.DisplayName != "Corp SSO" || !got.AllowSignup || len(got.Claims) != 0 {
		t.Errorf("PATCH = %d %+v", code, got)
	}
	var problem clientapi.LoginProviderProblem
	if code := api(adminToken, http.MethodPatch, "/admin/login_providers/"+id, map[string]any{"organizations": []string{"acme"}}, &problem); code != http.StatusUnprocessableEntity ||
		problem.Reason == nil || *problem.Reason != clientapi.LoginProviderProblemReasonConditions {
		t.Errorf("PATCH organizations of oidc = %d %+v", code, problem)
	}
	if code := api(adminToken, http.MethodPost, "/admin/login_providers/"+id+"/rotate_secret", map[string]any{"client_secret": ""}, &problem); code != http.StatusUnprocessableEntity ||
		problem.Reason == nil || *problem.Reason != clientapi.LoginProviderProblemReasonClientSecret {
		t.Errorf("rotate an empty secret = %d %+v", code, problem)
	}
	if code := api(adminToken, http.MethodPost, "/admin/login_providers/"+id+"/rotate_secret", map[string]any{"client_secret": rotated}, &got); code != http.StatusOK {
		t.Errorf("rotate_secret = %d", code)
	}
	if got := openSecret(); got != rotated {
		t.Errorf("sealed secret after the rotation = %q, want %q", got, rotated)
	}
	if code := api(adminToken, http.MethodPost, "/admin/login_providers/"+id+"/disable", nil, &got); code != http.StatusOK ||
		got.State != clientapi.AdminLoginProviderStateDisabled || got.LinkedUsers != 0 {
		t.Errorf("disable = %d %+v", code, got)
	}
	if code := api(adminToken, http.MethodPost, "/admin/login_providers/"+id+"/disable", nil, &got); code != http.StatusOK {
		t.Errorf("disable again = %d", code)
	}
	if code := api(adminToken, http.MethodPost, "/admin/login_providers/"+id+"/enable", nil, &got); code != http.StatusOK ||
		got.State != clientapi.AdminLoginProviderStateEnabled {
		t.Errorf("enable = %d %+v", code, got)
	}
	var list clientapi.AdminLoginProviderList
	if code := api(adminToken, http.MethodGet, "/admin/login_providers", nil, &list); code != http.StatusOK || len(list.Items) != 1 || list.Items[0].Id != id {
		t.Errorf("GET /admin/login_providers = %d %+v", code, list)
	}
	if code := api(adminToken, http.MethodDelete, "/admin/login_providers/"+id, nil, nil); code != http.StatusNoContent {
		t.Errorf("DELETE = %d", code)
	}
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/admin/login_providers/" + id},
		{http.MethodDelete, "/admin/login_providers/" + id},
		{http.MethodPost, "/admin/login_providers/" + id + "/enable"},
	} {
		if code := api(adminToken, op.method, op.path, nil, nil); code != http.StatusNotFound {
			t.Errorf("%s %s after the deletion = %d, want 404", op.method, op.path, code)
		}
	}

	var audit clientapi.AdminAuditEntryPage
	if code := api(adminToken, http.MethodGet, "/admin/audit_log?target_kind=login_provider&target_id="+id, nil, &audit); code != http.StatusOK {
		t.Fatalf("GET the audit log = %d", code)
	}
	want := []clientapi.AdminAuditAction{
		clientapi.AdminAuditActionLoginProviderDeleted, clientapi.AdminAuditActionLoginProviderEnabled,
		clientapi.AdminAuditActionLoginProviderDisabled, clientapi.AdminAuditActionLoginProviderSecretRotated,
		clientapi.AdminAuditActionLoginProviderUpdated, clientapi.AdminAuditActionLoginProviderCreated,
	}
	if len(audit.Items) != len(want) {
		t.Fatalf("audit log = %+v, want %v", audit.Items, want)
	}
	for i, e := range audit.Items {
		if e.Action != want[i] || e.ActorKind != clientapi.AdminAuditActorKindUser {
			t.Errorf("audit entry %d = %+v, want %s by a user", i, e, want[i])
		}
	}
	if audit.Items[5].TargetLabel != "Example SSO" || audit.Items[0].TargetLabel != "Corp SSO" {
		t.Errorf("audit labels = %q, %q", audit.Items[5].TargetLabel, audit.Items[0].TargetLabel)
	}
}
