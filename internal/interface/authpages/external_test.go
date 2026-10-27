// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// fakeExternalLogins offers two providers, of which only "corp" lets users sign up. Its callback
// answers with outcome, or err, and records the inputs.
type fakeExternalLogins struct {
	id        *fakeIdentity
	outcome   appidentity.ExternalLogin
	err       error
	finished  []appidentity.ExternalLoginInput
	signUps   []appidentity.ExternalSignUpInput
	signUpErr error
}

func (f *fakeExternalLogins) ExternalLoginProviders(context.Context) ([]appidentity.ExternalLoginProvider, error) {
	return []appidentity.ExternalLoginProvider{
		{ID: "corp", Kind: identity.LoginProviderOIDC, DisplayName: "Corp <SSO>", AllowSignup: true},
		{ID: "lab", Kind: identity.LoginProviderOIDC, DisplayName: "研究室 ID"},
	}, nil
}

func (f *fakeExternalLogins) BeginExternalLogin(_ context.Context, providerID, returnTo string) (appidentity.ExternalLoginStart, error) {
	if providerID != "corp" && providerID != "lab" {
		return appidentity.ExternalLoginStart{}, appidentity.ErrLoginProviderUnavailable
	}
	return appidentity.ExternalLoginStart{
		AuthorizationURL: "https://idp.example/authorize?state=s&return=" + returnTo,
		Cookie:           "sealed-" + providerID, ExpiresAt: time.Now().Add(10 * time.Minute),
	}, nil
}

func (f *fakeExternalLogins) FinishExternalLogin(_ context.Context, in appidentity.ExternalLoginInput) (appidentity.ExternalLogin, error) {
	f.finished = append(f.finished, in)
	return f.outcome, f.err
}

var corpSignUp = appidentity.ExternalSignUpView{
	Ticket: "ticket-1", ProviderID: "corp", ProviderName: "Corp <SSO>",
	Email: "dana@corp.example", EmailVerified: true, Username: "dana",
}

func (f *fakeExternalLogins) ExternalSignUp(_ context.Context, ticket string) (appidentity.ExternalSignUpView, error) {
	if ticket != corpSignUp.Ticket {
		return appidentity.ExternalSignUpView{}, appidentity.ErrExternalLoginExpired
	}
	return corpSignUp, nil
}

func (f *fakeExternalLogins) SignUpExternal(_ context.Context, in appidentity.ExternalSignUpInput) (appidentity.SignedUp, error) {
	f.signUps = append(f.signUps, in)
	if f.signUpErr != nil {
		return appidentity.SignedUp{}, f.signUpErr
	}
	return appidentity.SignedUp{Session: appidentity.SignedIn{Token: f.id.newToken(userBob), Session: identity.Session{UserID: userBob}}}, nil
}

func newExternalServer(t *testing.T) (*fakeIdentity, *fakeExternalLogins, http.Handler) {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	id := newFakeIdentity()
	x := &fakeExternalLogins{id: id}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{Identity: id, Locales: &i18n.Resolver{Bundle: b}, ExternalLogins: x}).Register(m)
	return id, x, m
}

