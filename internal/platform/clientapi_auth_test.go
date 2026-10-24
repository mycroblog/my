// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/interface/oauth"
	"github.com/mycroblog/my/internal/platform"
)

// issueTokens runs the web client's authorization code flow for a user and returns the tokens
// with scope ("" for every scope of the client).
func issueTokens(t *testing.T, app *platform.App, userID, scope string) identity.IssuedTokens {
	t.Helper()
	ctx := t.Context()
	verifier := strings.Repeat("v", 43)
	redirect := webOrigin + platform.WebClientCallbackPath
	a, err := app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
		ResponseType: "code", ClientID: identity.DefaultWebClientID, RedirectURI: redirect, Scope: scope,
		CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
	})
	if err != nil {
		t.Fatalf("ValidateAuthorization(%q): %v", scope, err)
	}
	code, err := app.Identity.IssueAuthorizationCode(ctx, a, userID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := app.Identity.ExchangeAuthorizationCode(ctx, identity.CodeExchange{
		ClientCredentials: identity.ClientCredentials{ClientID: identity.DefaultWebClientID},
		Code:              code, RedirectURI: redirect, CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

// TestClientAPIAuthentication drives the authenticated client API end to end on the real
// database: tokens from the authorization server, verify_credentials, scopes, revocation,
// the user's locale, an account that may no longer use the app, and cookies, which never
// authenticate.
func TestClientAPIAuthentication(t *testing.T) {
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
	issueFor := func(userID, scope string) identity.IssuedTokens {
		t.Helper()
		return issueTokens(t, app, userID, scope)
	}
	issue := func(scope string) identity.IssuedTokens { t.Helper(); return issueFor(alice.User.ID, scope) }
	get := func(header map[string]string) (*http.Response, clientapi.Problem) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/verify_credentials", nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		res := spec.Serve(t, h, req)
		var p clientapi.Problem
		if res.StatusCode != http.StatusOK {
			if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
				t.Fatal(err)
			}
		}
		return res, p
	}

	full := issue("")
	res, _ := get(map[string]string{"Authorization": "Bearer " + full.AccessToken})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("verify_credentials = %d", res.StatusCode)
	}
	var me clientapi.Account
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me.Id != alice.AccountID || me.Username != "alice" || me.Acct != "alice" || me.Url != "https://example.test/@alice" {
		t.Errorf("verify_credentials = %+v, want alice (%s)", me, alice.AccountID)
	}
	if got := res.Header.Get("RateLimit-Policy"); got != `"account";q=1500;w=300` || !strings.HasPrefix(res.Header.Get("RateLimit"), `"account";r=1499;`) {
		t.Errorf("RateLimit-Policy = %q, RateLimit = %q", got, res.Header.Get("RateLimit"))
	}

	// The refresh token cookie of the web client, and the session cookie, never authenticate.
	res, p := get(map[string]string{
		"Cookie": oauth.CookieRefreshToken + "=" + full.RefreshToken + "; __Host-my_session=" + full.AccessToken,
	})
	if res.StatusCode != http.StatusUnauthorized || p.Type != clientapi.ProblemTypeUnauthenticated {
		t.Errorf("cookies only = %d %s, want 401 unauthenticated", res.StatusCode, p.Type)
	}

	// The user's saved locale comes before Accept-Language.
	prefs, err := app.Identity.Preferences(ctx, alice.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Identity.SetPreferences(ctx, alice.User.ID, identity.PreferencesInput{
		Locale: "ja", TimeZone: prefs.TimeZone, DefaultVisibility: prefs.DefaultVisibility,
		DefaultPostLanguage: prefs.DefaultPostLanguage, RequireMediaDescription: prefs.RequireMediaDescription,
	}); err != nil {
		t.Fatal(err)
	}
	write := issue(domain.ScopeWrite)
	res, p = get(map[string]string{"Authorization": "Bearer " + write.AccessToken, "Accept-Language": "en"})
	if res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeInsufficientScope ||
		res.Header.Get("Content-Language") != "ja" || res.Header.Get("WWW-Authenticate") != `Bearer error="insufficient_scope", scope="read:accounts"` {
		t.Errorf("write token = %d %s (Content-Language %q, WWW-Authenticate %q), want 403 insufficient_scope in ja",
			res.StatusCode, p.Type, res.Header.Get("Content-Language"), res.Header.Get("WWW-Authenticate"))
	}
	fine := issue(domain.ScopeReadAccounts)
	if res, _ := get(map[string]string{"Authorization": "Bearer " + fine.AccessToken}); res.StatusCode != http.StatusOK {
		t.Errorf("read:accounts token = %d, want 200", res.StatusCode)
	}

	// A revoked token is an invalid token.
	if err := app.Identity.RevokeToken(ctx, identity.RevokeRequest{
		ClientCredentials: identity.ClientCredentials{ClientID: identity.DefaultWebClientID}, Token: fine.AccessToken,
	}); err != nil {
		t.Fatal(err)
	}
	res, p = get(map[string]string{"Authorization": "Bearer " + fine.AccessToken})
	if res.StatusCode != http.StatusUnauthorized || p.Type != clientapi.ProblemTypeInvalidToken || !strings.HasPrefix(res.Header.Get("WWW-Authenticate"), `Bearer error="invalid_token", resource_metadata="`) {
		t.Errorf("revoked token = %d %s, want 401 invalid_token", res.StatusCode, p.Type)
	}

	// A user waiting for a moderator cannot use the API, even with a token, until approved.
	if _, err := app.Instance.SetSetting(ctx, appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationApproval),
	}); err != nil {
		t.Fatal(err)
	}
	bob, err := app.Identity.SignUp(ctx, identity.SignUpInput{
		Username: "bob", Email: "bob@example.com", Password: "correct horse battery", Reason: "Friend of alice",
	})
	if err != nil || bob.State != domain.RegistrationPending {
		t.Fatalf("SignUp = %+v, %v; want a pending registration", bob, err)
	}
	bobToken := issueFor(bob.User.ID, "")
	res, p = get(map[string]string{"Authorization": "Bearer " + bobToken.AccessToken})
	if res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeAccountUnavailable {
		t.Errorf("pending user = %d %s, want 403 account-unavailable", res.StatusCode, p.Type)
	}
	if _, err := app.Identity.DecideRegistration(ctx, identity.DecideInput{UserID: bob.User.ID, Decision: domain.DecisionApproved}); err != nil {
		t.Fatal(err)
	}
	if res, _ := get(map[string]string{"Authorization": "Bearer " + bobToken.AccessToken}); res.StatusCode != http.StatusOK {
		t.Errorf("approved user = %d, want 200", res.StatusCode)
	}

	// A valid token stops working as soon as its account cannot be used.
	if _, err := app.Accounts.RecordLifecycleEvent(ctx, account.LifecycleInput{
		AccountID: alice.AccountID, Kind: domainaccount.LifecycleDeactivated, InitiatedBy: domainaccount.InitiatedBySelf,
	}); err != nil {
		t.Fatal(err)
	}
	res, p = get(map[string]string{"Authorization": "Bearer " + full.AccessToken})
	if res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeAccountUnavailable {
		t.Errorf("deactivated account = %d %s, want 403 account-unavailable", res.StatusCode, p.Type)
	}
}
