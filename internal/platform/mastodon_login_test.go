// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

// The fake Mastodon servers: mastoServer serves its metadata and registers clients, oldServer
// is older than 4.3 and has no metadata, and closedServer does not let others register.
const (
	mastoServer  = "https://masto.example"
	oldServer    = "https://old.example"
	closedServer = "https://closed.example"
)

// fakeMastodon is the Mastodon servers behind the App's HTTPClient. It registers clients at
// POST /api/v1/apps, redeems at /oauth/token the codes that issue registers, once, for the
// client, the PKCE verifier, and the redirect URI, and answers verify_credentials for the
// access token of the code.
type fakeMastodon struct {
	t  *testing.T
	mu sync.Mutex
	// apps are the registered clients by client_id: their secret, redirect URI, and scopes.
	apps     map[string]url.Values
	codes    map[string]mastoCode
	accounts map[string]map[string]string // by access token
	requests []string
}

type mastoCode struct {
	clientID, challenge, redirectURI string
	account                          map[string]string
}

func newFakeMastodon(t *testing.T) *fakeMastodon {
	return &fakeMastodon{t: t, apps: map[string]url.Values{}, codes: map[string]mastoCode{}, accounts: map[string]map[string]string{}}
}

func mastoAnswer(req *kernel.HTTPRequest, status int, v any) (*kernel.HTTPResponse, error) {
	b, _ := json.Marshal(v)
	return &kernel.HTTPResponse{
		StatusCode: status, URL: req.URL, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(b)),
	}, nil
}

func (m *fakeMastodon) Do(_ context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	if req.Purpose != kernel.HTTPPurposeLoginProvider || !req.NoRedirects {
		return nil, fmt.Errorf("request to %s without the login provider purpose", req.URL)
	}
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}
	server := "https://" + u.Host
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, method+" "+req.URL)
	notFound := func() (*kernel.HTTPResponse, error) {
		return mastoAnswer(req, http.StatusNotFound, map[string]string{"error": "Record not found"})
	}
	if server == oldServer {
		return notFound()
	}
	switch method + " " + u.Path {
	case "GET /.well-known/oauth-authorization-server":
		return mastoAnswer(req, http.StatusOK, map[string]any{
			"issuer": server + "/", "authorization_endpoint": server + "/oauth/authorize", "token_endpoint": server + "/oauth/token",
			"app_registration_endpoint": server + "/api/v1/apps", "scopes_supported": []string{"read", "write", "profile"},
			"code_challenge_methods_supported": []string{"S256"},
		})
	case "POST /api/v1/apps":
		if server == closedServer {
			return mastoAnswer(req, http.StatusForbidden, map[string]string{"error": "This action is not allowed"})
		}
		form, err := url.ParseQuery(string(req.Body))
		if err != nil {
			return nil, err
		}
		id := fmt.Sprintf("client-%d", len(m.apps)+1)
		form.Set("client_secret", "secret of "+id)
		m.apps[id] = form
		return mastoAnswer(req, http.StatusOK, map[string]string{"id": "1", "client_id": id, "client_secret": form.Get("client_secret")})
	case "POST /oauth/token":
		form, err := url.ParseQuery(string(req.Body))
		if err != nil {
			return nil, err
		}
		c, ok := m.codes[form.Get("code")]
		delete(m.codes, form.Get("code"))
		app := m.apps[c.clientID]
		basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(c.clientID)+":"+url.QueryEscape(app.Get("client_secret"))))
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		if !ok || app == nil || req.Header.Get("Authorization") != basic || form.Get("grant_type") != "authorization_code" ||
			form.Get("redirect_uri") != c.redirectURI || base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge {
			return mastoAnswer(req, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		}
		token := "token-" + form.Get("code")
		m.accounts[token] = c.account
		return mastoAnswer(req, http.StatusOK, map[string]string{"access_token": token, "token_type": "Bearer", "scope": "profile"})
	case "GET /api/v1/accounts/verify_credentials":
		a, ok := m.accounts[strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")]
		if !ok {
			return mastoAnswer(req, http.StatusUnauthorized, map[string]string{"error": "The access token is invalid"})
		}
		return mastoAnswer(req, http.StatusOK, a)
	}
	return notFound()
}

// issue registers a code of account for the authorization request at authURL.
func (m *fakeMastodon) issue(authURL string, account map[string]string) string {
	m.t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		m.t.Fatal(err)
	}
	q := u.Query()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.apps[q.Get("client_id")]; !ok || q.Get("scope") != "profile" {
		m.t.Fatalf("authorization request %s of an unknown client or another scope", authURL)
	}
	code := fmt.Sprintf("code-%d", len(m.codes)+len(m.accounts))
	m.codes[code] = mastoCode{
		clientID: q.Get("client_id"), challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri"), account: account,
	}
	return code
}

