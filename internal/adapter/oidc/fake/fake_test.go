// SPDX-License-Identifier: AGPL-3.0-only

package fake_test

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/httpclient"
	"github.com/mycroblog/my/internal/adapter/oidc"
	"github.com/mycroblog/my/internal/adapter/oidc/fake"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

const (
	clientID     = "example-client"
	clientSecret = "example-secret"
	redirectURI  = identitytest.ContractWebRedirect
	nonce        = "example-nonce"
	state        = "example-state"
)

// env is a fake provider served over TLS, the client the server uses against it, and the clock that
// both read.
type env struct {
	t        *testing.T
	srv      *httptest.Server
	clock    *testkit.Clock
	client   *oidc.Client
	provider appidentity.OIDCProvider
	browser  *http.Client
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, clock: testkit.NewClock(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))}
	var handler http.Handler
	e.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	t.Cleanup(e.srv.Close)
	p, err := fake.New(fake.Options{Issuer: e.srv.URL, ClientID: clientID, ClientSecret: clientSecret, Now: e.clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	handler = p.Handler()
	pool := x509.NewCertPool()
	pool.AddCert(e.srv.Certificate())
	e.client = oidc.New(oidc.Options{
		HTTP: httpclient.New(httpclient.Options{
			RootCAs:       pool,
			LoginProvider: httpclient.Destinations{Prefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}},
		}),
		Clock: e.clock,
	})
	e.browser = e.srv.Client()
	e.browser.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	e.provider, err = e.client.Discover(t.Context(), e.srv.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return e
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// query is an authentication request of the web client, with its parameters replaced by kv.
func query(kv ...string) url.Values {
	q := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirectURI}, "scope": {"openid email profile"},
		"state": {state}, "nonce": {nonce}, "code_challenge": {challenge(identitytest.ContractCodeVerifier)}, "code_challenge_method": {"S256"},
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == "" {
			q.Del(kv[i])
			continue
		}
		q.Set(kv[i], kv[i+1])
	}
	return q
}

func (e *env) do(method, target string, form url.Values, basic [2]string) (*http.Response, string) {
	e.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(e.t.Context(), method, target, body)
	if err != nil {
		e.t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if basic[0] != "" {
		req.SetBasicAuth(basic[0], basic[1])
	}
	resp, err := e.browser.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// person is who signs in on the authorization page.
func person(kv ...string) url.Values {
	f := url.Values{
		"sub": {"user-1"}, "email": {"alice@example.test"}, "email_verified": {"true"}, "name": {"Alice Example"},
		"preferred_username": {"alice"}, "action": {"allow"},
	}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == "" {
			f.Del(kv[i])
			continue
		}
		f.Set(kv[i], kv[i+1])
	}
	return f
}

// submit posts the form of the authorization page for q and returns the redirect it answers with.
func (e *env) submit(q, form url.Values) url.Values {
	e.t.Helper()
	resp, body := e.do(http.MethodPost, e.provider.AuthorizationEndpoint+"?"+q.Encode(), form, [2]string{})
	if resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("POST /authorize = %d, want 303\n%s", resp.StatusCode, body)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || loc.Scheme+"://"+loc.Host+loc.Path != redirectURI {
		e.t.Fatalf("Location = %q, want the redirect URI", resp.Header.Get("Location"))
	}
	return loc.Query()
}

// code signs the person in and returns the authorization code.
func (e *env) code(form url.Values) string {
	e.t.Helper()
	got := e.submit(query(), form)
	if got.Get("state") != state || got.Get("code") == "" || got.Get("error") != "" {
		e.t.Fatalf("redirect = %v, want a code and the state", got)
	}
	return got.Get("code")
}

type tokenResponse struct {
	IDToken string `json:"id_token"`
	Error   string `json:"error"`
}

func (e *env) redeem(kv ...string) (*http.Response, tokenResponse) {
	e.t.Helper()
	return e.redeemAs([2]string{clientID, clientSecret}, kv...)
}

func (e *env) redeemAs(basic [2]string, kv ...string) (*http.Response, tokenResponse) {
	e.t.Helper()
	f := url.Values{"grant_type": {"authorization_code"}, "redirect_uri": {redirectURI}, "code_verifier": {identitytest.ContractCodeVerifier}}
	for i := 0; i+1 < len(kv); i += 2 {
		f.Set(kv[i], kv[i+1])
	}
	resp, body := e.do(http.MethodPost, e.provider.TokenEndpoint, f, basic)
	var out tokenResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		e.t.Fatalf("POST /token = %d, body %q: %v", resp.StatusCode, body, err)
	}
	return resp, out
}

