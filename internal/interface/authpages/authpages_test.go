// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeIdentity signs alice@example.com in with "correct horse" and keeps the live tokens. Every
// session belongs to the user alice unless SignUp made it; access holds each user's Access
// (allowed when absent).
type fakeIdentity struct {
	live    map[string]bool
	n       int
	limited bool
	last    appidentity.SignInInput
	// second records the inputs of CompleteSignIn; challengeUsed makes bob's challenge expired.
	second        []appidentity.SecondFactorInput
	challengeUsed bool

	// providersOnly is the answer of LoginProvidersOnly; completeErr fails CompleteSignIn.
	providersOnly bool
	completeErr   error

	mode      identity.RegistrationMode
	owner     map[string]string // token -> user ID
	access    map[string]appidentity.Access
	signUps   []appidentity.SignUpInput
	signUpErr error
	// signUpReady makes a new user able to use the account at once (an open registration
	// without email confirmation).
	signUpReady bool
	verified    map[string]bool // tokens redeemed by VerifyEmail
	resent      int
	resendErr   error
}

func newFakeIdentity() *fakeIdentity {
	return &fakeIdentity{
		live: map[string]bool{}, mode: identity.RegistrationOpen, owner: map[string]string{},
		access: map[string]appidentity.Access{}, verified: map[string]bool{},
	}
}

// The users and accounts the fakes know.
var (
	userAlice    = testkit.NamedID("user-alice")
	userBob      = testkit.NamedID("user-bob")
	accountAlice = testkit.NamedID("account-alice")
)

func (f *fakeIdentity) newToken(userID string) string {
	f.n++
	token := "my_ses_token" + strconv.Itoa(f.n)
	f.live[token] = true
	f.owner[token] = userID
	return token
}

func (f *fakeIdentity) SignIn(_ context.Context, in appidentity.SignInInput) (appidentity.SignedIn, error) {
	f.last = in
	if f.limited {
		return appidentity.SignedIn{}, &kernel.RateLimitedError{RetryAfter: 61 * time.Second}
	}
	if in.Email == "bob@example.com" && in.Password == "correct horse" {
		return appidentity.SignedIn{}, &appidentity.SecondFactorRequiredError{Challenge: bobChallenge}
	}
	if in.Email != "alice@example.com" || in.Password != "correct horse" {
		return appidentity.SignedIn{}, appidentity.ErrInvalidCredentials
	}
	delete(f.live, in.CurrentToken)
	token := f.newToken(userAlice)
	return appidentity.SignedIn{Token: token, Session: identity.Session{UserID: userAlice, ExpiresAt: time.Now().Add(time.Hour)}}, nil
}

func (f *fakeIdentity) Authenticate(_ context.Context, token string) (identity.Session, error) {
	if !f.live[token] {
		return identity.Session{}, appidentity.ErrInvalidSession
	}
	return identity.Session{ID: token, UserID: f.owner[token]}, nil
}

func (f *fakeIdentity) SignOut(_ context.Context, token string) error {
	delete(f.live, token)
	return nil
}

func (f *fakeIdentity) SignUp(_ context.Context, in appidentity.SignUpInput) (appidentity.SignedUp, error) {
	f.signUps = append(f.signUps, in)
	if f.signUpErr != nil {
		return appidentity.SignedUp{}, f.signUpErr
	}
	userID := testkit.NamedID("new-user-" + strconv.Itoa(len(f.signUps)))
	delete(f.live, in.CurrentToken)
	token := f.newToken(userID)
	f.access[userID] = appidentity.Access{
		Registration: identity.RegistrationPending, Email: in.Email, EmailVerificationRequired: true,
	}
	if f.signUpReady {
		f.access[userID] = appidentity.Access{Registration: identity.RegistrationApproved, Email: in.Email, EmailVerified: true}
	}
	return appidentity.SignedUp{
		Registration: appidentity.Registration{User: identity.User{ID: userID}},
		State:        identity.RegistrationPending,
		Session:      appidentity.SignedIn{Token: token, Session: identity.Session{UserID: userID, ExpiresAt: time.Now().Add(time.Hour)}},
	}, nil
}

func (f *fakeIdentity) RegistrationMode(context.Context) (identity.RegistrationMode, error) {
	return f.mode, nil
}

func (f *fakeIdentity) LoginProvidersOnly(context.Context) (bool, error) {
	return f.providersOnly, nil
}

func (f *fakeIdentity) UserAccess(_ context.Context, userID string) (appidentity.Access, error) {
	if a, ok := f.access[userID]; ok {
		return a, nil
	}
	return appidentity.Access{Registration: identity.RegistrationApproved, EmailVerified: true}, nil
}

func (f *fakeIdentity) VerifyEmail(_ context.Context, token string) (string, error) {
	if token != "my_evt_good" || f.verified[token] {
		return "", appidentity.ErrInvalidToken
	}
	f.verified[token] = true
	return userAlice, nil
}