func newMastodonEnv(t *testing.T) (*externalEnv, *fakeMastodon) {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	m := newFakeMastodon(t)
	s := startApp(t, cfg, platform.AppOptions{HTTPClient: m}, platform.ServeOptions{}, nil)
	db, err := sqlite.Open(t.Context(), cfg.DatabaseURL, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &externalEnv{t: t, s: s, db: db}, m
}

// mastodonProvider sets up a mastodon provider at server as the administration does.
func mastodonProvider(e *externalEnv, server, clientID, secret string, allowSignup bool) (identity.LoginProviderView, error) {
	settings := domain.NewLoginProviderSettings(domain.LoginProviderMastodon)
	settings.DisplayName, settings.Issuer, settings.ClientID, settings.AllowSignup = server, strings.TrimPrefix(server, "https://"), clientID, allowSignup
	return e.s.app.Identity.CreateLoginProvider(e.t.Context(), settings, secret, "")
}

// mastodonSignIn runs a whole sign-in of account with a provider.
func mastodonSignIn(m *fakeMastodon, c *authClient, providerID string, account map[string]string) (*http.Response, string) {
	c.t.Helper()
	authURL := begin(c, providerID)
	u, _ := url.Parse(authURL)
	return callback(c, providerID, m.issue(authURL, account), u.Query().Get("state"))
}

// TestMastodonLoginProviderRegistration registers a client at the server named by the
// administrator, and refuses a server without metadata (ADR-0106 §1).
func TestMastodonLoginProviderRegistration(t *testing.T) {
	t.Parallel()
	e, m := newMastodonEnv(t)
	ctx := t.Context()

	v, err := mastodonProvider(e, mastoServer, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	p := v.Provider
	if p.Settings.Issuer != mastoServer || p.Settings.ClientID != "client-1" {
		t.Errorf("provider = %+v", p.Settings)
	}
	app := m.apps["client-1"]
	if app.Get("redirect_uris") != "https://example.test/auth/external/"+p.ID+"/callback" || app.Get("scopes") != "profile" ||
		app.Get("client_name") != "example.test" {
		t.Errorf("registration = %v", app)
	}
	sec, err := e.db.IdentityRepository().FindLoginProviderSecret(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := e.s.app.Sealer.Open(ctx, sec.Sealed, kernel.AAD(sec.ID, domain.LoginProviderSecretColumn)); err != nil || string(plain) != "secret of client-1" {
		t.Errorf("stored client secret = %q, %v", plain, err)
	}

	// The client_id and client_secret registered at a server that does not let others register.
	if _, err := mastodonProvider(e, closedServer, "", "", false); !errors.Is(err, identity.ErrMastodonRegistration) {
		t.Errorf("registration at a closed server: %v, want ErrMastodonRegistration", err)
	}
	m.apps["manual"] = url.Values{"client_secret": {"manual secret"}}
	v, err = mastodonProvider(e, closedServer, "manual", "manual secret", false)
	if err != nil || v.Provider.Settings.ClientID != "manual" {
		t.Errorf("manual client: %+v, %v", v.Provider.Settings, err)
	}
	if _, err := mastodonProvider(e, closedServer, "", "a secret alone", false); !errors.Is(err, domain.ErrInvalidLoginProviderClientID) {
		t.Errorf("secret without client_id: %v", err)
	}

	// A server older than 4.3, without metadata, is refused, with a client or without.
	for _, c := range [][2]string{{"", ""}, {"manual", "manual secret"}} {
		if _, err := mastodonProvider(e, oldServer, c[0], c[1], false); !errors.Is(err, identity.ErrMastodonServer) {
			t.Errorf("server without metadata (client_id %q): %v, want ErrMastodonServer", c[0], err)
		}
	}
	for _, r := range m.requests {
		if strings.HasPrefix(r, "POST "+oldServer) {
			t.Errorf("registered at a server without metadata: %s", r)
		}
	}

	// The server stays.
	moved := p.Settings
	moved.Issuer = closedServer
	if _, err := e.s.app.Identity.UpdateLoginProvider(ctx, p.ID, moved, ""); !errors.Is(err, domain.ErrInvalidLoginProviderURL) {
		t.Errorf("moving the provider to another server: %v", err)
	}
}

// TestMastodonSignIn signs a linked user in with the profile scope and the id of
// verify_credentials, and refuses the server while its domain has a domain action.
func TestMastodonSignIn(t *testing.T) {
	t.Parallel()
	e, m := newMastodonEnv(t)
	ctx := t.Context()
	alice := e.user("alice", "alice@example.com")
	v, err := mastodonProvider(e, mastoServer, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	id := v.Provider.ID
	e.link(id, "109", alice)

	t.Run("authorization request", func(t *testing.T) {
		u, err := url.Parse(begin(e.client(), id))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if u.Scheme+"://"+u.Host+u.Path != mastoServer+"/oauth/authorize" || q.Get("response_type") != "code" ||
			q.Get("client_id") != "client-1" || q.Get("redirect_uri") != "https://example.test/auth/external/"+id+"/callback" ||
			q.Get("scope") != "profile" || q.Get("code_challenge_method") != "S256" || len(q.Get("state")) < 43 ||
			q.Has("nonce") || q.Has("force_login") {
			t.Errorf("authorization request = %s", u)
		}
	})

	t.Run("sign-in", func(t *testing.T) {
		c := e.client()
		resp, body := mastodonSignIn(m, c, id, map[string]string{"id": "109", "username": "alice", "acct": "alice", "display_name": "Alice"})
		if resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
			t.Fatalf("callback = %d, session of %q\n%s", resp.StatusCode, e.sessionUser(c), body)
		}
	})

	t.Run("an unlinked account", func(t *testing.T) {
		c := e.client()
		resp, body := mastodonSignIn(m, c, id, map[string]string{"id": "110", "username": "alice"})
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "No account on this server is linked") || e.sessionUser(c) != "" {
			t.Errorf("callback = %d\n%s", resp.StatusCode, body)
		}
	})

	t.Run("domain action", func(t *testing.T) {
		if _, err := e.s.app.Moderation.IssueDomainAction(ctx, appmoderation.DomainActionInput{Domain: "masto.example", Kind: moderation.DomainLimit}); err != nil {
			t.Fatal(err)
		}
		c := e.client()
		// The button is gone, and the start and a sign-in under way are refused.
		if _, body := c.do(http.MethodGet, authpages.PathSignIn, nil, nil); strings.Contains(body, "/auth/external/"+id+"/start") {
			t.Errorf("sign-in page offers a server under a domain action:\n%s", body)
		}
		if resp, body := c.do(http.MethodGet, "/auth/external/"+id+"/start", nil, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("start = %d\n%s", resp.StatusCode, body)
		}
		if _, err := e.s.app.Identity.BeginExternalLogin(ctx, id, ""); !errors.Is(err, identity.ErrLoginProviderUnavailable) {
			t.Errorf("BeginExternalLogin = %v, want ErrLoginProviderUnavailable", err)
		}
	})
}

// TestMastodonSignUp signs a new user up with a Mastodon account, which gives no email
// address: the address is verified by mail.
func TestMastodonSignUp(t *testing.T) {
	t.Parallel()
	e, m := newMastodonEnv(t)
	ctx := t.Context()
	e.setMode(instance.RegistrationOpen)
	v, err := mastodonProvider(e, mastoServer, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	id := v.Provider.ID

	c := e.client()
	resp, body := mastodonSignIn(m, c, id, map[string]string{"id": "42", "username": "Bob", "acct": "Bob", "display_name": "Bob B."})
	ticket := ticketField.FindStringSubmatch(body)
	if resp.StatusCode != http.StatusOK || ticket == nil {
		t.Fatalf("callback = %d, want the registration form\n%s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `name="username" type="text" value="bob"`) || strings.Contains(body, "confirmed this address") {
		t.Errorf("registration form:\n%s", body)
	}
	form := url.Values{"csrf_token": {c.cookies[authpages.CookieCSRF]}, "ticket": {ticket[1]}, "username": {"bob"}, "email": {"bob@example.org"}}
	if resp, body := c.do(http.MethodPost, authpages.PathExternalSignUp, form, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-up = %d\n%s", resp.StatusCode, body)
	}
	bob := e.sessionUser(c)
	if l, err := e.db.IdentityRepository().FindLinkedExternalIdentity(ctx, id, "42"); err != nil || l.UserID != bob || bob == "" {
		t.Errorf("link = %+v, %v; want %q", l, err, bob)
	}
	if access, err := e.s.app.Identity.UserAccess(ctx, bob); err != nil || access.EmailVerified {
		t.Errorf("UserAccess = %+v, %v; want the address unverified", access, err)
	}
}
