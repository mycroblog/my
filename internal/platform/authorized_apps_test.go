// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestAuthorizedApps lists and revokes the apps a user authorized on the real database: one row
// per client however many grants it has, the last use recorded once an hour, and a revoked app
// whose tokens stop working.
func TestAuthorizedApps(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h := app.Handler()
	spec := apitest.Load(t)
	ctx := t.Context()

	alice, err := app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{
		Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	call := func(method, path, token string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return spec.Serve(t, h, req)
	}
	list := func(token string) []clientapi.AuthorizedApp {
		t.Helper()
		res := call(http.MethodGet, "/api/v1/authorized_apps", token)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /authorized_apps = %d", res.StatusCode)
		}
		var body clientapi.AuthorizedAppList
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Items
	}

	// Two grants of the same client are one row.
	first := issueTokens(t, app, alice.User.ID, "")
	second := issueTokens(t, app, alice.User.ID, "")
	apps := list(first.AccessToken)
	if len(apps) != 1 || !apps[0].FirstParty || apps[0].Registration != clientapi.AuthorizedAppRegistrationFirstParty || apps[0].Id != first.Client.ID {
		t.Fatalf("authorized apps = %+v, want the web client once", apps)
	}
	if apps[0].LastUsedAt == nil {
		t.Fatal("last_used_at is absent after the first authenticated request")
	}

	// Further requests within the hour do not write again.
	used := *apps[0].LastUsedAt
	for range 5 {
		list(first.AccessToken)
	}
	if got := list(first.AccessToken)[0].LastUsedAt; got == nil || !got.Equal(used) {
		t.Errorf("last_used_at = %v after more requests, want %v (one write an hour)", got, used)
	}

	// Revoking ends every grant of the client.
	if res := call(http.MethodDelete, "/api/v1/authorized_apps/"+first.Client.ID, first.AccessToken); res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /authorized_apps/{id} = %d, want 204", res.StatusCode)
	}
	for name, tok := range map[string]string{"first": first.AccessToken, "second": second.AccessToken} {
		if res := call(http.MethodGet, "/api/v1/accounts/verify_credentials", tok); res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s token after the revocation = %d, want 401", name, res.StatusCode)
		}
	}
	listed, err := app.Identity.AuthorizedApps(ctx, alice.User.ID)
	if err != nil || len(listed) != 0 {
		t.Errorf("AuthorizedApps after the revocation = %+v, %v; want none", listed, err)
	}
	if err := app.Identity.RevokeAuthorizedApp(ctx, alice.User.ID, first.Client.ID); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("revoking an app without a grant = %v, want ErrNotFound", err)
	}
}
