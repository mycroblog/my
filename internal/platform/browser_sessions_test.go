// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// browserEnv signs a user in as browsers do: a session, with token families that start from it.
type browserEnv struct {
	t   *testing.T
	app *platform.App
	h   http.Handler
}

const browserPassword = "correct horse battery staple"

func newBrowserEnv(t *testing.T) *browserEnv {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &browserEnv{t: t, app: app, h: app.Handler()}
}

// signIn starts a session from ip and userAgent; current is the session token the browser holds.
func (e *browserEnv) signIn(email, ip, userAgent, current string) identity.SignedIn {
	e.t.Helper()
	in, err := e.app.Identity.SignIn(e.t.Context(), identity.SignInInput{
		Email: email, Password: browserPassword, IP: ip, UserAgent: userAgent, CurrentToken: current,
	})
	if err != nil {
		e.t.Fatalf("SignIn: %v", err)
	}
	return in
}

// family runs the authorization code flow of a client in the session.
func (e *browserEnv) family(session identity.SignedIn, clientID, redirect, scope string) identity.IssuedTokens {
	e.t.Helper()
	ctx := e.t.Context()
	verifier := strings.Repeat("v", 43)
	a, err := e.app.Identity.ValidateAuthorization(ctx, identity.AuthorizationRequest{
		ResponseType: "code", ClientID: clientID, RedirectURI: redirect, Scope: scope,
		CodeChallenge: domain.S256Challenge(verifier), CodeChallengeMethod: domain.CodeChallengeMethodS256,
	})
	if err != nil {
		e.t.Fatalf("ValidateAuthorization: %v", err)
	}
	a.SessionID = session.Session.ID
	code, err := e.app.Identity.IssueAuthorizationCode(ctx, a, session.Session.UserID)
	if err != nil {
		e.t.Fatal(err)
	}
	tokens, err := e.app.Identity.ExchangeAuthorizationCode(ctx, identity.CodeExchange{
		ClientCredentials: identity.ClientCredentials{ClientID: clientID},
		Code:              code, RedirectURI: redirect, CodeVerifier: verifier,
	})
	if err != nil {
		e.t.Fatalf("ExchangeAuthorizationCode: %v", err)
	}
	return tokens
}

func (e *browserEnv) web(session identity.SignedIn) identity.IssuedTokens {
	e.t.Helper()
	return e.family(session, identity.DefaultWebClientID, webOrigin+platform.WebClientCallbackPath, "")
}

func (e *browserEnv) call(method, path, token string) *http.Response {
	e.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return apitest.Load(e.t).Serve(e.t, e.h, req)
}

func (e *browserEnv) list(token string) []clientapi.BrowserSession {
	e.t.Helper()
	res := e.call(http.MethodGet, "/api/v1/sessions", token)
	if res.StatusCode != http.StatusOK {
		e.t.Fatalf("GET /sessions = %d", res.StatusCode)
	}
	var body clientapi.BrowserSessionList
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		e.t.Fatal(err)
	}
	return body.Items
}

func (e *browserEnv) tokenWorks(token string) bool {
	e.t.Helper()
	res := e.call(http.MethodGet, "/api/v1/accounts/verify_credentials", token)
	switch res.StatusCode {
	case http.StatusOK:
		return true
	case http.StatusUnauthorized:
		return false
	}
	e.t.Fatalf("verify_credentials = %d", res.StatusCode)
	return false
}

func (e *browserEnv) sessionWorks(token string) bool {
	e.t.Helper()
	_, err := e.app.Identity.Authenticate(e.t.Context(), token)
	return err == nil
}

func (e *browserEnv) createUser(username string) identity.Registration {
	e.t.Helper()
	r, err := e.app.Identity.CreateAdmin(e.t.Context(), identity.CreateAdminInput{
		Username: username, Email: username + "@example.com", Password: browserPassword,
	})
	if err != nil {
		e.t.Fatalf("CreateAdmin: %v", err)
	}
	return r
}