func (e *env) verify(raw string, check appidentity.IDTokenCheck) (appidentity.IDToken, error) {
	e.t.Helper()
	return e.client.VerifyIDToken(e.t.Context(), e.provider, raw, check)
}

func TestDiscovery(t *testing.T) {
	e := newEnv(t)
	p := e.provider
	if p.Issuer != e.srv.URL {
		t.Errorf("issuer = %q, want %q", p.Issuer, e.srv.URL)
	}
	for name, endpoint := range map[string]string{"authorization": p.AuthorizationEndpoint, "token": p.TokenEndpoint, "jwks": p.JWKSURI} {
		if !strings.HasPrefix(endpoint, e.srv.URL+"/") {
			t.Errorf("%s endpoint = %q, want one of the issuer", name, endpoint)
		}
	}
	if !slices.Contains(p.SigningAlgs, "ES256") || !slices.Contains(p.CodeChallengeMethods, "S256") {
		t.Errorf("signing algorithms %v and code challenge methods %v, want ES256 and S256", p.SigningAlgs, p.CodeChallengeMethods)
	}
}

// TestCodeFlow runs the Authorization Code flow with PKCE as the server's login provider does, and
// verifies the ID token with the server's own client.
func TestCodeFlow(t *testing.T) {
	e := newEnv(t)
	signedAt := e.clock.Now()

	resp, body := e.do(http.MethodGet, e.provider.AuthorizationEndpoint+"?"+query().Encode(), nil, [2]string{})
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("GET /authorize = %d %q\n%s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	code := e.code(person())
	e.clock.Advance(time.Second)
	resp, tok := e.redeem("code", code)
	if resp.StatusCode != http.StatusOK || tok.IDToken == "" {
		t.Fatalf("POST /token = %d %+v", resp.StatusCode, tok)
	}
	check := appidentity.IDTokenCheck{ClientID: clientID, Nonce: nonce, AuthenticatedAfter: signedAt}
	got, err := e.verify(tok.IDToken, check)
	if err != nil {
		t.Fatalf("VerifyIDToken: %v", err)
	}
	if got.Issuer != e.srv.URL || got.Subject != "user-1" || !slices.Equal(got.Audience, []string{clientID}) || got.Nonce != nonce {
		t.Errorf("token = %+v", got)
	}
	if got.Email != "alice@example.test" || !got.EmailVerified {
		t.Errorf("email = %q verified %v, want alice@example.test verified", got.Email, got.EmailVerified)
	}
	if !got.AuthTime.Equal(signedAt) {
		t.Errorf("auth_time = %v, want the time of the form's submission %v", got.AuthTime, signedAt)
	}
	if want := got.IssuedAt.Add(fake.TokenTTL); !got.Expiry.Equal(want) {
		t.Errorf("exp = %v, want iat + TokenTTL = %v", got.Expiry, want)
	}
	var username, name string
	if err := json.Unmarshal(got.Claims["preferred_username"], &username); err != nil || username != "alice" {
		t.Errorf("preferred_username = %q, %v", username, err)
	}
	if err := json.Unmarshal(got.Claims["name"], &name); err != nil || name != "Alice Example" {
		t.Errorf("name = %q, %v", name, err)
	}

	// The token is for this client and this request alone.
	for name, bad := range map[string]appidentity.IDTokenCheck{
		"another client": {ClientID: "another", Nonce: nonce},
		"another nonce":  {ClientID: clientID, Nonce: "another"},
	} {
		if _, err := e.verify(tok.IDToken, bad); !errors.Is(err, appidentity.ErrInvalidIDToken) {
			t.Errorf("VerifyIDToken(%s) = %v, want ErrInvalidIDToken", name, err)
		}
	}
}

// TestEachSubmissionIsANewAuthentication checks that the auth_time is the time of the last
// submission of the form, so that a re-authentication (max_age=0) is answered like any other
// request.
func TestEachSubmissionIsANewAuthentication(t *testing.T) {
	e := newEnv(t)
	first := e.clock.Now()
	e.code(person())
	e.clock.Advance(30 * time.Second)
	second := e.clock.Now()
	_, tok := e.redeem("code", e.code(person()))

	if _, err := e.verify(tok.IDToken, appidentity.IDTokenCheck{ClientID: clientID, Nonce: nonce, AuthenticatedAfter: second}); err != nil {
		t.Errorf("VerifyIDToken(AuthenticatedAfter = the second submission) = %v", err)
	}
	if first.Equal(second) {
		t.Fatal("the clock did not move")
	}
	if _, err := e.verify(tok.IDToken, appidentity.IDTokenCheck{ClientID: clientID, Nonce: nonce, AuthenticatedAfter: second.Add(10 * time.Minute)}); !errors.Is(err, appidentity.ErrInvalidIDToken) {
		t.Errorf("VerifyIDToken(AuthenticatedAfter = later than the authentication) = %v, want ErrInvalidIDToken", err)
	}
}

func TestEmailNotVerified(t *testing.T) {
	e := newEnv(t)
	_, tok := e.redeem("code", e.code(person("email_verified", "")))
	got, err := e.verify(tok.IDToken, appidentity.IDTokenCheck{ClientID: clientID, Nonce: nonce})
	if err != nil {
		t.Fatalf("VerifyIDToken: %v", err)
	}
	if got.Email != "alice@example.test" || got.EmailVerified {
		t.Errorf("email = %q verified %v, want alice@example.test not verified", got.Email, got.EmailVerified)
	}
}

func TestAuthorizeRefusesTheRequestWithAPage(t *testing.T) {
	e := newEnv(t)
	for name, q := range map[string]url.Values{
		"another client":                 query("client_id", "another"),
		"no client":                      query("client_id", ""),
		"a redirect URI that is not one": query("redirect_uri", "not a uri"),
		"no redirect URI":                query("redirect_uri", ""),
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			var form url.Values
			if method == http.MethodPost {
				form = person()
			}
			resp, body := e.do(method, e.provider.AuthorizationEndpoint+"?"+q.Encode(), form, [2]string{})
			if resp.StatusCode != http.StatusBadRequest || resp.Header.Get("Location") != "" {
				t.Errorf("%s /authorize with %s = %d Location %q, want a 400 page\n%s", method, name, resp.StatusCode, resp.Header.Get("Location"), body)
			}
		}
	}
}

