// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"net/http"
	"strings"
	"testing"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

const (
	refusedSignIn = "This server signs in only through the login providers."
	refusedSignUp = "This server registers new users only through the login providers."
	refusedReset  = "so passwords cannot be reset here"
	refusedLink   = "so this link cannot set a password now"
)

// newLimitedServer serves every page with the limit to login providers on or off, on fakes that
// would accept the forms if the pages let them through.
func newLimitedServer(t *testing.T, limited bool) (*fakeIdentity, *fakePasskeys, *fakePasswordReset, http.Handler) {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	id := newFakeIdentity()
	id.providersOnly = limited
	p := &fakePasskeys{used: map[string]bool{}, id: id}
	r := &fakePasswordReset{byEmail: true}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{
		Identity: id, Locales: &i18n.Resolver{Bundle: b}, Passkeys: p, PasskeyScript: passkeyScript, PasswordReset: r,
		ExternalLogins: &fakeExternalLogins{id: id},
	}).Register(m)
	return id, p, r, m
}

// TestPagesStopOfferingWhatIsRefused checks that the pages show the forms with the limit off and
// only the providers with it on.
func TestPagesStopOfferingWhatIsRefused(t *testing.T) {
	_, _, _, on := newLimitedServer(t, true)
	body := newBrowser(t, on).do(http.MethodGet, authpages.PathSignIn+"?return_to=/home", nil, nil).Body.String()
	for _, gone := range []string{`action="/auth/sign_in"`, `action="/auth/sign_in/passkey"`, `name="password"`, "<script", "/auth/forgot_password"} {
		if strings.Contains(body, gone) {
			t.Errorf("limited sign-in page has %s:\n%s", gone, body)
		}
	}
	for _, want := range []string{refusedSignIn, "/auth/external/corp/start?return_to=%2Fhome", "/auth/external/lab/start", `href="/auth/sign_up"`} {
		if !strings.Contains(body, want) {
			t.Errorf("limited sign-in page lacks %s:\n%s", want, body)
		}
	}
	body = newBrowser(t, on).do(http.MethodGet, authpages.PathSignUp, nil, nil).Body.String()
	for _, gone := range []string{`action="/auth/sign_up"`, `name="password"`, "/auth/external/lab/start"} {
		if strings.Contains(body, gone) {
			t.Errorf("limited registration page has %s:\n%s", gone, body)
		}
	}
	for _, want := range []string{refusedSignUp, "/auth/external/corp/start"} {
		if !strings.Contains(body, want) {
			t.Errorf("limited registration page lacks %s:\n%s", want, body)
		}
	}
	body = newBrowser(t, on).do(http.MethodGet, authpages.PathForgotPassword, nil, nil).Body.String()
	if strings.Contains(body, `action="/auth/forgot_password"`) || !strings.Contains(body, refusedReset) {
		t.Errorf("limited forgot-password page:\n%s", body)
	}

	_, _, _, off := newLimitedServer(t, false)
	body = newBrowser(t, off).do(http.MethodGet, authpages.PathSignIn, nil, nil).Body.String()
	for _, want := range []string{`action="/auth/sign_in"`, `action="/auth/sign_in/passkey"`, "/auth/forgot_password", "/auth/external/corp/start"} {
		if !strings.Contains(body, want) {
			t.Errorf("sign-in page without the limit lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, refusedSignIn) {
		t.Errorf("sign-in page without the limit says it is limited:\n%s", body)
	}
	body = newBrowser(t, off).do(http.MethodGet, authpages.PathSignUp, nil, nil).Body.String()
	if !strings.Contains(body, `action="/auth/sign_up"`) || strings.Contains(body, refusedSignUp) {
		t.Errorf("registration page without the limit:\n%s", body)
	}
}

// TestRefusalsAreTheSameForEveryone posts each form as a page left open before the limit began,
// with a right, a wrong, an unknown, and an empty value. Each answer is the same refusal, and
// nothing reaches the service.
func TestRefusalsAreTheSameForEveryone(t *testing.T) {
	id, p, _, h := newLimitedServer(t, false)
	b := newBrowser(t, h)
	open := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	id.providersOnly = true

	check := func(name, path, want string, forms [][]string) {
		t.Helper()
		var first string
		for i, fields := range forms {
			res := b.do(http.MethodPost, path, b.form(open, fields...), nil)
			got := res.Body.String()
			if res.Code != http.StatusForbidden || !strings.Contains(got, want) {
				t.Errorf("%s %d = %d\n%s", name, i, res.Code, got)
			}
			if strings.Contains(got, `name="password"`) || strings.Contains(res.Header().Get("Set-Cookie"), authpages.CookieSession) {
				t.Errorf("%s %d offered the form again or a session:\n%s", name, i, got)
			}
			if i == 0 {
				first = got
			} else if got != first {
				t.Errorf("%s %d differs from the first answer:\n%s\n---\n%s", name, i, got, first)
			}
		}
	}
	check("sign-in", authpages.PathSignIn, refusedSignIn, [][]string{
		{"email", "alice@example.com", "password", "correct horse"},
		{"email", "alice@example.com", "password", "wrong"},
		{"email", "nobody@example.com", "password", "correct horse"},
		{"email", "", "password", ""},
	})
	check("passkey", authpages.PathSignInPasskey, refusedSignIn, [][]string{
		{"challenge", "challenge-1", "credential", `{"id":"alice"}`},
		{"challenge", "challenge-9", "credential", `{"id":"nobody"}`},
		{"challenge", "", "credential", ""},
	})
	check("sign-up", authpages.PathSignUp, refusedSignUp, [][]string{
		{"username", "alice", "email", "alice@example.com", "password", "correct horse battery"},
		{"username", "nobody", "email", "nobody@example.com", "password", "correct horse battery"},
		{"username", "", "email", "", "password", ""},
	})
	if id.last != (appidentity.SignInInput{}) || len(p.inputs) != 0 || len(id.signUps) != 0 {
		t.Errorf("the forms reached the service: %+v %+v %+v", id.last, p.inputs, id.signUps)
	}
}

// TestResetLinkIsRefusedWhateverTheToken checks that a page tells nothing about the token, and
// that the token stays usable for when the limit ends.
func TestResetLinkIsRefusedWhateverTheToken(t *testing.T) {
	id, _, r, h := newLimitedServer(t, false)
	b := newBrowser(t, h)
	open := b.do(http.MethodGet, authpages.PathResetPassword+"?token=my_prt_good", nil, nil)
	if open.Code != http.StatusOK {
		t.Fatalf("reset page without the limit = %d", open.Code)
	}
	id.providersOnly = true
	for _, token := range []string{"my_prt_good", "my_prt_bad", ""} {
		res := b.do(http.MethodGet, authpages.PathResetPassword+"?token="+token, nil, nil)
		if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), refusedLink) || strings.Contains(res.Body.String(), `name="password"`) {
			t.Errorf("reset page with %q = %d\n%s", token, res.Code, res.Body)
		}
	}
	// The page that was open before the limit: the service refuses it.
	r.err = appidentity.ErrLoginProvidersOnly
	res := b.do(http.MethodPost, authpages.PathResetPassword, b.form(open, "token", "my_prt_good", "password", "correct horse battery"), nil)
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), refusedLink) {
		t.Errorf("reset form = %d\n%s", res.Code, res.Body)
	}
	if r.used || len(r.resets) != 0 {
		t.Errorf("the refused reset used the token: %+v", r.resets)
	}
}

