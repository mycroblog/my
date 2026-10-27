// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	domain "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// Credentials of the providers that the tests set up at the fake OpenID Connect provider.
const (
	idpClientID     = "my-client"
	idpClientSecret = "s3cret value&more"
)

// fakeOIDC is an OpenID Connect provider behind the App's HTTPClient: testkit.FakeOIDCProvider
// with a token endpoint that redeems the codes that issue registers, once, for a client with
// idpClientID and idpClientSecret, the PKCE verifier, and the redirect URI of the code.
type fakeOIDC struct {
	*testkit.FakeOIDCProvider
	t      *testing.T
	signer crypto.Signer

	mu    sync.Mutex
	codes map[string]idpCode
}

type idpCode struct {
	idToken, challenge, redirectURI string
}

func newFakeOIDC(t *testing.T) *fakeOIDC {
	p := &fakeOIDC{FakeOIDCProvider: testkit.NewFakeOIDCProvider(t), t: t, signer: testkit.NewFakeOIDCSigner(t, "ES256"), codes: map[string]idpCode{}}
	p.AddKey("test", "ES256", p.signer)
	return p
}

// issue registers a code for the authorization request at authURL whose ID token has claims
// over the standard ones.
func (p *fakeOIDC) issue(authURL string, claims map[string]any) string {
	p.t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		p.t.Fatal(err)
	}
	q := u.Query()
	now := time.Now()
	all := map[string]any{
		"iss": testkit.FakeOIDCIssuer, "aud": idpClientID, "sub": "subject-1",
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "nonce": q.Get("nonce"),
	}
	for k, v := range claims {
		all[k] = v
	}
	code := fmt.Sprintf("code-%d", now.UnixNano())
	p.mu.Lock()
	defer p.mu.Unlock()
	p.codes[code] = idpCode{
		idToken:   testkit.SignFakeIDToken(p.t, "ES256", "test", p.signer, all),
		challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri"),
	}
	return code
}

func (p *fakeOIDC) Do(ctx context.Context, req *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	if req.Method != http.MethodPost || req.URL != testkit.FakeOIDCIssuer+"/token" {
		return p.FakeOIDCProvider.Do(ctx, req)
	}
	answer := func(status int, v any) (*kernel.HTTPResponse, error) {
		b, _ := json.Marshal(v)
		return &kernel.HTTPResponse{
			StatusCode: status, URL: req.URL, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(b)),
		}, nil
	}
	if req.Purpose != kernel.HTTPPurposeLoginProvider || !req.NoRedirects {
		return nil, errors.New("token request without the login provider purpose")
	}
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(idpClientID)+":"+url.QueryEscape(idpClientSecret)))
	if req.Header.Get("Authorization") != basic {
		return answer(http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
	}
	form, err := url.ParseQuery(string(req.Body))
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	c, ok := p.codes[form.Get("code")]
	delete(p.codes, form.Get("code"))
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(form.Get("code_verifier")))
	if !ok || form.Get("grant_type") != "authorization_code" || form.Get("redirect_uri") != c.redirectURI ||
		base64.RawURLEncoding.EncodeToString(sum[:]) != c.challenge {
		return answer(http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
	}
	return answer(http.StatusOK, map[string]string{"access_token": "at", "token_type": "Bearer", "id_token": c.idToken})
}

// externalEnv is an App with the fake provider and a second handle on its database, through
// which the tests set up providers and links as the administration does.
type externalEnv struct {
	t   *testing.T
	s   *server
	idp *fakeOIDC
	db  *sqlite.DB
	n   int
}