func TestSignInPageOffersExternalLogins(t *testing.T) {
	_, _, h := newExternalServer(t)
	body := newBrowser(t, h).do(http.MethodGet, authpages.PathSignIn+"?return_to=/notifications", nil, nil).Body.String()
	for _, want := range []string{
		`<h2 id="external-logins-heading" class="hint">Or sign in with</h2>`,
		// The display names are shown as the administrators wrote them, escaped.
		`<a class="button button-secondary" href="/auth/external/corp/start?return_to=%2Fnotifications" dir="auto">Corp &lt;SSO&gt;</a>`,
		`<a class="button button-secondary" href="/auth/external/lab/start?return_to=%2Fnotifications" dir="auto">研究室 ID</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("sign-in page lacks %s:\n%s", want, body)
		}
	}
	// The registration page offers only the providers that let users sign up.
	body = newBrowser(t, h).do(http.MethodGet, authpages.PathSignUp, nil, nil).Body.String()
	if !strings.Contains(body, "/auth/external/corp/start") || strings.Contains(body, "/auth/external/lab/start") {
		t.Errorf("registration page buttons:\n%s", body)
	}
	// Without external logins, neither page has the section.
	plain := newServer(t, newFakeIdentity())
	for _, path := range []string{authpages.PathSignIn, authpages.PathSignUp} {
		if body := newBrowser(t, plain).do(http.MethodGet, path, nil, nil).Body.String(); strings.Contains(body, "external-logins-heading") {
			t.Errorf("%s offers external logins without any:\n%s", path, body)
		}
	}
}

func TestExternalLoginStart(t *testing.T) {
	_, _, h := newExternalServer(t)
	w := newBrowser(t, h).do(http.MethodGet, "/auth/external/corp/start?return_to=//evil.example", nil, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "https://idp.example/authorize?state=s&return=" {
		t.Errorf("start = %d %q", w.Code, w.Header().Get("Location"))
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == authpages.CookieExternal {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value != "sealed-corp" || !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 || cookie.MaxAge > 600 {
		t.Errorf("cookie = %+v", cookie)
	}
	if w := newBrowser(t, h).do(http.MethodGet, "/auth/external/other/start", nil, nil); w.Code != http.StatusNotFound ||
		!strings.Contains(w.Body.String(), "This way of signing in is not available.") {
		t.Errorf("start of an unknown provider = %d\n%s", w.Code, w.Body.String())
	}
}

func TestExternalLoginCallback(t *testing.T) {
	for _, c := range []struct {
		name     string
		outcome  appidentity.ExternalLogin
		err      error
		status   int
		location string
		text     string
	}{
		{name: "signed in", outcome: appidentity.ExternalLogin{ReturnTo: "/settings", SignedIn: &appidentity.SignedIn{
			Token: "my_ses_x", Session: identity.Session{UserID: userAlice},
		}}, status: http.StatusSeeOther, location: "/settings"},
		{name: "second factor", outcome: appidentity.ExternalLogin{SecondFactorChallenge: "challenge"},
			status: http.StatusSeeOther, location: authpages.PathSignInTwoFactor},
		{name: "expired", err: appidentity.ErrExternalLoginExpired, status: http.StatusUnprocessableEntity, text: "The sign-in took too long"},
		{name: "denied", err: appidentity.ErrExternalLoginDenied, status: http.StatusForbidden, text: "does not accept that account"},
		{name: "not linked", err: appidentity.ErrExternalAccountNotLinked, status: http.StatusUnprocessableEntity, text: "No account on this server is linked"},
		{name: "failed", err: appidentity.ErrExternalLoginFailed, status: http.StatusUnprocessableEntity, text: "did not complete"},
		{name: "disabled", err: appidentity.ErrLoginProviderUnavailable, status: http.StatusNotFound, text: "not available"},
	} {
		t.Run(c.name, func(t *testing.T) {
			id, x, h := newExternalServer(t)
			if c.outcome.SignedIn != nil {
				c.outcome.SignedIn.Token = id.newToken(userAlice)
			}
			x.outcome, x.err = c.outcome, c.err
			b := newBrowser(t, h)
			b.cookies[authpages.CookieExternal] = "sealed-corp"
			w := b.do(http.MethodGet, "/auth/external/corp/callback?code=abc&state=xyz", nil, nil)
			if w.Code != c.status || w.Header().Get("Location") != c.location || !strings.Contains(w.Body.String(), c.text) {
				t.Errorf("callback = %d %q\n%s", w.Code, w.Header().Get("Location"), w.Body.String())
			}
			if _, ok := b.cookies[authpages.CookieExternal]; ok {
				t.Error("the callback kept the cookie")
			}
			in := x.finished[0]
			if in.ProviderID != "corp" || in.Cookie != "sealed-corp" || in.Code != "abc" || in.State != "xyz" {
				t.Errorf("FinishExternalLogin input = %+v", in)
			}
		})
	}
}

func TestExternalSignUpPage(t *testing.T) {
	_, x, h := newExternalServer(t)
	x.outcome = appidentity.ExternalLogin{SignUp: &corpSignUp}
	b := newBrowser(t, h)
	page := b.do(http.MethodGet, "/auth/external/corp/callback?code=abc&state=xyz", nil, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK {
		t.Fatalf("callback = %d\n%s", page.Code, body)
	}
	for _, want := range []string{
		`<h1 dir="auto">Create an account with Corp &lt;SSO&gt;</h1>`,
		`<form class="form" method="post" action="/auth/external/sign_up">`,
		`<input type="hidden" name="ticket" value="ticket-1">`,
		`name="username" type="text" value="dana"`,
		`name="email" type="email" value="dana@corp.example"`,
		`Corp &lt;SSO&gt; confirmed this address.`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("registration form lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, `name="password"`) || strings.Contains(body, "external-logins-heading") {
		t.Errorf("registration form of an external account has a password or the provider buttons:\n%s", body)
	}

	// A field that fails shows the form again with the ticket.
	w := b.do(http.MethodPost, authpages.PathExternalSignUp, b.form(page, "ticket", "ticket-1", "username", "Dana!", "email", "dana@corp.example"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `name="ticket" value="ticket-1"`) || len(x.signUps) != 0 {
		t.Errorf("invalid sign-up = %d\n%s", w.Code, w.Body.String())
	}
	// What the identity module refuses shows the form again too.
	x.signUpErr = appidentity.ErrEmailTaken
	w = b.do(http.MethodPost, authpages.PathExternalSignUp, b.form(page, "ticket", "ticket-1", "username", "dana", "email", "alice@example.com"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "This email address is already in use.") ||
		!strings.Contains(w.Body.String(), `name="ticket" value="ticket-1"`) {
		t.Errorf("sign-up with a taken address = %d\n%s", w.Code, w.Body.String())
	}
	// A used ticket starts again.
	x.signUpErr = appidentity.ErrExternalIdentityLinked
	w = b.do(http.MethodPost, authpages.PathExternalSignUp, b.form(page, "ticket", "ticket-1", "username", "dana", "email", "dana@corp.example"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "The sign-in took too long or was already used.") {
		t.Errorf("sign-up with a used ticket = %d\n%s", w.Code, w.Body.String())
	}
	x.signUpErr = nil
	w = b.do(http.MethodPost, authpages.PathExternalSignUp, b.form(page, "ticket", "ticket-1", "username", "dana", "email", "dana@corp.example"), nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn || b.cookies[authpages.CookieSession] == "" {
		t.Fatalf("sign-up = %d %q\n%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if in := x.signUps[len(x.signUps)-1]; in.Ticket != "ticket-1" || in.Username != "dana" || in.Email != "dana@corp.example" {
		t.Errorf("SignUpExternal input = %+v", in)
	}
	// A forged ticket is refused.
	n := len(x.signUps)
	w = b.do(http.MethodPost, authpages.PathExternalSignUp, b.form(page, "ticket", "forged", "username", "eve", "email", "eve@example.com"), nil)
	if w.Code != http.StatusUnprocessableEntity || len(x.signUps) != n {
		t.Errorf("sign-up with a forged ticket = %d\n%s", w.Code, w.Body.String())
	}
}