func TestAuthorizeRefusesTheRequestByRedirect(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name  string
		q     url.Values
		error string
	}{
		{"no PKCE", query("code_challenge", "", "code_challenge_method", ""), "invalid_request"},
		{"a plain challenge", query("code_challenge_method", "plain"), "invalid_request"},
		{"no challenge", query("code_challenge", ""), "invalid_request"},
		{"no openid scope", query("scope", "email profile"), "invalid_scope"},
		{"a response type that is not code", query("response_type", "token"), "unsupported_response_type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				var form url.Values
				if method == http.MethodPost {
					form = person()
				}
				resp, body := e.do(method, e.provider.AuthorizationEndpoint+"?"+tt.q.Encode(), form, [2]string{})
				loc, err := url.Parse(resp.Header.Get("Location"))
				if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound || err != nil {
					t.Fatalf("%s /authorize = %d Location %q, want a redirect\n%s", method, resp.StatusCode, resp.Header.Get("Location"), body)
				}
				got := loc.Query()
				if got.Get("error") != tt.error || got.Get("code") != "" || got.Get("state") != state ||
					loc.Scheme+"://"+loc.Host+loc.Path != redirectURI {
					t.Errorf("%s /authorize redirects to %s, want %s with the state", method, loc, tt.error)
				}
			}
		})
	}
}