func newExternalEnv(t *testing.T) *externalEnv {
	t.Helper()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	idp := newFakeOIDC(t)
	s := startApp(t, cfg, platform.AppOptions{HTTPClient: idp}, platform.ServeOptions{}, nil)
	db, err := sqlite.Open(t.Context(), cfg.DatabaseURL, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &externalEnv{t: t, s: s, idp: idp, db: db}
}

func (e *externalEnv) id() string {
	e.n++
	return fmt.Sprintf("01TEST%020d", e.n)
}

// provider sets up an enabled provider at the fake issuer.
func (e *externalEnv) provider(name string, allowSignup bool, claims ...domain.ClaimCondition) string {
	e.t.Helper()
	settings := domain.NewLoginProviderSettings(domain.LoginProviderOIDC)
	settings.DisplayName, settings.AllowSignup, settings.Claims = name, allowSignup, claims
	return e.providerWith(settings)
}

// providerWith sets up an enabled provider with settings at the fake issuer.
func (e *externalEnv) providerWith(settings domain.LoginProviderSettings) string {
	e.t.Helper()
	ctx := e.t.Context()
	id, revID, secretID, changeID := e.id(), e.id(), e.id(), e.id()
	sealed, err := e.s.app.Sealer.Seal(ctx, []byte(idpClientSecret), kernel.AAD(secretID, "login_provider_secrets.sealed_secret"))
	if err != nil {
		e.t.Fatal(err)
	}
	settings.Issuer, settings.ClientID = testkit.FakeOIDCIssuer, idpClientID
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

func (e *externalEnv) disable(providerID string) {
	e.t.Helper()
	ctx := e.t.Context()
	r := e.db.IdentityRepository()
	err := e.db.Tx(ctx, func(tx uow.Tx) error {
		h, err := r.LoginProviderHeadForUpdate(ctx, tx, providerID)
		if err != nil {
			return err
		}
		c := domain.LoginProviderStateChange{ID: e.id(), ProviderID: providerID, State: domain.LoginProviderDisabled, ChangedAt: time.Now()}
		if err := r.InsertLoginProviderStateChange(ctx, tx, c); err != nil {
			return err
		}
		h.State = domain.LoginProviderDisabled
		return r.UpsertLoginProviderHead(ctx, tx, h)
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

// link links the account subject at a provider to a user.
func (e *externalEnv) link(providerID, subject, userID string) {
	e.t.Helper()
	ctx := e.t.Context()
	r := e.db.IdentityRepository()
	err := e.db.Tx(ctx, func(tx uow.Tx) error {
		x, err := r.EnsureExternalIdentity(ctx, tx, domain.ExternalIdentity{ID: e.id(), ProviderID: providerID, Subject: subject, CreatedAt: time.Now()})
		if err != nil {
			return err
		}
		return r.InsertExternalIdentityLink(ctx, tx, x, domain.ExternalIdentityLink{ID: e.id(), ExternalIdentityID: x.ID, UserID: userID, LinkedAt: time.Now()})
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *externalEnv) user(username, email string) string {
	e.t.Helper()
	reg, err := e.s.app.Identity.CreateAdmin(e.t.Context(), identity.CreateAdminInput{
		Username: username, Email: email, Password: "correct horse battery staple",
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return reg.User.ID
}

func (e *externalEnv) setMode(mode instance.RegistrationMode) {
	e.t.Helper()
	if _, err := e.s.app.Instance.SetSetting(e.t.Context(), appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(mode),
	}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *externalEnv) client() *authClient {
	return &authClient{t: e.t, s: e.s, cookies: map[string]string{}}
}

// begin starts a sign-in with a provider and returns the authorization URL.
func begin(c *authClient, providerID string) string {
	c.t.Helper()
	resp, body := c.do(http.MethodGet, "/auth/external/"+providerID+"/start", nil, nil)
	if resp.StatusCode != http.StatusSeeOther || c.cookies[authpages.CookieExternal] == "" {
		c.t.Fatalf("start = %d\n%s", resp.StatusCode, body)
	}
	return resp.Header.Get("Location")
}

// callback answers a sign-in as the provider does.
func callback(c *authClient, providerID, code, state string) (*http.Response, string) {
	c.t.Helper()
	return c.do(http.MethodGet, "/auth/external/"+providerID+"/callback?"+url.Values{"code": {code}, "state": {state}}.Encode(), nil, nil)
}

// signInWith runs a whole sign-in with a provider whose ID token has claims.
func (e *externalEnv) signInWith(c *authClient, providerID string, claims map[string]any) (*http.Response, string) {
	e.t.Helper()
	authURL := begin(c, providerID)
	u, _ := url.Parse(authURL)
	return callback(c, providerID, e.idp.issue(authURL, claims), u.Query().Get("state"))
}

func (e *externalEnv) sessionUser(c *authClient) string {
	e.t.Helper()
	s, err := e.s.app.Identity.Authenticate(e.t.Context(), c.cookies[authpages.CookieSession])
	if err != nil {
		return ""
	}
	return s.UserID
}

var ticketField = regexp.MustCompile(`name="ticket" value="([^"]+)"`)

// TestExternalSignIn signs a linked user in with an OpenID Connect provider (ADR-0106 §3).
func TestExternalSignIn(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	alice := e.user("alice", "alice@example.com")
	id := e.provider(`Example <IdP> & Co`, false)
	e.link(id, "subject-alice", alice)

	t.Run("buttons", func(t *testing.T) {
		c := e.client()
		_, body := c.do(http.MethodGet, authpages.PathSignIn+"?return_to=/home", nil, nil)
		want := `<a class="button button-secondary" href="/auth/external/` + id + `/start?return_to=%2Fhome" dir="auto">Example &lt;IdP&gt; &amp; Co</a>`
		if !strings.Contains(body, want) {
			t.Errorf("sign-in page lacks %s:\n%s", want, body)
		}
	})

	t.Run("authorization request", func(t *testing.T) {
		u, err := url.Parse(begin(e.client(), id))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		if u.Scheme+"://"+u.Host+u.Path != testkit.FakeOIDCIssuer+"/authorize" || q.Get("response_type") != "code" ||
			q.Get("client_id") != idpClientID || q.Get("redirect_uri") != "https://example.test/auth/external/"+id+"/callback" ||
			q.Get("scope") != "openid email profile" || q.Get("code_challenge_method") != "S256" ||
			len(q.Get("state")) < 43 || len(q.Get("nonce")) < 43 || len(q.Get("code_challenge")) != 43 || q.Has("hd") {
			t.Errorf("authorization request = %s", u)
		}
	})

	t.Run("sign-in", func(t *testing.T) {
		c := e.client()
		resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-alice", "email": "someone@example.org"})
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" || e.sessionUser(c) != alice {
			t.Fatalf("callback = %d %q, session of %q\n%s", resp.StatusCode, resp.Header.Get("Location"), e.sessionUser(c), body)
		}
		if _, ok := c.cookies[authpages.CookieExternal]; ok {
			t.Error("the callback kept the cookie of the sign-in")
		}
	})

	t.Run("return_to", func(t *testing.T) {
		c := e.client()
		resp, _ := c.do(http.MethodGet, "/auth/external/"+id+"/start?return_to=/settings", nil, nil)
		authURL := resp.Header.Get("Location")
		u, _ := url.Parse(authURL)
		resp, _ = callback(c, id, e.idp.issue(authURL, map[string]any{"sub": "subject-alice"}), u.Query().Get("state"))
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/settings" {
			t.Errorf("callback = %d %q, want /settings", resp.StatusCode, resp.Header.Get("Location"))
		}
	})

	t.Run("state", func(t *testing.T) {
		c := e.client()
		authURL := begin(c, id)
		u, _ := url.Parse(authURL)
		state, cookie := u.Query().Get("state"), c.cookies[authpages.CookieExternal]
		// A missing state.
		if resp, body := callback(c, id, e.idp.issue(authURL, map[string]any{"sub": "subject-alice"}), ""); resp.StatusCode != http.StatusUnprocessableEntity ||
			!strings.Contains(body, "The sign-in took too long or was already used.") || e.sessionUser(c) != "" {
			t.Errorf("callback without a state = %d\n%s", resp.StatusCode, body)
		}
		// A replayed state, with the cookie the browser had.
		c.cookies[authpages.CookieExternal] = cookie
		if resp, body := callback(c, id, e.idp.issue(authURL, map[string]any{"sub": "subject-alice"}), state); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("callback = %d\n%s", resp.StatusCode, body)
		}
		c2 := e.client()
		c2.cookies[authpages.CookieExternal] = cookie
		if resp, _ := callback(c2, id, e.idp.issue(authURL, map[string]any{"sub": "subject-alice"}), state); resp.StatusCode != http.StatusUnprocessableEntity || e.sessionUser(c2) != "" {
			t.Errorf("replayed callback = %d, want 422", resp.StatusCode)
		}
		// A missing cookie.
		c3 := e.client()
		if resp, _ := callback(c3, id, e.idp.issue(authURL, map[string]any{"sub": "subject-alice"}), state); resp.StatusCode != http.StatusUnprocessableEntity || e.sessionUser(c3) != "" {
			t.Errorf("callback without the cookie = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("a cancelled sign-in", func(t *testing.T) {
		c := e.client()
		u, _ := url.Parse(begin(c, id))
		resp, body := c.do(http.MethodGet, "/auth/external/"+id+"/callback?error=access_denied&state="+url.QueryEscape(u.Query().Get("state")), nil, nil)
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "did not complete") || e.sessionUser(c) != "" {
			t.Errorf("cancelled callback = %d\n%s", resp.StatusCode, body)
		}
	})

	t.Run("an unlinked account", func(t *testing.T) {
		c := e.client()
		resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-nobody", "email": "alice@example.com", "email_verified": true})
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "No account on this server is linked") || e.sessionUser(c) != "" {
			t.Errorf("callback of an unlinked account = %d\n%s", resp.StatusCode, body)
		}
	})
}

// TestExternalSignInTwoFactor asks a linked user with TOTP for the code (ADR-0106 §3).
func TestExternalSignInTwoFactor(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	ctx := t.Context()
	alice := e.user("alice", "alice@example.com")
	id := e.provider("IdP", false)
	e.link(id, "subject-alice", alice)
	setup, err := e.s.app.Identity.BeginTOTPSetup(ctx, alice)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(setup.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.app.Identity.EnableTOTP(ctx, alice, setup.Token, domain.TOTPCode(secret, domain.TOTPStep(time.Now()))); err != nil {
		t.Fatal(err)
	}
	c := e.client()
	resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-alice"})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != authpages.PathSignInTwoFactor ||
		c.cookies[authpages.CookieTwoFactor] == "" || e.sessionUser(c) != "" {
		t.Fatalf("callback = %d %q\n%s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	// The code of the next step (the one EnableTOTP used is spent) completes it.
	code := domain.TOTPCode(secret, domain.TOTPStep(time.Now().Add(30*time.Second)))
	resp, body = c.do(http.MethodPost, authpages.PathSignInTwoFactor, c.formAt(authpages.PathSignInTwoFactor, "code", code), nil)
	if resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
		t.Errorf("code = %d\n%s", resp.StatusCode, body)
	}
}

// TestExternalSignInPolicy checks the provider's conditions on every sign-in and refuses a
// disabled provider (ADR-0106 §2, §4).
func TestExternalSignInPolicy(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	alice := e.user("alice", "alice@example.com")
	id := e.provider("Workspace", false, domain.ClaimCondition{Claim: "hd", Values: []string{"example.com"}})
	other := e.provider("Other", false)
	e.link(id, "subject-alice", alice)

	if u, _ := url.Parse(begin(e.client(), id)); u.Query().Get("hd") != "example.com" {
		t.Errorf("authorization request = %s, want hd=example.com", u)
	}
	c := e.client()
	if resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-alice", "hd": "other.example"}); resp.StatusCode != http.StatusForbidden ||
		!strings.Contains(body, "does not accept that account") || e.sessionUser(c) != "" {
		t.Errorf("callback with another hd = %d\n%s", resp.StatusCode, body)
	}
	if resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-alice"}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("callback without hd = %d\n%s", resp.StatusCode, body)
	}
	if resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-alice", "hd": "example.com"}); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) != alice {
		t.Errorf("callback with the hd = %d\n%s", resp.StatusCode, body)
	}

	// A disabled provider: no button, no start, and no callback of a sign-in begun before.
	c = e.client()
	authURL := begin(c, other)
	e.disable(other)
	u, _ := url.Parse(authURL)
	if resp, body := callback(c, other, e.idp.issue(authURL, map[string]any{"sub": "subject-alice"}), u.Query().Get("state")); resp.StatusCode != http.StatusNotFound {
		t.Errorf("callback of a disabled provider = %d\n%s", resp.StatusCode, body)
	}
	if resp, _ := c.do(http.MethodGet, "/auth/external/"+other+"/start", nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("start of a disabled provider = %d", resp.StatusCode)
	}
	if resp, _ := c.do(http.MethodGet, "/auth/external/unknown/start", nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("start of an unknown provider = %d", resp.StatusCode)
	}
	if _, body := c.do(http.MethodGet, authpages.PathSignIn, nil, nil); strings.Contains(body, other) || !strings.Contains(body, id) {
		t.Errorf("sign-in page buttons:\n%s", body)
	}
}

// TestExternalSignUp signs up with an account that no user links, per registration mode, and
// never links a user whose address matches (ADR-0106 §3).
func TestExternalSignUp(t *testing.T) {
	t.Parallel()
	e := newExternalEnv(t)
	ctx := t.Context()
	e.user("alice", "alice@example.com")
	id := e.provider("IdP", true)
	noSignup := e.provider("Closed IdP", false)
	repo := e.db.IdentityRepository()

	// signUp runs a sign-in of subject and sends the registration form with fields.
	signUp := func(c *authClient, subject string, claims map[string]any, fields ...string) (*http.Response, string) {
		t.Helper()
		all := map[string]any{"sub": subject}
		for k, v := range claims {
			all[k] = v
		}
		resp, body := e.signInWith(c, id, all)
		m := ticketField.FindStringSubmatch(body)
		if resp.StatusCode != http.StatusOK || m == nil || strings.Contains(body, `name="password"`) {
			t.Fatalf("callback = %d, want the registration form without a password\n%s", resp.StatusCode, body)
		}
		form := url.Values{"csrf_token": {c.cookies[authpages.CookieCSRF]}, "ticket": {m[1]}}
		for i := 0; i+1 < len(fields); i += 2 {
			form.Set(fields[i], fields[i+1])
		}
		return c.do(http.MethodPost, authpages.PathExternalSignUp, form, nil)
	}
	linked := func(subject string) string {
		t.Helper()
		l, err := repo.FindLinkedExternalIdentity(ctx, id, subject)
		if errors.Is(err, identity.ErrNotFound) {
			return ""
		}
		if err != nil {
			t.Fatal(err)
		}
		return l.UserID
	}

	t.Run("closed", func(t *testing.T) {
		e.setMode(instance.RegistrationClosed)
		c := e.client()
		if resp, body := e.signInWith(c, id, map[string]any{"sub": "subject-closed"}); resp.StatusCode != http.StatusUnprocessableEntity ||
			!strings.Contains(body, "No account on this server is linked") {
			t.Errorf("callback while closed = %d\n%s", resp.StatusCode, body)
		}
	})

	t.Run("buttons", func(t *testing.T) {
		e.setMode(instance.RegistrationOpen)
		_, body := e.client().do(http.MethodGet, authpages.PathSignUp, nil, nil)
		if !strings.Contains(body, `/auth/external/`+id+`/start`) || strings.Contains(body, noSignup) {
			t.Errorf("registration page buttons:\n%s", body)
		}
	})

	t.Run("allow_signup off", func(t *testing.T) {
		c := e.client()
		authURL := begin(c, noSignup)
		u, _ := url.Parse(authURL)
		if resp, body := callback(c, noSignup, e.idp.issue(authURL, map[string]any{"sub": "subject-x"}), u.Query().Get("state")); resp.StatusCode != http.StatusUnprocessableEntity ||
			!strings.Contains(body, "No account on this server is linked") {
			t.Errorf("callback = %d\n%s", resp.StatusCode, body)
		}
	})

	t.Run("open with a verified address", func(t *testing.T) {
		e.setMode(instance.RegistrationOpen)
		c := e.client()
		resp, body := signUp(c, "subject-bob", map[string]any{"email": "bob@example.org", "email_verified": true, "preferred_username": "Bob"},
			"username", "bob", "email", "bob@example.org")
		if resp.StatusCode != http.StatusSeeOther || e.sessionUser(c) == "" {
			t.Fatalf("sign-up = %d\n%s", resp.StatusCode, body)
		}
		bob := e.sessionUser(c)
		if linked("subject-bob") != bob {
			t.Errorf("subject-bob is linked to %q, want %q", linked("subject-bob"), bob)
		}
		if has, err := e.s.app.Identity.HasPassword(ctx, bob); err != nil || has {
			t.Errorf("HasPassword = %v, %v; want none", has, err)
		}
		if access, err := e.s.app.Identity.UserAccess(ctx, bob); err != nil || !access.EmailVerified {
			t.Errorf("UserAccess = %+v, %v; want the address verified", access, err)
		}
		// The new user signs in with the provider.
		c2 := e.client()
		if resp, _ := e.signInWith(c2, id, map[string]any{"sub": "subject-bob"}); resp.StatusCode != http.StatusSeeOther || e.sessionUser(c2) != bob {
			t.Errorf("sign-in of the new user = %d", resp.StatusCode)
		}
	})

	t.Run("open with an unverified address", func(t *testing.T) {
		c := e.client()
		resp, body := signUp(c, "subject-carol", map[string]any{"email": "carol@example.org", "email_verified": false},
			"username", "carol", "email", "carol@example.org")
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("sign-up = %d\n%s", resp.StatusCode, body)
		}
		if access, err := e.s.app.Identity.UserAccess(ctx, e.sessionUser(c)); err != nil || access.EmailVerified {
			t.Errorf("UserAccess = %+v, %v; want unverified", access, err)
		}
	})

	t.Run("a matching address never links", func(t *testing.T) {
		c := e.client()
		resp, body := signUp(c, "subject-mallory", map[string]any{"email": "alice@example.com", "email_verified": true},
			"username", "mallory", "email", "alice@example.com")
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "This email address is already in use.") ||
			e.sessionUser(c) != "" || linked("subject-mallory") != "" {
			t.Errorf("sign-up with alice's address = %d, linked to %q\n%s", resp.StatusCode, linked("subject-mallory"), body)
		}
	})

	t.Run("approval", func(t *testing.T) {
		e.setMode(instance.RegistrationApproval)
		c := e.client()
		resp, body := signUp(c, "subject-dave", map[string]any{"email": "dave@example.org", "email_verified": true},
			"username", "dave", "email", "dave@example.org", "reason", "hello")
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("sign-up = %d\n%s", resp.StatusCode, body)
		}
		if access, err := e.s.app.Identity.UserAccess(ctx, e.sessionUser(c)); err != nil || access.Registration != domain.RegistrationPending {
			t.Errorf("UserAccess = %+v, %v; want pending", access, err)
		}
	})

	t.Run("invite", func(t *testing.T) {
		e.setMode(instance.RegistrationInvite)
		c := e.client()
		resp, body := signUp(c, "subject-erin", nil, "username", "erin", "email", "erin@example.org")
		if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "Enter your invite code.") || linked("subject-erin") != "" {
			t.Fatalf("sign-up without an invite = %d\n%s", resp.StatusCode, body)
		}
		inv, err := e.s.app.Identity.CreateInvite(ctx, identity.CreateInviteInput{})
		if err != nil {
			t.Fatal(err)
		}
		m := ticketField.FindStringSubmatch(body)
		form := url.Values{"csrf_token": {c.cookies[authpages.CookieCSRF]}, "ticket": {m[1]},
			"username": {"erin"}, "email": {"erin@example.org"}, "invite_code": {inv.Code}}
		if resp, body := c.do(http.MethodPost, authpages.PathExternalSignUp, form, nil); resp.StatusCode != http.StatusSeeOther || linked("subject-erin") == "" {
			t.Fatalf("sign-up with an invite = %d\n%s", resp.StatusCode, body)
		}
		// The ticket works once.
		form.Set("username", "erin2")
		form.Set("email", "erin2@example.org")
		if resp, body := c.do(http.MethodPost, authpages.PathExternalSignUp, form, nil); resp.StatusCode != http.StatusUnprocessableEntity ||
			!strings.Contains(body, "The sign-in took too long or was already used.") {
			t.Errorf("second sign-up with the ticket = %d\n%s", resp.StatusCode, body)
		}
	})
}
