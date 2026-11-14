// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

// authClient keeps the cookies of the sign-in screens by hand: they are Secure, which a
// cookie jar would not send to the plain-HTTP test server.
type authClient struct {
	t       *testing.T
	s       *server
	cookies map[string]string
}

func (c *authClient) do(method, path string, form url.Values, header map[string]string) (*http.Response, string) {
	c.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.s.url+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	for name, value := range c.cookies {
		req.Header.Add("Cookie", name+"="+value)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	for _, ck := range resp.Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck.Value
		}
	}
	return resp, string(b)
}

var csrfToken = regexp.MustCompile(`"csrfToken":"([^"]+)"`)

// authDataBlock is the JSON data block of an auth screen, the data that the web client's auth
// entry renders.
var authDataBlock = regexp.MustCompile(`<script type="application/json" id="auth">([^<]*)</script>`)

// authData returns the data of the auth screen body, failing t when it has none.
func authData(t *testing.T, body string) map[string]any {
	t.Helper()
	m := authDataBlock.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the answer is no auth screen:\n%s", body)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		t.Fatalf("the auth screen's data is not JSON: %v\n%s", err, m[1])
	}
	return data
}

// authAt returns the value at the dotted path of an auth screen's data ("status.verifyEmail"),
// with numbers for the elements of an array ("external.0.url"), or nil when there is none.
func authAt(data map[string]any, path string) any {
	var v any = data
	for _, key := range strings.Split(path, ".") {
		switch c := v.(type) {
		case map[string]any:
			v = c[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(c) {
				return nil
			}
			v = c[i]
		default:
			return nil
		}
	}
	return v
}

// authText returns the string at path of an auth screen's data, "" when there is none.
func authText(data map[string]any, path string) string {
	s, _ := authAt(data, path).(string)
	return s
}

// form fetches the sign-in screen and returns its form fields with the anti-forgery token.
func (c *authClient) form(fields ...string) url.Values {
	c.t.Helper()
	return c.formAt(authpages.PathSignIn, fields...)
}

// formAt fetches the screen at path and returns its form fields with the anti-forgery token.
func (c *authClient) formAt(path string, fields ...string) url.Values {
	c.t.Helper()
	_, body := c.do(http.MethodGet, path, nil, nil)
	m := csrfToken.FindStringSubmatch(body)
	if m == nil {
		c.t.Fatalf("sign-in screen has no csrf_token: %s", body)
	}
	v := url.Values{"csrf_token": {m[1]}}
	for i := 0; i+1 < len(fields); i += 2 {
		v.Set(fields[i], fields[i+1])
	}
	return v
}

// TestSignInScreens drives the sign-in screens on the real database and key-value store.
func TestSignInScreens(t *testing.T) {
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
	signIn := func(password string) *http.Response {
		resp, _ := c.do(http.MethodPost, authpages.PathSignIn, c.form("email", "alice@example.com", "password", password), nil)
		return resp
	}

	resp, body := c.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "ja"})
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `<html lang="ja" dir="ltr">`) ||
		!strings.Contains(resp.Header.Get("Content-Security-Policy"), "form-action 'self'") {
		t.Fatalf("GET sign_in = %d %v\n%s", resp.StatusCode, resp.Header, body)
	}

	if resp := signIn("correct horse battery staple"); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-in = %d, want 303", resp.StatusCode)
	}
	first := c.cookies[authpages.CookieSession]
	if !strings.HasPrefix(first, identity.SessionTokenPrefix) {
		t.Fatalf("session cookie = %q", first)
	}
	if _, body := c.do(http.MethodGet, authpages.PathSignIn, nil, nil); authText(authData(t, body), "screen") != "signed_in" {
		t.Fatalf("signed-in screen = %s", body)
	}

	// Signing in again issues a new session and ends the old one.
	signIn("correct horse battery staple")
	second := c.cookies[authpages.CookieSession]
	if second == "" || second == first {
		t.Fatalf("second sign-in kept the session cookie %q", second)
	}
	if _, err := s.app.Identity.Authenticate(t.Context(), first); err == nil {
		t.Error("the replaced session still authenticates")
	}

	// A cross-origin post is refused before it reaches the form handler.
	resp, _ = c.do(http.MethodPost, authpages.PathSignOut, c.form(), map[string]string{"Sec-Fetch-Site": "cross-site"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin sign-out = %d, want 403", resp.StatusCode)
	}
	if _, err := s.app.Identity.Authenticate(t.Context(), second); err != nil {
		t.Fatalf("the refused sign-out ended the session: %v", err)
	}

	resp, _ = c.do(http.MethodPost, authpages.PathSignOut, c.form(), nil)
	if resp.StatusCode != http.StatusSeeOther || c.cookies[authpages.CookieSession] != "" {
		t.Fatalf("sign-out = %d, cookie %q", resp.StatusCode, c.cookies[authpages.CookieSession])
	}
	if _, err := s.app.Identity.Authenticate(t.Context(), second); err == nil {
		t.Error("the signed-out session still authenticates")
	}

	// Five failures from this address close it, even for the right password.
	for i := range identity.LoginIPLimit.Max {
		if resp := signIn("wrong password"); resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("failure #%d = %d, want 422", i+1, resp.StatusCode)
		}
	}
	resp = signIn("correct horse battery staple")
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("sign-in after five failures = %d Retry-After %q, want 429", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

// TestSignInLimitPerForwardedClient checks the sign-in IP limit behind a trusted proxy
// (ADR-0019 §5): the clients that the proxy forwards have limits of their own, while the
// forwarding header of a peer outside trusted_proxies is ignored.
func TestSignInLimitPerForwardedClient(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		trusted string
		// separate reports whether a second forwarded client still gets through.
		separate bool
	}{
		{name: "trusted proxy", trusted: "127.0.0.0/8, ::1", separate: true},
		{name: "untrusted peer", trusted: "192.0.2.0/24", separate: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true), "MY_TRUSTED_PROXIES": tt.trusted})
			s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
				_, err := a.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
					Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
				})
				if err != nil {
					t.Fatalf("CreateAdmin: %v", err)
				}
			})
			c := &authClient{t: t, s: s, cookies: map[string]string{}}
			signIn := func(client string) int {
				form := c.form("email", "alice@example.com", "password", "wrong password")
				resp, _ := c.do(http.MethodPost, authpages.PathSignIn, form, map[string]string{"X-Forwarded-For": "198.51.100.1, " + client})
				return resp.StatusCode
			}
			for i := range identity.LoginIPLimit.Max {
				if got := signIn("203.0.113.1"); got != http.StatusUnprocessableEntity {
					t.Fatalf("failure #%d = %d, want 422", i+1, got)
				}
			}
			if got := signIn("203.0.113.1"); got != http.StatusTooManyRequests {
				t.Fatalf("first client after %d failures = %d, want 429", identity.LoginIPLimit.Max, got)
			}
			want := http.StatusTooManyRequests
			if tt.separate {
				want = http.StatusUnprocessableEntity
			}
			if got := signIn("203.0.113.2"); got != want {
				t.Errorf("second client = %d, want %d", got, want)
			}
		})
	}
}