func TestDenyIsAccessDenied(t *testing.T) {
	e := newEnv(t)
	got := e.submit(query(), person("action", "deny"))
	if got.Get("error") != "access_denied" || got.Get("code") != "" || got.Get("state") != state {
		t.Errorf("redirect = %v, want access_denied with the state and no code", got)
	}
}

func TestAllowWithoutASubjectIsAPage(t *testing.T) {
	e := newEnv(t)
	resp, body := e.do(http.MethodPost, e.provider.AuthorizationEndpoint+"?"+query().Encode(), person("sub", ""), [2]string{})
	if resp.StatusCode != http.StatusBadRequest || resp.Header.Get("Location") != "" {
		t.Errorf("POST /authorize without sub = %d Location %q, want a 400 page\n%s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
}

func TestTokenRefusesWrongCredentials(t *testing.T) {
	e := newEnv(t)
	for name, basic := range map[string][2]string{
		"a wrong secret": {clientID, "wrong"},
		"another client": {"another", clientSecret},
		"no credentials": {},
	} {
		resp, tok := e.redeemAs(basic, "code", e.code(person()))
		if resp.StatusCode != http.StatusUnauthorized || tok.Error != "invalid_client" || tok.IDToken != "" {
			t.Errorf("POST /token with %s = %d %+v, want 401 invalid_client", name, resp.StatusCode, tok)
		}
	}
}

func TestTokenRefusesTheCode(t *testing.T) {
	e := newEnv(t)
	invalidGrant := func(name string, resp *http.Response, tok tokenResponse) {
		t.Helper()
		if resp.StatusCode != http.StatusBadRequest || tok.Error != "invalid_grant" || tok.IDToken != "" {
			t.Errorf("POST /token with %s = %d %+v, want 400 invalid_grant", name, resp.StatusCode, tok)
		}
	}

	code := e.code(person())
	if resp, tok := e.redeem("code", code); resp.StatusCode != http.StatusOK || tok.IDToken == "" {
		t.Fatalf("first redemption = %d %+v", resp.StatusCode, tok)
	}
	resp, tok := e.redeem("code", code)
	invalidGrant("a code redeemed before", resp, tok)

	resp, tok = e.redeem("code", "unknown-code")
	invalidGrant("an unknown code", resp, tok)

	resp, tok = e.redeem("code", e.code(person()), "redirect_uri", "https://other.example/callback")
	invalidGrant("another redirect URI", resp, tok)

	resp, tok = e.redeem("code", e.code(person()), "code_verifier", "wrong-verifier-wrong-verifier-wrong-verifier-123")
	invalidGrant("a wrong verifier", resp, tok)

	expired := e.code(person())
	e.clock.Advance(fake.CodeTTL + time.Second)
	resp, tok = e.redeem("code", expired)
	invalidGrant("an expired code", resp, tok)
}

// TestCodeIsRedeemableUntilItsTTL checks the other side of the expiry.
func TestCodeIsRedeemableUntilItsTTL(t *testing.T) {
	e := newEnv(t)
	code := e.code(person())
	e.clock.Advance(fake.CodeTTL - time.Second)
	if resp, tok := e.redeem("code", code); resp.StatusCode != http.StatusOK || tok.IDToken == "" {
		t.Errorf("POST /token within the TTL = %d %+v", resp.StatusCode, tok)
	}
}

func TestNewRefusesIncompleteOptions(t *testing.T) {
	ok := fake.Options{Issuer: "https://localhost:8931", ClientID: clientID, ClientSecret: clientSecret}
	if _, err := fake.New(ok); err != nil {
		t.Fatalf("New(%+v) = %v", ok, err)
	}
	for name, o := range map[string]fake.Options{
		"no issuer": {ClientID: clientID, ClientSecret: clientSecret},
		"no client": {Issuer: ok.Issuer, ClientSecret: clientSecret},
		"no secret": {Issuer: ok.Issuer, ClientID: clientID},
	} {
		if _, err := fake.New(o); err == nil {
			t.Errorf("New(%s) = nil, want an error", name)
		}
	}
}
