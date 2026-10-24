// SPDX-License-Identifier: AGPL-3.0-only

package oauth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/oauth"
)

// fakeIdentity records what the endpoints pass on. Refresh tokens "good" work; others fail
// with invalid_grant.
type fakeIdentity struct {
	exchanges []appidentity.CodeExchange
	refreshes []appidentity.RefreshRequest
	web       []string
	revokes   []appidentity.RevokeRequest

	registrations []appidentity.DynamicRegistrationRequest
	registerErr   error

	dynamic, cimd bool
}

func issued(rt string) appidentity.IssuedTokens {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	return appidentity.IssuedTokens{
		AccessToken: "my_at_new", RefreshToken: rt,
		Token: identity.AccessToken{Scope: "read write", IssuedAt: now, ExpiresAt: now.Add(time.Hour), RefreshExpiresAt: now.Add(24 * time.Hour)},
	}
}

var errInvalidGrant = &appidentity.OAuthError{Code: appidentity.OAuthInvalidGrant}

func (f *fakeIdentity) WebClientID() string { return "my-web" }

func (f *fakeIdentity) ExchangeAuthorizationCode(_ context.Context, in appidentity.CodeExchange) (appidentity.IssuedTokens, error) {
	f.exchanges = append(f.exchanges, in)
	if in.ClientSecret == "wrong" {
		return appidentity.IssuedTokens{}, &appidentity.OAuthError{Code: appidentity.OAuthInvalidClient}
	}
	return issued("my_rt_new"), nil
}

func (f *fakeIdentity) RefreshAccessToken(_ context.Context, in appidentity.RefreshRequest) (appidentity.IssuedTokens, error) {
	f.refreshes = append(f.refreshes, in)
	if in.RefreshToken != "good" {
		return appidentity.IssuedTokens{}, errInvalidGrant
	}
	return issued("my_rt_new"), nil
}

func (f *fakeIdentity) RefreshWebClient(_ context.Context, rt string) (appidentity.IssuedTokens, error) {
	f.web = append(f.web, rt)
	if rt != "good" {
		return appidentity.IssuedTokens{}, errInvalidGrant
	}
	return issued("my_rt_rotated"), nil
}

func (f *fakeIdentity) DynamicClientRegistrationEnabled(context.Context) (bool, error) {
	return f.dynamic, nil
}

func (f *fakeIdentity) MetadataDocumentClientsEnabled(context.Context) (bool, error) {
	return f.cimd, nil
}

func (f *fakeIdentity) RevokeToken(_ context.Context, in appidentity.RevokeRequest) error {
	f.revokes = append(f.revokes, in)
	return nil
}

type mux struct{ *http.ServeMux }

func (m mux) Handle(pattern string, h http.Handler) { m.ServeMux.Handle(pattern, h) }

func newServer(t *testing.T, id *fakeIdentity) http.Handler {
	t.Helper()
	h, err := oauth.New(oauth.Options{Identity: id, WebOrigin: "https://web.example", Origin: "https://social.example"})
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	h.Register(m)
	return m
}

// post sends a form like the web client's fetch from the server's own origin.
func post(h http.Handler, path string, form url.Values, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "https://social.example"+path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	for k, v := range header {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", w.Header().Get("Cache-Control"))
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return m
}

func wantError(t *testing.T, w *httptest.ResponseRecorder, status int, code, what string) {
	t.Helper()
	if w.Code != status {
		t.Errorf("%s = %d %s, want %d", what, w.Code, w.Body.String(), status)
		return
	}
	if got := decode(t, w)["error"]; got != code {
		t.Errorf("%s: error = %v, want %s", what, got, code)
	}
}

func refreshCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == oauth.CookieRefreshToken {
			return c
		}
	}
	return nil
}

var webCode = url.Values{
	"grant_type": {"authorization_code"}, "client_id": {"my-web"}, "code": {"c"},
	"redirect_uri": {"https://web.example/login/callback"}, "code_verifier": {"v"},
}

