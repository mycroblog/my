// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeOAuthApps keeps the OAuth apps of their owners in memory and validates their settings as
// the identity module does. err, when set, fails every registration.
type fakeOAuthApps struct {
	apps     map[string]appidentity.OAuthApp
	err      error
	disabled bool
	calls    []string
}

var oauthAppCreatedAt = time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)

func (f *fakeOAuthApps) own(userID, id string) (appidentity.OAuthApp, error) {
	a, ok := f.apps[id]
	if !ok || !a.Client.OwnedBy(userID) {
		return appidentity.OAuthApp{}, appidentity.ErrNotFound
	}
	return a, nil
}

func (f *fakeOAuthApps) OAuthApps(_ context.Context, userID string) ([]appidentity.OAuthApp, error) {
	var out []appidentity.OAuthApp
	for _, a := range f.apps {
		if a.Client.OwnedBy(userID) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeOAuthApps) OAuthApp(_ context.Context, userID, id string) (appidentity.OAuthApp, error) {
	return f.own(userID, id)
}

func (f *fakeOAuthApps) RegisterOAuthApp(_ context.Context, userID string, in appidentity.OAuthAppInput) (appidentity.RegisteredOAuthApp, error) {
	if f.err != nil {
		return appidentity.RegisteredOAuthApp{}, f.err
	}
	s, err := identity.ValidateOAuthClientSettings(in.Name, in.Website, in.RedirectURIs, in.Scope)
	if err != nil {
		return appidentity.RegisteredOAuthApp{}, err
	}
	id := testkit.NamedID("app-" + s.Name)
	c := identity.OAuthClient{
		ID: id, ClientID: "cid-" + s.Name, Name: s.Name, Website: s.Website, Scope: s.Scope,
		Registration: identity.ClientRegisteredByUser, OwnerUserID: userID, CreatedAt: oauthAppCreatedAt,
	}
	var secret string
	if in.Confidential {
		secret = appidentity.ClientSecretPrefix + "secret"
		c.SecretHash = appidentity.HashToken(secret)
	}
	a := appidentity.OAuthApp{Client: c, RedirectURIs: s.RedirectURIs, Disabled: f.disabled}
	if f.apps == nil {
		f.apps = map[string]appidentity.OAuthApp{}
	}
	f.apps[id] = a
	return appidentity.RegisteredOAuthApp{OAuthApp: a, ClientSecret: secret}, nil
}

func (f *fakeOAuthApps) UpdateOAuthApp(_ context.Context, userID, id string, ch appidentity.OAuthAppChanges) (appidentity.OAuthApp, error) {
	a, err := f.own(userID, id)
	if err != nil {
		return appidentity.OAuthApp{}, err
	}
	name, website, uris, scope := a.Client.Name, a.Client.Website, a.RedirectURIs, string(a.Client.Scope)
	if ch.Name != nil {
		name = *ch.Name
	}
	if ch.Website != nil {
		website = *ch.Website
	}
	if ch.RedirectURIs != nil {
		uris = ch.RedirectURIs
	}
	if ch.Scope != nil {
		scope = *ch.Scope
	}
	s, err := identity.ValidateOAuthClientSettings(name, website, uris, scope)
	if err != nil {
		return appidentity.OAuthApp{}, err
	}
	a.Client.Name, a.Client.Website, a.Client.Scope, a.RedirectURIs = s.Name, s.Website, s.Scope, s.RedirectURIs
	f.apps[id] = a
	return a, nil
}

func (f *fakeOAuthApps) DeleteOwnOAuthApp(_ context.Context, userID, id string) error {
	if _, err := f.own(userID, id); err != nil {
		return err
	}
	delete(f.apps, id)
	f.calls = append(f.calls, "delete "+id)
	return nil
}

func (f *fakeOAuthApps) RotateOAuthAppSecret(_ context.Context, userID, id string) (string, error) {
	a, err := f.own(userID, id)
	if err != nil {
		return "", err
	}
	if a.Client.Public() {
		return "", appidentity.ErrPublicOAuthApp
	}
	return appidentity.ClientSecretPrefix + "rotated", nil
}

func (f *fakeOAuthApps) IssueOAuthAppTokens(_ context.Context, userID, id, scope string) (appidentity.IssuedTokens, error) {
	a, err := f.own(userID, id)
	if err != nil {
		return appidentity.IssuedTokens{}, err
	}
	if a.Disabled {
		return appidentity.IssuedTokens{}, appidentity.ErrOAuthAppDisabled
	}
	granted, err := identity.ParseScope(scope)
	if err != nil || !granted.Within(a.Client.Scope) {
		return appidentity.IssuedTokens{}, &appidentity.OAuthError{Code: appidentity.OAuthInvalidScope}
	}
	if granted == "" {
		granted = a.Client.Scope
	}
	return appidentity.IssuedTokens{
		AccessToken: appidentity.AccessTokenPrefix + "a", RefreshToken: appidentity.RefreshTokenPrefix + "r", Client: a.Client,
		Token: identity.AccessToken{
			ID: testkit.NamedID("token"), ClientID: id, UserID: userID, Scope: granted,
			IssuedAt: oauthAppCreatedAt, ExpiresAt: oauthAppCreatedAt.Add(appidentity.AccessTokenTTL),
		},
	}, nil
}

func newOAuthAppEnv(t *testing.T) (*authEnv, *fakeOAuthApps) {
	t.Helper()
	f := &fakeOAuthApps{}
	// Without a RateLimiter, so that the many registrations of a test stay under no limit.
	return newAuthEnv(t, func(o *clientapi.Options) { o.OAuthApps = f; o.RateLimiter = nil }), f
}

const confidentialAppBody = `{"name":"Bot","website":"https://bot.example/","redirect_uris":["https://bot.example/cb"],"scope":"read write:statuses","client_type":"confidential"}`

func TestOAuthAppRegistration(t *testing.T) {
	e, f := newOAuthAppEnv(t)
	res := e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", confidentialAppBody)
	created := decodeJSON[clientapi.OAuthAppCreated](t, res)
	if res.StatusCode != http.StatusCreated || created.ClientSecret == nil || *created.ClientSecret != appidentity.ClientSecretPrefix+"secret" ||
		created.App.ClientType != clientapi.OAuthAppClientTypeConfidential || created.App.Scope != "read write:statuses" ||
		created.App.Website == nil || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("create = %d %+v, want the confidential app with its secret", res.StatusCode, created)
	}
	id := created.App.Id

	// The secret is in the registration only.
	res = e.send(t, http.MethodGet, "/oauth_apps/"+id, "read-accounts", "")
	if body, _ := io.ReadAll(res.Body); res.StatusCode != http.StatusOK || strings.Contains(string(body), "secret") {
		t.Errorf("get = %d %s, want the app without its secret", res.StatusCode, body)
	}
	res = e.sendJSON(t, http.MethodPost, "/oauth_apps", "write",
		`{"name":"Script","redirect_uris":["http://127.0.0.1/cb"],"scope":"read","client_type":"public"}`)
	if pub := decodeJSON[clientapi.OAuthAppCreated](t, res); res.StatusCode != http.StatusCreated || pub.ClientSecret != nil ||
		pub.App.ClientType != clientapi.OAuthAppClientTypePublic || pub.App.Website != nil {
		t.Errorf("create public = %d %+v, want no secret", res.StatusCode, pub)
	}
	res = e.send(t, http.MethodGet, "/oauth_apps", "read-accounts", "")
	if list := decodeJSON[clientapi.OAuthAppList](t, res); res.StatusCode != http.StatusOK || len(list.Items) != 2 {
		t.Errorf("list = %d %+v, want both apps", res.StatusCode, list)
	}
	if res := e.send(t, http.MethodGet, "/oauth_apps/"+id, "bob", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("get by another user = %d, want 404", res.StatusCode)
	}

	for _, tc := range []struct{ body, param string }{
		{`{"name":"","redirect_uris":["https://bot.example/cb"],"scope":"read","client_type":"public"}`, "name"},
		{`{"name":"Bot","redirect_uris":["http://localhost/cb"],"scope":"read","client_type":"public"}`, "redirect_uris"},
		{`{"name":"Bot","website":"ftp://bot.example/","redirect_uris":["https://bot.example/cb"],"scope":"read","client_type":"public"}`, "website"},
		{`{"name":"Bot","redirect_uris":["https://bot.example/cb"],"scope":"admin:read","client_type":"public"}`, "scope"},
	} {
		res := e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", tc.body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, tc.param) {
			t.Errorf("create %s = %d %+v, want a 422 naming %s", tc.body, res.StatusCode, p, tc.param)
		}
	}

	for _, tc := range []struct {
		err    error
		status int
		typ    string
	}{
		{appidentity.ErrTooManyOAuthApps, http.StatusConflict, clientapi.ProblemTypeTooManyOAuthApps},
		{appidentity.ErrOAuthAppRegistrationDisabled, http.StatusForbidden, clientapi.ProblemTypeOAuthAppRegistrationDisabled},
	} {
		f.err = tc.err
		res := e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", confidentialAppBody)
		if p := decodeProblem(t, res); res.StatusCode != tc.status || p.Type != tc.typ {
			t.Errorf("create with %v = %d %+v, want %d %s", tc.err, res.StatusCode, p, tc.status, tc.typ)
		}
	}
}