func (f *fakeIdentity) ResendVerificationEmail(context.Context, string) error {
	if f.resendErr != nil {
		return f.resendErr
	}
	f.resent++
	return nil
}

type mux struct{ *http.ServeMux }

func (m mux) Handle(pattern string, h http.Handler) { m.ServeMux.Handle(pattern, h) }

func newServer(t *testing.T, id authpages.Identity) http.Handler {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{Identity: id, Locales: &i18n.Resolver{Bundle: b}}).Register(m)
	return m
}

// browser is a cookie jar in front of the handler, sending what a same-origin form post sends.
type browser struct {
	t       *testing.T
	h       http.Handler
	cookies map[string]string
}

func newBrowser(t *testing.T, h http.Handler) *browser {
	return &browser{t: t, h: h, cookies: map[string]string{}}
}

func (b *browser) do(method, target string, form url.Values, header map[string]string) *httptest.ResponseRecorder {
	b.t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, "https://social.example"+target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://social.example")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	for k, v := range header {
		r.Header.Set(k, v)
	}
	for name, value := range b.cookies {
		r.Header.Add("Cookie", name+"="+value)
	}
	w := httptest.NewRecorder()
	b.h.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
		} else {
			b.cookies[c.Name] = c.Value
		}
	}
	return w
}

var csrfField = regexp.MustCompile(`"csrfToken":"([^"]+)"`)

// form returns the fields of the page's form with the anti-forgery token filled in.
func (b *browser) form(page *httptest.ResponseRecorder, fields ...string) url.Values {
	b.t.Helper()
	m := csrfField.FindStringSubmatch(page.Body.String())
	if m == nil {
		b.t.Fatalf("no csrf_token field in %s", page.Body.String())
	}
	v := url.Values{"csrf_token": {m[1]}}
	for i := 0; i+1 < len(fields); i += 2 {
		v.Set(fields[i], fields[i+1])
	}
	return v
}

func TestSignInPage(t *testing.T) {
	b := newBrowser(t, newServer(t, newFakeIdentity()))
	w := b.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "ja"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET sign_in = %d", w.Code)
	}
	h := w.Header()
	if h.Get("Content-Type") != "text/html; charset=utf-8" || h.Get("Cache-Control") != "no-store" || h.Get("Content-Language") != "ja" {
		t.Errorf("headers = %v", h)
	}
	if csp := h.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "unsafe-inline") ||
		!strings.Contains(csp, "form-action 'self'") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	data := screenData(t, w)
	if text(data, "screen") != "sign_in" || at(data, "error") != nil || at(data, "email") != nil {
		t.Errorf("data = %v, want a fresh sign-in form", data)
	}
	csrf, ok := b.cookies[authpages.CookieCSRF]
	if !ok || text(data, "csrfToken") != csrf {
		t.Errorf("the form does not carry the CSRF cookie's token")
	}
	c := w.Result().Cookies()[0]
	if c.Name != authpages.CookieCSRF || !c.Secure || !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("CSRF cookie = %+v", c)
	}
}

func TestSignInIssuesANewSession(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn+"?return_to=%2Foauth%2Fauthorize%3Fx%3D1", nil, nil)
	if got := text(screenData(t, page), "returnTo"); got != "/oauth/authorize?x=1" {
		t.Fatalf("the form drops return_to: returnTo = %q", got)
	}
	w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page,
		"email", " alice@example.com ", "password", "correct horse", "return_to", "/oauth/authorize?x=1"), map[string]string{"User-Agent": "test-agent"})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/oauth/authorize?x=1" {
		t.Fatalf("POST sign_in = %d → %q, want 303 to return_to", w.Code, w.Header().Get("Location"))
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == authpages.CookieSession {
			session = c
		}
	}
	if session == nil || !session.Secure || !session.HttpOnly || session.Path != "/" || session.SameSite != http.SameSiteLaxMode ||
		session.Domain != "" || session.Expires.IsZero() {
		t.Fatalf("session cookie = %+v, want __Host- attributes and an expiry", session)
	}
	if id.last.UserAgent != "test-agent" || id.last.IP != "192.0.2.1" || id.last.Email != "alice@example.com" {
		t.Errorf("SignIn input = %+v", id.last)
	}
	first := session.Value

	// Signing in again from the same browser replaces the session.
	page = b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	if data := screenData(t, page); text(data, "screen") != "signed_in" || text(data, "returnTo") != authpages.PathAppSignIn {
		t.Fatalf("signed-in GET = %v, want the signed-in screen going on to %s", data, authpages.PathAppSignIn)
	}
	b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse"), nil)
	if id.last.CurrentToken != first || b.cookies[authpages.CookieSession] == first || id.live[first] {
		t.Fatalf("second sign-in: current token %q, new cookie %q; want the old session replaced", id.last.CurrentToken, b.cookies[authpages.CookieSession])
	}
}