// TestForgotPasswordRefusedWhateverTheAddress posts the form that was open before the limit.
func TestForgotPasswordRefusedWhateverTheAddress(t *testing.T) {
	id, _, r, h := newLimitedServer(t, false)
	b := newBrowser(t, h)
	open := b.do(http.MethodGet, authpages.PathForgotPassword, nil, nil)
	id.providersOnly = true
	r.err = appidentity.ErrLoginProvidersOnly
	var first string
	for i, email := range []string{"alice@example.com", "nobody@example.com", "not an address", ""} {
		res := b.do(http.MethodPost, authpages.PathForgotPassword, b.form(open, "email", email), nil)
		got := res.Body.String()
		if res.Code != http.StatusForbidden || !strings.Contains(got, refusedReset) || strings.Contains(got, `action="/auth/forgot_password"`) {
			t.Errorf("request for %q = %d\n%s", email, res.Code, got)
		}
		if strings.Contains(got, email) && email != "" {
			t.Errorf("the answer repeats %q:\n%s", email, got)
		}
		if i == 0 {
			first = got
		} else if got != first {
			t.Errorf("the answer for %q differs:\n%s\n---\n%s", email, got, first)
		}
	}
}

// TestTwoFactorStepOfAPasswordSignInIsRefused posts the code of a sign-in that began before the
// limit: the service refuses it, and the browser keeps no challenge.
func TestTwoFactorStepOfAPasswordSignInIsRefused(t *testing.T) {
	id := newFakeIdentity()
	h := newServer(t, id)
	b := newBrowser(t, h)
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	res := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "bob@example.com", "password", "correct horse"), nil)
	if res.Code != http.StatusSeeOther {
		t.Fatalf("sign-in = %d\n%s", res.Code, res.Body)
	}
	step := b.do(http.MethodGet, authpages.PathSignInTwoFactor, nil, nil)
	id.providersOnly = true
	id.completeErr = appidentity.ErrLoginProvidersOnly
	res = b.do(http.MethodPost, authpages.PathSignInTwoFactor, b.form(step, "code", "123456"), nil)
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), refusedSignIn) {
		t.Errorf("code = %d\n%s", res.Code, res.Body)
	}
	if strings.Contains(res.Body.String(), `name="password"`) {
		t.Errorf("the refusal offers the password form:\n%s", res.Body)
	}
}

// TestInviteLinkReachesTheProviderForm carries the invite code of a link through a provider to the
// form, with or without the limit.
func TestInviteLinkReachesTheProviderForm(t *testing.T) {
	id, x, h := newExternalServer(t)
	id.providersOnly = true
	b := newBrowser(t, h)
	body := b.do(http.MethodGet, authpages.PathSignUp+"?invite=CODE1", nil, nil).Body.String()
	const want = `/auth/external/corp/start?return_to=%2Fauth%2Fsign_up%3Finvite%3DCODE1`
	if !strings.Contains(body, want) {
		t.Fatalf("registration page lacks %s:\n%s", want, body)
	}
	view := corpSignUp
	x.outcome = appidentity.ExternalLogin{SignUp: &view, ReturnTo: authpages.PathSignUp + "?invite=CODE1"}
	id.mode = identity.RegistrationInvite
	body = b.do(http.MethodGet, "/auth/external/corp/callback?state=s&code=c", nil, nil).Body.String()
	if !strings.Contains(body, `name="invite_code" type="text" value="CODE1"`) {
		t.Errorf("the provider's sign-up form lost the code:\n%s", body)
	}
	if strings.Contains(body, `name="password"`) {
		t.Errorf("the provider's sign-up form asks for a password:\n%s", body)
	}
}