func TestOAuthAppChanges(t *testing.T) {
	e, f := newOAuthAppEnv(t)
	created := decodeJSON[clientapi.OAuthAppCreated](t, e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", confidentialAppBody))
	id := created.App.Id

	res := e.sendJSON(t, http.MethodPatch, "/oauth_apps/"+id, "write", `{"name":"Renamed","website":"","redirect_uris":["com.example.bot:/cb"]}`)
	if app := decodeJSON[clientapi.OAuthApp](t, res); res.StatusCode != http.StatusOK || app.Name != "Renamed" || app.Website != nil ||
		len(app.RedirectUris) != 1 || app.RedirectUris[0] != "com.example.bot:/cb" || app.Scope != "read write:statuses" {
		t.Errorf("update = %d %+v, want the changes and the same scope", res.StatusCode, app)
	}
	res = e.sendJSON(t, http.MethodPatch, "/oauth_apps/"+id, "write", `{"scope":"write admin"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "scope") {
		t.Errorf("update with a first-party-only scope = %d %+v, want a 422 naming scope", res.StatusCode, p)
	}

	res = e.send(t, http.MethodPost, "/oauth_apps/"+id+"/secret", "write", "")
	if s := decodeJSON[clientapi.OAuthAppSecret](t, res); res.StatusCode != http.StatusOK || s.ClientSecret != appidentity.ClientSecretPrefix+"rotated" {
		t.Errorf("rotate = %d %+v, want the new secret", res.StatusCode, s)
	}

	res = e.sendJSON(t, http.MethodPost, "/oauth_apps/"+id+"/tokens", "write", `{"scope":"read"}`)
	tok := decodeJSON[clientapi.OAuthAppToken](t, res)
	if res.StatusCode != http.StatusOK || tok.AccessToken == "" || tok.RefreshToken == "" || tok.TokenType != clientapi.OAuthAppTokenTokenTypeBearer ||
		tok.ExpiresIn != 3600 || tok.Scope != "read" || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("issue tokens = %d %+v, want a pair for an hour", res.StatusCode, tok)
	}
	res = e.send(t, http.MethodPost, "/oauth_apps/"+id+"/tokens", "write", "")
	if tok := decodeJSON[clientapi.OAuthAppToken](t, res); res.StatusCode != http.StatusOK || tok.Scope != "read write:statuses" {
		t.Errorf("issue tokens without a body = %d %+v, want the app's scope", res.StatusCode, tok)
	}
	res = e.sendJSON(t, http.MethodPost, "/oauth_apps/"+id+"/tokens", "write", `{"scope":"follow"}`)
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Detail == nil || !strings.Contains(*p.Detail, "scope") {
		t.Errorf("issue tokens beyond the app = %d %+v, want a 422 naming scope", res.StatusCode, p)
	}

	if res := e.send(t, http.MethodDelete, "/oauth_apps/"+id, "write", ""); res.StatusCode != http.StatusNoContent || len(f.calls) != 1 {
		t.Errorf("delete = %d (calls %v), want 204", res.StatusCode, f.calls)
	}
	if res := e.send(t, http.MethodGet, "/oauth_apps/"+id, "read-accounts", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("get deleted = %d, want 404", res.StatusCode)
	}

	// A public app has no secret, and a disabled app issues no tokens.
	f.disabled = true
	pub := decodeJSON[clientapi.OAuthAppCreated](t, e.sendJSON(t, http.MethodPost, "/oauth_apps", "write",
		`{"name":"Script","redirect_uris":["http://127.0.0.1/cb"],"scope":"read","client_type":"public"}`))
	res = e.send(t, http.MethodPost, "/oauth_apps/"+pub.App.Id+"/secret", "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypePublicOAuthApp {
		t.Errorf("rotate a public app = %d %+v, want 409", res.StatusCode, p)
	}
	res = e.send(t, http.MethodPost, "/oauth_apps/"+pub.App.Id+"/tokens", "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeOAuthAppDisabled {
		t.Errorf("issue tokens of a disabled app = %d %+v, want 409", res.StatusCode, p)
	}
	if !pub.App.Disabled {
		t.Errorf("app = %+v, want it disabled", pub.App)
	}
}

func TestOAuthAppRegistrationIsRateLimited(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) { o.OAuthApps = &fakeOAuthApps{} })
	body := `{"name":"Script","redirect_uris":["http://127.0.0.1/cb"],"scope":"read","client_type":"public"}`
	for i := range clientapi.OAuthAppRegisterLimit.Max {
		if res := e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", body); res.StatusCode != http.StatusCreated {
			t.Fatalf("registration %d = %d, want 201", i, res.StatusCode)
		}
	}
	res := e.sendJSON(t, http.MethodPost, "/oauth_apps", "write", body)
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Errorf("registration over the limit = %d, Retry-After %q, want 429 with Retry-After", res.StatusCode, res.Header.Get("Retry-After"))
	}
}

func TestClientLimitCapsAllUsersOfAClient(t *testing.T) {
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.ClientLimit = func(id string) (kernel.Limit, bool) {
			return kernel.Limit{Name: "test.client", Max: 2, Window: time.Minute}, id == "public-client-1"
		}
	})
	for i, token := range []string{"read", "bob"} {
		res := e.get(t, "/instance", map[string]string{"Authorization": "Bearer " + token})
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("RateLimit-Policy"), `"client";q=2`) {
			t.Fatalf("request %d = %d, RateLimit-Policy %q, want 200 under the client policy", i, res.StatusCode, res.Header.Get("RateLimit-Policy"))
		}
	}
	// A different user of the same client shares the cap.
	res := e.get(t, "/instance", map[string]string{"Authorization": "Bearer write"})
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") == "" {
		t.Errorf("request over the client cap = %d, Retry-After %q, want 429 with Retry-After", res.StatusCode, res.Header.Get("Retry-After"))
	}
}