// TestBrowserSessions lists the browsers of a user on the real database and signs them out: one
// row per session however many families it has, this browser first and marked, and the 404s and
// the 409 of what a row cannot be.
func TestBrowserSessions(t *testing.T) {
	t.Parallel()
	e := newBrowserEnv(t)
	e.createUser("alice")
	e.createUser("bob")
	const email = "alice@example.com"

	here := e.signIn(email, "203.0.113.1", "here-agent", "")
	hereTokens := e.web(here)
	// A second family of the session, as after the refresh cookie was lost.
	hereAgain := e.web(here)

	phone := e.signIn(email, "198.51.100.2", "phone-agent", "")
	phoneTokens := e.web(phone)

	// A session that got no family yet can still get tokens, so it is a browser too.
	noFamily := e.signIn(email, "", "", "")

	items := e.list(hereTokens.AccessToken)
	if len(items) != 3 {
		t.Fatalf("browsers = %+v, want one row per session", items)
	}
	if !items[0].Current || items[0].Id != here.Session.ID {
		t.Errorf("first browser = %+v, want this one marked", items[0])
	}
	if items[0].Ip == nil || *items[0].Ip != "203.0.113.1" || items[0].UserAgent == nil || *items[0].UserAgent != "here-agent" {
		t.Errorf("this browser = %+v, want the origin of the sign-in", items[0])
	}
	if items[0].LastUsedAt == nil {
		t.Errorf("this browser = %+v, want its last use", items[0])
	}
	for _, b := range items[1:] {
		if b.Current {
			t.Errorf("browser %+v is marked as this one", b)
		}
		if b.Id == noFamily.Session.ID && (b.Ip != nil || b.UserAgent != nil) {
			t.Errorf("browser without an origin = %+v, want neither field", b)
		}
	}
	// The other family of this browser marks the same row.
	if again := e.list(hereAgain.AccessToken); !again[0].Current || again[0].Id != here.Session.ID {
		t.Errorf("first browser for the other family = %+v, want the same row marked", again[0])
	}

	// This browser is signed out by signing out, not from its row.
	res := e.call(http.MethodDelete, "/api/v1/sessions/"+here.Session.ID, hereTokens.AccessToken)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("DELETE this browser = %d, want 409", res.StatusCode)
	}
	var problem clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&problem); err != nil || problem.Type != clientapi.ProblemTypeCurrentSession {
		t.Errorf("DELETE this browser problem = %+v, %v", problem, err)
	}

	// Someone else's session and one that does not exist are the same 404.
	bob := e.signIn("bob@example.com", "", "", "")
	bobTokens := e.web(bob)
	for name, id := range map[string]string{"another user's": bob.Session.ID, "unknown": "019a0000-0000-7000-8000-0000000000ff"} {
		if res := e.call(http.MethodDelete, "/api/v1/sessions/"+id, hereTokens.AccessToken); res.StatusCode != http.StatusNotFound {
			t.Errorf("DELETE %s session = %d, want 404", name, res.StatusCode)
		}
	}
	if !e.sessionWorks(bob.Token) || !e.tokenWorks(bobTokens.AccessToken) {
		t.Error("another user's browser was signed out")
	}

	// Signing a browser out ends its session and its tokens at once, and nothing else.
	if res := e.call(http.MethodDelete, "/api/v1/sessions/"+phone.Session.ID, hereTokens.AccessToken); res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE a browser = %d, want 204", res.StatusCode)
	}
	if e.sessionWorks(phone.Token) || e.tokenWorks(phoneTokens.AccessToken) {
		t.Error("the browser that was signed out still works")
	}
	if !e.sessionWorks(here.Token) || !e.tokenWorks(hereTokens.AccessToken) || !e.tokenWorks(hereAgain.AccessToken) {
		t.Error("this browser stopped working")
	}
	// It left the list, so a second try is a 404.
	if res := e.call(http.MethodDelete, "/api/v1/sessions/"+phone.Session.ID, hereTokens.AccessToken); res.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE a browser again = %d, want 404", res.StatusCode)
	}
	if _, err := e.app.Identity.RefreshWebClient(t.Context(), phoneTokens.RefreshToken); err == nil {
		t.Error("the refresh token of the browser that was signed out still works")
	}
	if got := e.list(hereTokens.AccessToken); len(got) != 2 {
		t.Errorf("browsers after signing one out = %+v", got)
	}
}

