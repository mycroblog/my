// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/oauth"
	"github.com/mycroblog/my/internal/platform"
)

// TestOAuthWebClientFlow drives the web client's authorization code flow with PKCE, the
// refresh cookie, reuse detection, and revocation on the real database.
func TestOAuthWebClientFlow(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		_, err := a.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
			Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("CreateAdmin: %v", err)
		}
	})
	c := &authClient{t: t, s: s, cookies: map[string]string{}}
	redirect := "https://example.test" + platform.WebClientCallbackPath
	verifier := strings.Repeat("v", 43)
	authorize := authpages.PathAuthorize + "?" + url.Values{
		"response_type": {"code"}, "client_id": {identity.DefaultWebClientID}, "redirect_uri": {redirect},
		"state": {"s1"}, "code_challenge": {domain.S256Challenge(verifier)}, "code_challenge_method": {"S256"},
	}.Encode()

	// Without a session the browser signs in first and comes back.
	resp, _ := c.do(http.MethodGet, authorize, nil, nil)
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), authpages.PathSignIn+"?return_to=") {
		t.Fatalf("authorize without a session = %d → %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, _ = c.do(http.MethodPost, authpages.PathSignIn, c.form("email", "alice@example.com", "password", "correct horse battery staple",
		"return_to", authorize), nil)
	if resp.Header.Get("Location") != authorize {
		t.Fatalf("sign-in returned to %q, want %q", resp.Header.Get("Location"), authorize)
	}
	// The first-party client skips the consent.
	resp, _ = c.do(http.MethodGet, authorize, nil, nil)
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || resp.StatusCode != http.StatusSeeOther || back.Scheme+"://"+back.Host+back.Path != redirect {
		t.Fatalf("authorize = %d → %q, want the callback", resp.StatusCode, resp.Header.Get("Location"))
	}
	if back.Query().Get("state") != "s1" || back.Query().Get("iss") != "https://example.test" {
		t.Errorf("callback query = %v, want the state and iss", back.Query())
	}
	code := back.Query().Get("code")

	token := func(form url.Values, header map[string]string) (*http.Response, map[string]any) {
		t.Helper()
		resp, body := c.do(http.MethodPost, oauth.PathToken, form, header)
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("token body %q: %v", body, err)
		}
		return resp, m
	}
	exchange := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {identity.DefaultWebClientID}, "code": {code},
		"redirect_uri": {redirect}, "code_verifier": {verifier},
	}
	resp, body := token(exchange, nil)
	if resp.StatusCode != http.StatusOK || body["refresh_token"] != nil || body["token_type"] != "Bearer" {
		t.Fatalf("exchange = %d %v", resp.StatusCode, body)
	}
	firstAccess := body["access_token"].(string)
	firstRefresh := c.cookies[oauth.CookieRefreshToken]
	if !strings.HasPrefix(firstRefresh, identity.RefreshTokenPrefix) {
		t.Fatalf("refresh cookie = %q", firstRefresh)
	}
	if _, err := s.app.Identity.AuthenticateAccessToken(t.Context(), firstAccess); err != nil {
		t.Fatalf("the issued access token does not authenticate: %v", err)
	}
	// The code works once.
	if resp, body := token(exchange, nil); resp.StatusCode != http.StatusBadRequest || body["error"] != identity.OAuthInvalidGrant {
		t.Fatalf("second exchange = %d %v", resp.StatusCode, body)
	}
	if _, err := s.app.Identity.AuthenticateAccessToken(t.Context(), firstAccess); err == nil {
		t.Fatal("the token of a reused code still authenticates")
	}

	// A new grant.
	resp, _ = c.do(http.MethodGet, authorize, nil, nil)
	back, _ = url.Parse(resp.Header.Get("Location"))
	exchange.Set("code", back.Query().Get("code"))
	if resp, body := token(exchange, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("exchange = %d %v", resp.StatusCode, body)
	}
	cookie := c.cookies[oauth.CookieRefreshToken]

	// The token endpoint does not take the web client's refresh token, from the cookie or the form.
	for _, form := range []url.Values{
		{"grant_type": {"refresh_token"}, "client_id": {identity.DefaultWebClientID}},
		{"grant_type": {"refresh_token"}, "client_id": {identity.DefaultWebClientID}, "refresh_token": {cookie}},
	} {
		if resp, body := token(form, nil); resp.StatusCode != http.StatusBadRequest || body["error"] != identity.OAuthUnauthorizedClient {
			t.Errorf("refresh_token grant of the web client = %d %v", resp.StatusCode, body)
		}
	}

	refresh := func(header map[string]string) (*http.Response, map[string]any) {
		t.Helper()
		resp, body := c.do(http.MethodPost, oauth.PathRefresh, url.Values{}, header)
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("refresh body %q: %v", body, err)
		}
		return resp, m
	}
	if resp, _ := refresh(nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("refresh without %s = %d, want 403", oauth.HeaderRefresh, resp.StatusCode)
	}
	resp, body = refresh(map[string]string{oauth.HeaderRefresh: "1"})
	if resp.StatusCode != http.StatusOK || body["access_token"] == nil || body["refresh_token"] != nil {
		t.Fatalf("refresh = %d %v", resp.StatusCode, body)
	}
	rotated := c.cookies[oauth.CookieRefreshToken]
	if rotated == cookie || rotated == "" {
		t.Fatal("refreshing did not rotate the cookie")
	}
	newest := body["access_token"].(string)

	// The used cookie value coming back revokes the family.
	c.cookies[oauth.CookieRefreshToken] = cookie
	if resp, body := refresh(map[string]string{oauth.HeaderRefresh: "1"}); resp.StatusCode != http.StatusBadRequest || body["error"] != identity.OAuthInvalidGrant {
		t.Fatalf("reused refresh = %d %v", resp.StatusCode, body)
	}
	if _, ok := c.cookies[oauth.CookieRefreshToken]; ok {
		t.Error("invalid_grant did not delete the cookie")
	}
	if _, err := s.app.Identity.AuthenticateAccessToken(t.Context(), newest); err == nil {
		t.Error("the newest access token survived the reuse")
	}
	c.cookies[oauth.CookieRefreshToken] = rotated
	if resp, _ := refresh(map[string]string{oauth.HeaderRefresh: "1"}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("refresh with the newest cookie after the reuse = %d, want 400", resp.StatusCode)
	}

	// Revoking the access token ends the grant and deletes the cookie.
	resp, _ = c.do(http.MethodGet, authorize, nil, nil)
	back, _ = url.Parse(resp.Header.Get("Location"))
	exchange.Set("code", back.Query().Get("code"))
	_, body = token(exchange, nil)
	access := body["access_token"].(string)
	resp, _ = c.do(http.MethodPost, oauth.PathRevoke, url.Values{"client_id": {identity.DefaultWebClientID}, "token": {access}}, nil)
	if resp.StatusCode != http.StatusOK || c.cookies[oauth.CookieRefreshToken] != "" {
		t.Fatalf("revoke = %d, cookie %q", resp.StatusCode, c.cookies[oauth.CookieRefreshToken])
	}
	if _, err := s.app.Identity.AuthenticateAccessToken(t.Context(), access); err == nil {
		t.Error("the revoked access token still authenticates")
	}
	// The revocation is the web client's sign-out: it ends the browser's session of the
	// authorization screens too, so the next sign-in asks for the password again.
	resp, _ = c.do(http.MethodGet, authorize, nil, nil)
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), authpages.PathSignIn+"?return_to=") {
		t.Errorf("authorize after the sign-out = %d → %q, want the sign-in form", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// TestOAuthWebClientPlainHTTPOnLocalhost checks the local setup of deploy/docker/example.env (host localhost:8080,
// web_origin http://localhost:8080): the shell calls the API over plain HTTP, the authorization
// endpoint answers with that issuer, and the code it returns works at the token endpoint, while the
// ActivityPub ids stay under https://localhost:8080/.
func TestOAuthWebClientPlainHTTPOnLocalhost(t *testing.T) {
	t.Parallel()
	const origin = "http://localhost:8080"
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_HOST": "localhost:8080", "MY_WEB_ORIGIN": origin})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		_, err := a.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
			Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("CreateAdmin: %v", err)
		}
	})
	c := &authClient{t: t, s: s, cookies: map[string]string{}}
	if _, body := c.do(http.MethodGet, "/", nil, map[string]string{"Accept": "text/html"}); !strings.Contains(body, `"apiOrigin":"`+origin+`"`) {
		t.Errorf("the shell does not call the API at %s:\n%s", origin, body)
	}
	redirect := origin + platform.WebClientCallbackPath
	verifier := strings.Repeat("v", 43)
	authorize := authpages.PathAuthorize + "?" + url.Values{
		"response_type": {"code"}, "client_id": {identity.DefaultWebClientID}, "redirect_uri": {redirect},
		"state": {"s1"}, "code_challenge": {domain.S256Challenge(verifier)}, "code_challenge_method": {"S256"},
	}.Encode()
	c.do(http.MethodPost, authpages.PathSignIn, c.form("email", "alice@example.com", "password", "correct horse battery staple",
		"return_to", authorize), nil)
	resp, _ := c.do(http.MethodGet, authorize, nil, nil)
	back, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || back.Scheme+"://"+back.Host+back.Path != redirect || back.Query().Get("iss") != origin {
		t.Fatalf("authorize = %d → %q, want the callback with the issuer %s", resp.StatusCode, resp.Header.Get("Location"), origin)
	}
	resp, body := c.do(http.MethodPost, oauth.PathToken, url.Values{
		"grant_type": {"authorization_code"}, "client_id": {identity.DefaultWebClientID}, "code": {back.Query().Get("code")},
		"redirect_uri": {redirect}, "code_verifier": {verifier},
	}, nil)
	if resp.StatusCode != http.StatusOK || c.cookies[oauth.CookieRefreshToken] == "" {
		t.Fatalf("exchange = %d %s", resp.StatusCode, body)
	}
	_, body = c.do(http.MethodGet, "/.well-known/webfinger?resource=acct:alice@localhost:8080", nil, nil)
	if !strings.Contains(body, `"href":"https://localhost:8080/ap/actors/`) {
		t.Errorf("WebFinger does not point at an https Actor id: %s", body)
	}
}
