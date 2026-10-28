// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

const (
	ghClientID     = "gh-client"
	ghClientSecret = "gh-secret"
)

// ghAccount is an account at the fake GitHub, which a code stands for.
type ghAccount struct {
	id     int64
	login  string
	emails []map[string]any
	orgs   []string
}

type ghCode struct {
	acct                        ghAccount
	challenge, redirectURI, tok string
}

// fakeGitHub is github.com and api.github.com behind the App's HTTPClient.
type fakeGitHub struct {
	t *testing.T

	mu     sync.Mutex
	codes  map[string]ghCode
	tokens map[string]ghAccount
	calls  []string
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	return &fakeGitHub{t: t, codes: map[string]ghCode{}, tokens: map[string]ghAccount{}}
}

// issue registers a code for the authorization request at authURL.
func (g *fakeGitHub) issue(authURL string, a ghAccount) string {
	u, err := url.Parse(authURL)
	if err != nil {
		g.t.Fatal(err)
	}
	q := u.Query()
	code := fmt.Sprintf("ghcode-%d", time.Now().UnixNano())
	g.mu.Lock()
	defer g.mu.Unlock()
	g.codes[code] = ghCode{acct: a, challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri"), tok: "tok-" + code}
	return code
}

func (g *fakeGitHub) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	if req.Purpose != kernel.HTTPPurposeLoginProvider || !req.NoRedirects {
		return nil, fmt.Errorf("request to %s without the login provider purpose", req.URL)
	}
	answer := func(status int, v any) (*kernel.HTTPResponse, error) {
		b, _ := json.Marshal(v)
		return &kernel.HTTPResponse{StatusCode: status, URL: req.URL, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(b))}, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}
	g.calls = append(g.calls, req.Method+" "+u.Host+u.Path)
	if req.URL == "https://github.com/login/oauth/access_token" && req.Method == http.MethodPost {
		form, _ := url.ParseQuery(string(req.Body))
		c, ok := g.codes[form.Get("code")]
		delete(g.codes, form.Get("code"))
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		switch {
		case form.Get("client_id") != ghClientID || form.Get("client_secret") != ghClientSecret:
			return answer(http.StatusOK, map[string]string{"error": "incorrect_client_credentials"})
		case !ok || form.Get("redirect_uri") != c.redirectURI || base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge:
			return answer(http.StatusOK, map[string]string{"error": "bad_verification_code"})
		}
		g.tokens[c.tok] = c.acct
		return answer(http.StatusOK, map[string]string{"access_token": c.tok, "token_type": "bearer"})
	}
	if u.Host != "api.github.com" || req.Method != http.MethodGet {
		return answer(http.StatusNotFound, map[string]string{"message": "Not Found"})
	}
	a, ok := g.tokens[strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")]
	if !ok {
		return answer(http.StatusUnauthorized, map[string]string{"message": "Bad credentials"})
	}
	switch u.Path {
	case "/user":
		return answer(http.StatusOK, map[string]any{"id": a.id, "login": a.login})
	case "/user/emails":
		return answer(http.StatusOK, a.emails)
	case "/user/orgs":
		orgs := []map[string]any{}
		for _, o := range a.orgs {
			orgs = append(orgs, map[string]any{"login": o})
		}
		return answer(http.StatusOK, orgs)
	}
	return answer(http.StatusNotFound, map[string]string{"message": "Not Found"})
}

