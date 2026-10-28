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
	dcClientID     = "dc-client"
	dcClientSecret = "dc-secret"
)

// dcAccount is an account at the fake Discord, which a code stands for.
type dcAccount struct {
	id         string
	username   string
	globalName *string
	email      *string
	verified   bool
	guilds     []string
}

type dcCode struct {
	acct                        dcAccount
	challenge, redirectURI, tok string
}

// fakeDiscord is discord.com behind the App's HTTPClient. Its token endpoint takes the client
// credentials in a form body, as Discord documents.
type fakeDiscord struct {
	t *testing.T

	mu     sync.Mutex
	codes  map[string]dcCode
	tokens map[string]dcAccount
	calls  []string
}

func newFakeDiscord(t *testing.T) *fakeDiscord {
	return &fakeDiscord{t: t, codes: map[string]dcCode{}, tokens: map[string]dcAccount{}}
}

func (g *fakeDiscord) issue(authURL string, a dcAccount) string {
	u, err := url.Parse(authURL)
	if err != nil {
		g.t.Fatal(err)
	}
	q := u.Query()
	code := fmt.Sprintf("dccode-%d", time.Now().UnixNano())
	g.mu.Lock()
	defer g.mu.Unlock()
	g.codes[code] = dcCode{acct: a, challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri"), tok: "tok-" + code}
	return code
}

func (g *fakeDiscord) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
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
	if req.URL == "https://discord.com/api/oauth2/token" && req.Method == http.MethodPost {
		if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			return answer(http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		}
		form, _ := url.ParseQuery(string(req.Body))
		c, ok := g.codes[form.Get("code")]
		delete(g.codes, form.Get("code"))
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		switch {
		case form.Get("client_id") != dcClientID || form.Get("client_secret") != dcClientSecret:
			return answer(http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		case form.Get("grant_type") != "authorization_code" || !ok || form.Get("redirect_uri") != c.redirectURI ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge:
			return answer(http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		}
		g.tokens[c.tok] = c.acct
		return answer(http.StatusOK, map[string]any{"access_token": c.tok, "token_type": "Bearer", "expires_in": 604800})
	}
	if u.Host != "discord.com" || req.Method != http.MethodGet {
		return answer(http.StatusNotFound, map[string]string{"message": "404: Not Found"})
	}
	a, ok := g.tokens[strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")]
	if !ok {
		return answer(http.StatusUnauthorized, map[string]string{"message": "401: Unauthorized"})
	}
	switch u.Path {
	case "/api/users/@me":
		user := map[string]any{"id": a.id, "username": a.username, "global_name": a.globalName}
		if a.email != nil {
			user["email"], user["verified"] = *a.email, a.verified
		}
		return answer(http.StatusOK, user)
	case "/api/users/@me/guilds":
		guilds := []map[string]any{}
		for _, id := range a.guilds {
			guilds = append(guilds, map[string]any{"id": id, "name": "Server " + id})
		}
		return answer(http.StatusOK, guilds)
	}
	return answer(http.StatusNotFound, map[string]string{"message": "404: Not Found"})
}

func newDiscordEnv(t *testing.T) (*externalEnv, *fakeDiscord) {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	dc := newFakeDiscord(t)
	s := startApp(t, cfg, platform.AppOptions{HTTPClient: dc}, platform.ServeOptions{}, nil)
	db, err := sqlite.Open(t.Context(), cfg.DatabaseURL, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &externalEnv{t: t, s: s, db: db}, dc
}

// discordProvider sets up an enabled discord provider that only members of guilds may use.
func (e *externalEnv) discordProvider(allowSignup bool, guilds ...string) string {
	e.t.Helper()
	settings := domain.NewLoginProviderSettings(domain.LoginProviderDiscord)
	settings.DisplayName, settings.ClientID, settings.AllowSignup, settings.Guilds = "Discord", dcClientID, allowSignup, guilds
	return e.insertProvider(settings.Normalize(), dcClientSecret)
}

func (g *fakeDiscord) signIn(c *authClient, providerID string, a dcAccount) (*http.Response, string) {
	g.t.Helper()
	authURL := begin(c, providerID)
	u, _ := url.Parse(authURL)
	return callback(c, providerID, g.issue(authURL, a), u.Query().Get("state"))
}

func strPtr(s string) *string { return &s }

// TestDiscordSignIn signs in and signs up with Discord, keeps the subject as the snowflake id,
// uses the address only when Discord verified it, and applies the guild condition on every
// sign-in (ADR-0106 §1, §4).
func TestDiscordSignIn(t *testing.T) {
	t.Parallel()
	e, dc := newDiscordEnv(t)
	ctx := t.Context()
	alice := e.user("alice", "alice@example.com")
	const aliceID, guildA, guildB = "80351110224678912", "100000000000000001", "100000000000000002"
	open := e.discordProvider(true)
	closed := e.discordProvider(false)
	guilds := e.discordProvider(false, guildB, guildA)
	e.link(open, aliceID, alice)
	e.link(guilds, aliceID, alice)
	repo := e.db.IdentityRepository()

	t.Run("authorization request", func(t *testing.T) {
		u, err := url.Parse(begin(e.client(), open))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if u.Scheme+"://"+u.Host+u.Path != "https://discord.com/oauth2/authorize" || q.Get("client_id") != dcClientID ||
			q.Get("redirect_uri") != "https://example.test/auth/external/"+open+"/callback" || q.Get("scope") != "email identify" ||
			q.Get("code_challenge_method") != "S256" || len(q.Get("state")) < 43 || len(q.Get("code_challenge")) != 43 {
			t.Errorf("authorization request = %s", u)
		}
		u, _ = url.Parse(begin(e.client(), guilds))
		if u.Query().Get("scope") != "email guilds identify" {
			t.Errorf("scope with guilds = %q", u.Query().Get("scope"))
		}
	})

	t.Run("button", func(t *testing.T) {
		_, body := e.client().do(http.MethodGet, authpages.PathSignIn, nil, nil)
		if !strings.Contains(body, "/auth/external/"+open+"/start") || !strings.Contains(body, ">Sign in with Discord</a>") {
			t.Errorf("sign-in page:\n%s", body)
		}
	})

	t.Run("sign-in of a linked user without a guild condition", func(t *testing.T) {
		c := e.client()
		// The username changed; the snowflake decides.
		resp, body := dc.signIn(c, open, dcAccount{id: aliceID, username: "alice_renamed", guilds: []string{"999"}})
		if resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
			t.Fatalf("callback = %d, session of %q\n%s", resp.StatusCode, e.sessionUser(c), body)
		}
		for _, call := range dc.calls {
			if strings.Contains(call, "/guilds") {
				t.Errorf("a provider without guilds read %s", call)
			}
		}
	})

	t.Run("sign-up with a verified address", func(t *testing.T) {
		e.setMode(instance.RegistrationOpen)
		c := e.client()
		resp, body := dc.signIn(c, open, dcAccount{id: "200000000000000002", username: "bob_dev", globalName: strPtr("Bob"), email: strPtr("bob@example.org"), verified: true})
		m := ticketField.FindStringSubmatch(body)
		if resp.StatusCode != http.StatusOK || m == nil || !strings.Contains(body, `value="bob@example.org"`) {
			t.Fatalf("callback = %d, want the registration form with the address\n%s", resp.StatusCode, body)
		}
		form := url.Values{"csrf_token": {c.cookies[authpages.CookieCSRF]}, "ticket": {m[1]}, "username": {"bob"}, "email": {"bob@example.org"}}
		if resp, body := c.do(http.MethodPost, authpages.PathExternalSignUp, form, nil); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) == "" {
			t.Fatalf("sign-up = %d\n%s", resp.StatusCode, body)
		}
		bob := e.sessionUser(c)
		if l, err := repo.FindLinkedExternalIdentity(ctx, open, "200000000000000002"); err != nil || l.UserID != bob {
			t.Errorf("link = %+v, %v; want user %s", l, err, bob)
		}
		if access, err := e.s.app.Identity.UserAccess(ctx, bob); err != nil || !access.EmailVerified {
			t.Errorf("UserAccess = %+v, %v; want the address verified", access, err)
		}
	})

	t.Run("sign-up with an unverified address", func(t *testing.T) {
		c := e.client()
		resp, body := dc.signIn(c, open, dcAccount{id: "300000000000000003", username: "carol", email: strPtr("carol@example.org"), verified: false})
		m := ticketField.FindStringSubmatch(body)
		if resp.StatusCode != http.StatusOK || m == nil || strings.Contains(body, "carol@example.org") {
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
		if resp, body := dc.signIn(c, closed, dcAccount{id: "400000000000000004", username: "dave"}); resp.StatusCode != http.StatusUnprocessableEntity ||
			!strings.Contains(body, "No account on this server is linked") {
			t.Errorf("callback = %d\n%s", resp.StatusCode, body)
		}
	})

	t.Run("guilds", func(t *testing.T) {
		c := e.client()
		if resp, body := dc.signIn(c, guilds, dcAccount{id: aliceID, username: "alice", guilds: []string{"999"}}); resp.StatusCode != http.StatusForbidden ||
			!strings.Contains(body, "does not accept that account") || e.sessionUser(c) != "" {
			t.Errorf("callback of a user outside the guilds = %d\n%s", resp.StatusCode, body)
		}
		if resp, body := dc.signIn(c, guilds, dcAccount{id: aliceID, username: "alice", guilds: []string{"999", guildA}}); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
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