// TestBrowserSessionsRevokeOthers keeps this browser, even when its session has ended and only its
// token family works, and keeps the apps the user authorized.
func TestBrowserSessionsRevokeOthers(t *testing.T) {
	t.Parallel()
	e := newBrowserEnv(t)
	alice := e.createUser("alice")
	ctx := t.Context()
	const email = "alice@example.com"

	here := e.signIn(email, "", "here-agent", "")
	hereTokens := e.web(here)
	other := e.signIn(email, "", "other-agent", "")
	otherTokens := e.web(other)
	idle := e.signIn(email, "", "idle-agent", "")

	// A third-party app authorized in the other browser.
	const redirect = "https://app.example/callback"
	registered, err := e.app.Identity.RegisterOAuthApp(ctx, alice.User.ID, identity.OAuthAppInput{
		Name: "App", RedirectURIs: []string{redirect}, Scope: "read",
	})
	if err != nil {
		t.Fatalf("RegisterOAuthApp: %v", err)
	}
	appTokens := e.family(other, registered.Client.ClientID, redirect, "read")

	// A third-party token cannot call any of it.
	if res := e.call(http.MethodGet, "/api/v1/sessions", appTokens.AccessToken); res.StatusCode != http.StatusForbidden {
		t.Errorf("GET /sessions with a third-party token = %d, want 403", res.StatusCode)
	}

	// This browser's session ends, as a password change does to the browser that asks, while its
	// family stays: it is still listed and still this browser.
	if err := e.app.Identity.SignOut(ctx, here.Token); err != nil {
		t.Fatalf("SignOut: %v", err)
	}
	items := e.list(hereTokens.AccessToken)
	if len(items) != 3 || !items[0].Current || items[0].Id != here.Session.ID {
		t.Fatalf("browsers = %+v, want this one first though its session ended", items)
	}

	if res := e.call(http.MethodPost, "/api/v1/sessions/revoke_others", hereTokens.AccessToken); res.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /sessions/revoke_others = %d, want 204", res.StatusCode)
	}
	if !e.tokenWorks(hereTokens.AccessToken) {
		t.Error("this browser's token stopped working")
	}
	if e.sessionWorks(other.Token) || e.tokenWorks(otherTokens.AccessToken) || e.sessionWorks(idle.Token) {
		t.Error("another browser still works")
	}
	if !e.tokenWorks(appTokens.AccessToken) {
		t.Error("the third-party app's token stopped working")
	}
	if got := e.list(hereTokens.AccessToken); len(got) != 1 || !got[0].Current {
		t.Errorf("browsers after signing out the others = %+v, want this one", got)
	}
	// With nothing else to sign out, it still succeeds.
	if res := e.call(http.MethodPost, "/api/v1/sessions/revoke_others", hereTokens.AccessToken); res.StatusCode != http.StatusNoContent {
		t.Errorf("POST /sessions/revoke_others again = %d, want 204", res.StatusCode)
	}
}

// TestSignInAgainEndsTheFamiliesOfTheReplacedSession: the same browser does not stay listed twice,
// and a third-party app authorized in it keeps its tokens (ADR-0136 §5).
func TestSignInAgainEndsTheFamiliesOfTheReplacedSession(t *testing.T) {
	t.Parallel()
	e := newBrowserEnv(t)
	alice := e.createUser("alice")
	ctx := t.Context()
	const email = "alice@example.com"

	first := e.signIn(email, "", "agent", "")
	firstTokens := e.web(first)
	const redirect = "https://app.example/callback"
	registered, err := e.app.Identity.RegisterOAuthApp(ctx, alice.User.ID, identity.OAuthAppInput{
		Name: "App", RedirectURIs: []string{redirect}, Scope: "read",
	})
	if err != nil {
		t.Fatalf("RegisterOAuthApp: %v", err)
	}
	appTokens := e.family(first, registered.Client.ClientID, redirect, "read")

	second := e.signIn(email, "", "agent", first.Token)
	secondTokens := e.web(second)

	if e.tokenWorks(firstTokens.AccessToken) {
		t.Error("the token of the replaced session still works")
	}
	if _, err := e.app.Identity.RefreshWebClient(ctx, firstTokens.RefreshToken); err == nil {
		t.Error("the refresh token of the replaced session still works")
	}
	if !e.tokenWorks(appTokens.AccessToken) {
		t.Error("the third-party app's token stopped working")
	}
	items := e.list(secondTokens.AccessToken)
	if len(items) != 1 || items[0].Id != second.Session.ID || !items[0].Current {
		t.Errorf("browsers = %+v, want the new session once", items)
	}
}