func TestFailedSignInsLookAlike(t *testing.T) {
	b := newBrowser(t, newServer(t, newFakeIdentity()))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	var bodies []string
	for _, creds := range [][2]string{{"alice@example.com", "wrong"}, {"nobody@example.com", "correct horse"}} {
		w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", creds[0], "password", creds[1]), nil)
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("failed sign-in = %d, want 422", w.Code)
		}
		body := w.Body.String()
		data := screenData(t, w)
		if text(data, "screen") != "sign_in" || text(data, "error") != "The email address or password is incorrect." {
			t.Errorf("failed sign-in page lacks the error: %v", data)
		}
		if text(data, "email") != creds[0] || strings.Contains(body, creds[1]+`"`) {
			t.Errorf("the form should keep the email address and drop the password")
		}
		bodies = append(bodies, strings.ReplaceAll(body, creds[0], ""))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("an unknown address and a wrong password answer differently:\n%s\n---\n%s", bodies[0], bodies[1])
	}
	w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com"), nil)
	if w.Code != http.StatusUnprocessableEntity || text(screenData(t, w), "error") != "Enter your email address and password." {
		t.Errorf("empty password = %d %s", w.Code, w.Body.String())
	}
}

func TestRateLimitedSignIn(t *testing.T) {
	id := newFakeIdentity()
	id.limited = true
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse"), nil)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "61" {
		t.Fatalf("rate-limited sign-in = %d Retry-After %q, want 429 and 61", w.Code, w.Header().Get("Retry-After"))
	}
	if !strings.Contains(text(screenData(t, w), "error"), "Try again in 2 minutes.") {
		t.Errorf("rate-limited page = %s", w.Body.String())
	}
}

func TestCrossOriginPostsAreRefused(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	form := b.form(page, "email", "alice@example.com", "password", "correct horse")
	for name, header := range map[string]map[string]string{
		"Sec-Fetch-Site": {"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"},
		"Origin only":    {"Sec-Fetch-Site": "", "Origin": "https://evil.example"},
	} {
		for _, path := range []string{authpages.PathSignIn, authpages.PathSignOut} {
			w := b.do(http.MethodPost, path, form, header)
			if w.Code != http.StatusForbidden || !strings.Contains(text(screenData(t, w), "error"), "came from another site") {
				t.Errorf("%s POST %s = %d %s, want the 403 page", name, path, w.Code, w.Body.String())
			}
		}
	}
	if id.last.Email != "" {
		t.Error("a cross-origin post reached SignIn")
	}
}

func TestPostsNeedTheCSRFToken(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	for name, mutate := range map[string]func(url.Values){
		"missing field": func(v url.Values) { v.Del("csrf_token") },
		"wrong field":   func(v url.Values) { v.Set("csrf_token", strings.Repeat("A", 43)) },
		"missing cookie": func(url.Values) {
			delete(b.cookies, authpages.CookieCSRF)
		},
	} {
		form := b.form(page, "email", "alice@example.com", "password", "correct horse")
		mutate(form)
		w := b.do(http.MethodPost, authpages.PathSignIn, form, map[string]string{"Sec-Fetch-Site": "", "Origin": ""})
		if w.Code != http.StatusForbidden || !strings.Contains(text(screenData(t, w), "error"), "This form has expired.") {
			t.Errorf("%s: POST sign_in = %d, want 403 with the expired-form error", name, w.Code)
		}
		page = w // the error page carries a fresh form
	}
	if id.last.Email != "" {
		t.Error("a post without the CSRF token reached SignIn")
	}
}

func TestSignOut(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse"), nil)
	token := b.cookies[authpages.CookieSession]
	page = b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	w := b.do(http.MethodPost, authpages.PathSignOut, b.form(page), nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Fatalf("POST sign_out = %d → %q", w.Code, w.Header().Get("Location"))
	}
	if id.live[token] || b.cookies[authpages.CookieSession] != "" {
		t.Errorf("sign-out left the session live (%v) or the cookie set (%q)", id.live[token], b.cookies[authpages.CookieSession])
	}
}

func TestReturnToStaysOnSite(t *testing.T) {
	b := newBrowser(t, newServer(t, newFakeIdentity()))
	for _, bad := range []string{"https://evil.example/", "//evil.example/", "/\\evil.example", "javascript:alert(1)", "evil", "/x\nSet-Cookie: a=b"} {
		page := b.do(http.MethodGet, authpages.PathSignIn+"?return_to="+url.QueryEscape(bad), nil, nil)
		if at(screenData(t, page), "returnTo") != nil {
			t.Errorf("return_to %q kept in the form", bad)
		}
		w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse", "return_to", bad), nil)
		if loc := w.Header().Get("Location"); loc != authpages.PathAppSignIn {
			t.Errorf("return_to %q redirected to %q, want %s", bad, loc, authpages.PathAppSignIn)
		}
		delete(b.cookies, authpages.CookieSession)
	}
}