func newGitHubEnv(t *testing.T) (*externalEnv, *fakeGitHub) {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	gh := newFakeGitHub(t)
	s := startApp(t, cfg, platform.AppOptions{HTTPClient: gh}, platform.ServeOptions{}, nil)
	db, err := sqlite.Open(t.Context(), cfg.DatabaseURL, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &externalEnv{t: t, s: s, db: db}, gh
}

// githubProvider sets up an enabled github provider that only members of orgs may use.
func (e *externalEnv) githubProvider(allowSignup bool, orgs ...string) string {
	e.t.Helper()
	settings := domain.NewLoginProviderSettings(domain.LoginProviderGitHub)
	settings.DisplayName, settings.ClientID, settings.AllowSignup, settings.Organizations = "GitHub", ghClientID, allowSignup, orgs
	return e.insertProvider(settings, ghClientSecret)
}

// insertProvider stores an enabled provider with settings and the client secret.
func (e *externalEnv) insertProvider(settings domain.LoginProviderSettings, clientSecret string) string {
	e.t.Helper()
	ctx := e.t.Context()
	id, revID, secretID, changeID := e.id(), e.id(), e.id(), e.id()
	sealed, err := e.s.app.Sealer.Seal(ctx, []byte(clientSecret), kernel.AAD(secretID, domain.LoginProviderSecretColumn))
	if err != nil {
		e.t.Fatal(err)
	}
	now := time.Now()
	r := e.db.IdentityRepository()
	err = e.db.Tx(ctx, func(tx uow.Tx) error {
		if err := r.InsertLoginProvider(ctx, tx, id, "", now); err != nil {
			return err
		}
		if err := r.InsertLoginProviderRevision(ctx, tx, domain.LoginProviderRevision{ID: revID, ProviderID: id, Settings: settings, RevisedAt: now}); err != nil {
			return err
		}
		if err := r.InsertLoginProviderSecret(ctx, tx, domain.LoginProviderSecret{ID: secretID, ProviderID: id, Sealed: sealed, CreatedAt: now}); err != nil {
			return err
		}
		if err := r.InsertLoginProviderStateChange(ctx, tx, domain.LoginProviderStateChange{ID: changeID, ProviderID: id, State: domain.LoginProviderEnabled, ChangedAt: now}); err != nil {
			return err
		}
		return r.UpsertLoginProviderHead(ctx, tx, domain.LoginProviderHead{ProviderID: id, RevisionID: revID, State: domain.LoginProviderEnabled, SecretID: secretID})
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (g *fakeGitHub) signIn(c *authClient, providerID string, a ghAccount) (*http.Response, string) {
	g.t.Helper()
	authURL := begin(c, providerID)
	u, _ := url.Parse(authURL)
	return callback(c, providerID, g.issue(authURL, a), u.Query().Get("state"))
}

func verifiedPrimary(addr string) []map[string]any {
	return []map[string]any{
		{"email": "other@example.org", "primary": false, "verified": true},
		{"email": addr, "primary": true, "verified": true},
	}
}

// TestGitHubSignIn signs in and signs up with GitHub, keeps the subject as the numeric id, and
// applies the Organization condition on every sign-in (ADR-0106 §1, §4).
func TestGitHubSignIn(t *testing.T) {
	t.Parallel()
	e, gh := newGitHubEnv(t)
	ctx := t.Context()
	alice := e.user("alice", "alice@example.com")
	open := e.githubProvider(true)
	closed := e.githubProvider(false)
	orgs := e.githubProvider(false, "acme", "labs")
	e.link(open, "1001", alice)
	e.link(orgs, "1001", alice)
	repo := e.db.IdentityRepository()

	t.Run("authorization request", func(t *testing.T) {
		u, err := url.Parse(begin(e.client(), open))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if u.Scheme+"://"+u.Host+u.Path != "https://github.com/login/oauth/authorize" || q.Get("client_id") != ghClientID ||
			q.Get("redirect_uri") != "https://example.test/auth/external/"+open+"/callback" || q.Get("scope") != "user:email" ||
			q.Get("code_challenge_method") != "S256" || len(q.Get("state")) < 43 || len(q.Get("code_challenge")) != 43 {
			t.Errorf("authorization request = %s", u)
		}
		u, _ = url.Parse(begin(e.client(), orgs))
		if u.Query().Get("scope") != "read:org user:email" {
			t.Errorf("scope with Organizations = %q", u.Query().Get("scope"))
		}
	})

	t.Run("button", func(t *testing.T) {
		_, body := e.client().do(http.MethodGet, authpages.PathSignIn, nil, nil)
		if !strings.Contains(body, "/auth/external/"+open+"/start") || !strings.Contains(body, ">Sign in with GitHub</a>") {
			t.Errorf("sign-in page:\n%s", body)
		}
	})

	t.Run("sign-in of a linked user", func(t *testing.T) {
		c := e.client()
		// The login changed; the numeric id decides.
		resp, body := gh.signIn(c, open, ghAccount{id: 1001, login: "alice-renamed", emails: verifiedPrimary("x@example.org")})
		if resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
			t.Fatalf("callback = %d, session of %q\n%s", resp.StatusCode, e.sessionUser(c), body)
		}
		for _, call := range gh.calls {
			if strings.Contains(call, "/user/orgs") {
				t.Errorf("a provider without Organizations read %s", call)
			}
		}
	})

	t.Run("sign-up with a verified primary address", func(t *testing.T) {
		e.setMode(instance.RegistrationOpen)
		c := e.client()
		resp, body := gh.signIn(c, open, ghAccount{id: 2002, login: "Bob-Dev", emails: verifiedPrimary("bob@example.org")})
		m := ticketField.FindStringSubmatch(body)
		if resp.StatusCode != http.StatusOK || m == nil || !strings.Contains(body, `value="bob@example.org"`) {
			t.Fatalf("callback = %d, want the registration form with the address\n%s", resp.StatusCode, body)
		}
		form := url.Values{"csrf_token": {c.cookies[authpages.CookieCSRF]}, "ticket": {m[1]}, "username": {"bob"}, "email": {"bob@example.org"}}
		if resp, body := c.do(http.MethodPost, authpages.PathExternalSignUp, form, nil); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) == "" {
			t.Fatalf("sign-up = %d\n%s", resp.StatusCode, body)
		}
		bob := e.sessionUser(c)
		if l, err := repo.FindLinkedExternalIdentity(ctx, open, "2002"); err != nil || l.UserID != bob {
			t.Errorf("link of 2002 = %+v, %v; want user %s", l, err, bob)
		}
		if access, err := e.s.app.Identity.UserAccess(ctx, bob); err != nil || !access.EmailVerified {
			t.Errorf("UserAccess = %+v, %v; want the address verified", access, err)
		}
	})

	t.Run("sign-up without a verified address", func(t *testing.T) {
		c := e.client()
		emails := []map[string]any{{"email": "carol@example.org", "primary": true, "verified": false}, {"email": "c2@example.org", "primary": false, "verified": true}}
		resp, body := gh.signIn(c, open, ghAccount{id: 3003, login: "carol", emails: emails})
		m := ticketField.FindStringSubmatch(body)
		if resp.StatusCode != http.StatusOK || m == nil || strings.Contains(body, "carol@example.org") || strings.Contains(body, "c2@example.org") {
			t.Fatalf("callback = %d, want the form without an address\n%s", resp.StatusCode, body)
		}
		form := url.Values{"csrf_token": {c.cookies[authpages.CookieCSRF]}, "ticket": {m[1]}, "username": {"carol"}, "email": {"carol@example.org"}}
		if resp, body := c.do(http.MethodPost, authpages.PathExternalSignUp, form, nil); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("sign-up = %d\n%s", resp.StatusCode, body)
		}
		if access, err := e.s.app.Identity.UserAccess(ctx, e.sessionUser(c)); err != nil || access.EmailVerified {
			t.Errorf("UserAccess = %+v, %v; want the address unverified", access, err)
		}
	})

	t.Run("allow_signup off", func(t *testing.T) {
		c := e.client()
		if resp, body := gh.signIn(c, closed, ghAccount{id: 4004, login: "dave"}); resp.StatusCode != http.StatusUnprocessableEntity ||
			!strings.Contains(body, "No account on this server is linked") {
			t.Errorf("callback = %d\n%s", resp.StatusCode, body)
		}
	})

	t.Run("Organizations", func(t *testing.T) {
		c := e.client()
		if resp, body := gh.signIn(c, orgs, ghAccount{id: 1001, login: "alice", orgs: []string{"elsewhere"}}); resp.StatusCode != http.StatusForbidden ||
			!strings.Contains(body, "does not accept that account") || e.sessionUser(c) != "" {
			t.Errorf("callback of a user outside the Organizations = %d\n%s", resp.StatusCode, body)
		}
		if resp, body := gh.signIn(c, orgs, ghAccount{id: 1001, login: "alice", orgs: []string{"elsewhere", "Labs"}}); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
			t.Errorf("callback of a member = %d\n%s", resp.StatusCode, body)
		}
	})

	t.Run("a refused code", func(t *testing.T) {
		c := e.client()
		u, _ := url.Parse(begin(c, open))
		if resp, body := callback(c, open, "forged", u.Query().Get("state")); resp.StatusCode != http.StatusUnprocessableEntity ||
			!strings.Contains(body, "did not complete") || e.sessionUser(c) != "" {
			t.Errorf("callback with a forged code = %d\n%s", resp.StatusCode, body)
		}
	})
}