func TestWebClientGetsItsRefreshTokenOnlyAsACookie(t *testing.T) {
	id := &fakeIdentity{}
	h := newServer(t, id)
	w := post(h, oauth.PathToken, webCode, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("token = %d %s", w.Code, w.Body.String())
	}
	body := decode(t, w)
	if body["access_token"] != "my_at_new" || body["token_type"] != "Bearer" || body["expires_in"] != float64(3600) ||
		body["scope"] != "read write" {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["refresh_token"]; ok {
		t.Error("the web client's refresh token is in the body")
	}
	c := refreshCookie(w)
	if c == nil || c.Value != "my_rt_new" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Domain != "" {
		t.Fatalf("cookie = %+v, want __Host-my_rt HttpOnly Secure SameSite=Strict Path=/", c)
	}

	// A cross-site form cannot plant a refresh token cookie.
	w = post(h, oauth.PathToken, webCode, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"})
	wantError(t, w, http.StatusForbidden, appidentity.OAuthInvalidRequest, "cross-site code exchange")
	if refreshCookie(w) != nil || len(id.exchanges) != 1 {
		t.Error("a cross-site exchange reached the service or set the cookie")
	}
}

func TestThirdPartyClientsGetRefreshTokensInTheBody(t *testing.T) {
	id := &fakeIdentity{}
	h := newServer(t, id)
	form := url.Values{"grant_type": {"authorization_code"}, "code": {"c"}, "redirect_uri": {"https://app.example/cb"}, "code_verifier": {"v"}}
	r := httptest.NewRequest(http.MethodPost, "https://social.example"+oauth.PathToken, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetBasicAuth(url.QueryEscape("app id"), url.QueryEscape("s3cret:+"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || decode(t, w)["refresh_token"] != "my_rt_new" || refreshCookie(w) != nil {
		t.Fatalf("token = %d %s, want the refresh token in the body and no cookie", w.Code, w.Body.String())
	}
	if got := id.exchanges[0].ClientCredentials; got.ClientID != "app id" || got.ClientSecret != "s3cret:+" {
		t.Errorf("credentials = %+v, want the decoded Basic credentials", got)
	}

	r = httptest.NewRequest(http.MethodPost, "https://social.example"+oauth.PathToken, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetBasicAuth("app", "wrong")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	wantError(t, w, http.StatusUnauthorized, appidentity.OAuthInvalidClient, "wrong secret")
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("401 after Basic authentication without WWW-Authenticate")
	}
}

func TestTokenEndpointRejectsMalformedRequests(t *testing.T) {
	h := newServer(t, &fakeIdentity{})
	wantError(t, post(h, oauth.PathToken, url.Values{"client_id": {"x"}}, nil), http.StatusBadRequest, appidentity.OAuthInvalidRequest, "no grant_type")
	wantError(t, post(h, oauth.PathToken, url.Values{"grant_type": {"password"}}, nil), http.StatusBadRequest,
		appidentity.OAuthUnsupportedGrantType, "password grant")
	wantError(t, post(h, oauth.PathToken, url.Values{"grant_type": {"refresh_token", "refresh_token"}}, nil), http.StatusBadRequest,
		appidentity.OAuthInvalidRequest, "repeated parameter")
	wantError(t, post(h, oauth.PathToken, webCode, map[string]string{"Content-Type": "application/json"}), http.StatusBadRequest,
		appidentity.OAuthInvalidRequest, "JSON body")
	wantError(t, post(h, oauth.PathToken, webCode, map[string]string{"Authorization": "Bearer my_at_x"}), http.StatusBadRequest,
		appidentity.OAuthInvalidRequest, "bearer client authentication")
}

// The refresh token cookie is accepted only by POST /oauth/token/refresh (ADR-0030).
func TestOnlyTheRefreshEndpointReadsTheCookie(t *testing.T) {
	id := &fakeIdentity{}
	h := newServer(t, id)
	cookie := map[string]string{"Cookie": oauth.CookieRefreshToken + "=good"}

	// The token endpoint ignores the cookie: the refresh_token grant sees no refresh token.
	w := post(h, oauth.PathToken, url.Values{"grant_type": {"refresh_token"}, "client_id": {"app"}}, cookie)
	wantError(t, w, http.StatusBadRequest, appidentity.OAuthInvalidGrant, "refresh_token grant with only the cookie")
	if len(id.refreshes) != 1 || id.refreshes[0].RefreshToken != "" || len(id.web) != 0 {
		t.Errorf("the token endpoint read the cookie: %+v, %q", id.refreshes, id.web)
	}
	// Revocation ignores the cookie too.
	post(h, oauth.PathRevoke, url.Values{"client_id": {"my-web"}}, cookie)
	if len(id.revokes) != 1 || id.revokes[0].Token != "" {
		t.Errorf("the revocation endpoint read the cookie: %+v", id.revokes)
	}

	// The refresh endpoint needs its header and a same-origin (or web origin) request.
	for name, header := range map[string]map[string]string{
		"no header":  {"Cookie": cookie["Cookie"]},
		"header 0":   {"Cookie": cookie["Cookie"], oauth.HeaderRefresh: "0"},
		"cross-site": {"Cookie": cookie["Cookie"], oauth.HeaderRefresh: "1", "Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"},
	} {
		wantError(t, post(h, oauth.PathRefresh, nil, header), http.StatusForbidden, appidentity.OAuthInvalidRequest, name)
	}
	if len(id.web) != 0 {
		t.Fatalf("a refused refresh reached the service: %q", id.web)
	}

	w = post(h, oauth.PathRefresh, nil, map[string]string{"Cookie": cookie["Cookie"], oauth.HeaderRefresh: "1"})
	if w.Code != http.StatusOK {
		t.Fatalf("refresh = %d %s", w.Code, w.Body.String())
	}
	body := decode(t, w)
	if body["access_token"] != "my_at_new" || body["refresh_token"] != nil {
		t.Errorf("refresh body = %v, want only an access token", body)
	}
	if c := refreshCookie(w); c == nil || c.Value != "my_rt_rotated" || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("rotated cookie = %+v", c)
	}
	if len(id.web) != 1 || id.web[0] != "good" {
		t.Errorf("RefreshWebClient got %q, want the cookie's value", id.web)
	}

	// The web origin may call it with credentials; an invalid refresh token clears the cookie.
	w = post(h, oauth.PathRefresh, nil, map[string]string{
		"Cookie": oauth.CookieRefreshToken + "=used", oauth.HeaderRefresh: "1", "Sec-Fetch-Site": "same-site", "Origin": "https://web.example",
	})
	wantError(t, w, http.StatusBadRequest, appidentity.OAuthInvalidGrant, "a used refresh token")
	if c := refreshCookie(w); c == nil || c.MaxAge >= 0 {
		t.Errorf("cookie after invalid_grant = %+v, want it deleted", c)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://web.example" || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("CORS headers = %v", w.Header())
	}
}

func TestPreflightAllowsOnlyTheWebOrigin(t *testing.T) {
	h := newServer(t, &fakeIdentity{})
	for origin, want := range map[string]string{"https://web.example": "https://web.example", "https://evil.example": ""} {
		r := httptest.NewRequest(http.MethodOptions, "https://social.example"+oauth.PathRefresh, nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", oauth.HeaderRefresh)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("preflight from %s: Allow-Origin = %q, want %q", origin, got, want)
		}
		if want != "" && !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), oauth.HeaderRefresh) {
			t.Errorf("preflight allows headers %q", w.Header().Get("Access-Control-Allow-Headers"))
		}
	}
}

func TestRevokeDeletesTheWebClientCookie(t *testing.T) {
	id := &fakeIdentity{}
	h := newServer(t, id)
	w := post(h, oauth.PathRevoke, url.Values{"client_id": {"my-web"}, "token": {"my_at_x"}, "token_type_hint": {"access_token"}}, nil)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revoke = %d %v", w.Code, w.Header())
	}
	if c := refreshCookie(w); c == nil || c.MaxAge >= 0 {
		t.Errorf("cookie after revoking = %+v, want it deleted", c)
	}
	if len(id.revokes) != 1 || id.revokes[0].Token != "my_at_x" || id.revokes[0].TokenTypeHint != "access_token" {
		t.Errorf("revokes = %+v", id.revokes)
	}
}
