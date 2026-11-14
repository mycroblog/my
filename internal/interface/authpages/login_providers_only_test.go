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
		Identity: id, Locales: &i18n.Resolver{Bundle: b}, Passkeys: p, PasswordReset: r,
		ExternalLogins: &fakeExternalLogins{id: id},
	}).Register(m)
	return id, p, r, m
}

// TestPagesStopOfferingWhatIsRefused checks that the pages show the forms with the limit off and
// only the providers with it on.
func TestPagesStopOfferingWhatIsRefused(t *testing.T) {
	_, _, _, on := newLimitedServer(t, true)
	data := screenData(t, newBrowser(t, on).do(http.MethodGet, authpages.PathSignIn+"?return_to=/home", nil, nil))
	if at(data, "loginProvidersOnly") != true || at(data, "passkey") != nil || at(data, "canResetPassword") != nil ||
		at(data, "humanCheck") != nil || at(data, "canSignUp") != true {
		t.Errorf("limited sign-in page offers a form, or no registration: %v", data)
	}
	if got := externalButtons(data); len(got) != 2 || !strings.HasSuffix(got[0], "/auth/external/corp/start?return_to=%2Fhome") ||
		!strings.HasSuffix(got[1], "/auth/external/lab/start?return_to=%2Fhome") {
		t.Errorf("limited sign-in page buttons = %q", got)
	}
	data = screenData(t, newBrowser(t, on).do(http.MethodGet, authpages.PathSignUp, nil, nil))
	if at(data, "signUp.loginProvidersOnly") != true {
		t.Errorf("limited registration page offers the form: %v", data)
	}
	if got := externalButtons(data); len(got) != 1 || !strings.HasSuffix(got[0], "/auth/external/corp/start") {
		t.Errorf("limited registration page buttons = %q", got)
	}
	data = screenData(t, newBrowser(t, on).do(http.MethodGet, authpages.PathForgotPassword, nil, nil))
	if at(data, "forgotPassword.loginProvidersOnly") != true || at(data, "csrfToken") != nil {
		t.Errorf("limited forgot-password page: %v", data)
	}

	_, _, _, off := newLimitedServer(t, false)
	data = screenData(t, newBrowser(t, off).do(http.MethodGet, authpages.PathSignIn, nil, nil))
	if at(data, "loginProvidersOnly") != nil || text(data, "passkey.challenge") == "" || at(data, "canResetPassword") != true ||
		len(externalButtons(data)) != 2 {
		t.Errorf("sign-in page without the limit: %v", data)
	}
	data = screenData(t, newBrowser(t, off).do(http.MethodGet, authpages.PathSignUp, nil, nil))
	if at(data, "signUp.loginProvidersOnly") != nil {
		t.Errorf("registration page without the limit: %v", data)
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
			data := screenData(t, res)
			if res.Code != http.StatusForbidden || !strings.Contains(text(data, "error"), want) {
				t.Errorf("%s %d = %d\n%s", name, i, res.Code, got)
			}
			limited := at(data, "loginProvidersOnly") == true || at(data, "signUp.loginProvidersOnly") == true
			if !limited || at(data, "passkey") != nil || strings.Contains(res.Header().Get("Set-Cookie"), authpages.CookieSession) {
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
		if data := screenData(t, res); res.Code != http.StatusForbidden || !strings.Contains(text(data, "error"), refusedLink) || at(data, "newPassword") != nil {
			t.Errorf("reset page with %q = %d\n%s", token, res.Code, res.Body)
		}
	}
	// The page that was open before the limit: the service refuses it.
	r.err = appidentity.ErrLoginProvidersOnly
	res := b.do(http.MethodPost, authpages.PathResetPassword, b.form(open, "token", "my_prt_good", "password", "correct horse battery"), nil)
	if res.Code != http.StatusForbidden || !strings.Contains(text(screenData(t, res), "error"), refusedLink) {
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
		data := screenData(t, res)
		if res.Code != http.StatusForbidden || at(data, "forgotPassword.loginProvidersOnly") != true || at(data, "csrfToken") != nil {
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
	data := screenData(t, res)
	if res.Code != http.StatusForbidden || !strings.Contains(text(data, "error"), refusedSignIn) {
		t.Errorf("code = %d\n%s", res.Code, res.Body)
	}
	if text(data, "screen") == "sign_in" && at(data, "loginProvidersOnly") != true {
		t.Errorf("the refusal offers the password form: %v", data)
	}
}

// TestInviteLinkReachesTheProviderForm carries the invite code of a link through a provider to the
// form, with or without the limit.
func TestInviteLinkReachesTheProviderForm(t *testing.T) {
	id, x, h := newExternalServer(t)
	id.providersOnly = true
	b := newBrowser(t, h)
	data := screenData(t, b.do(http.MethodGet, authpages.PathSignUp+"?invite=CODE1", nil, nil))
	const want = `/auth/external/corp/start?return_to=%2Fauth%2Fsign_up%3Finvite%3DCODE1`
	if got := externalButtons(data); len(got) != 1 || !strings.HasSuffix(got[0], " "+want) {
		t.Fatalf("registration page buttons = %q, want %s", got, want)
	}
	view := corpSignUp
	x.outcome = appidentity.ExternalLogin{SignUp: &view, ReturnTo: authpages.PathSignUp + "?invite=CODE1"}
	id.mode = identity.RegistrationInvite
	data = screenData(t, b.do(http.MethodGet, "/auth/external/corp/callback?state=s&code=c", nil, nil))
	if text(data, "signUp.inviteCode") != "CODE1" || at(data, "signUp.askInvite") != true {
		t.Errorf("the provider's sign-up form lost the code: %v", data)
	}
	if text(data, "signUp.external.ticket") == "" {
		t.Errorf("the provider's sign-up form asks for a password: %v", data)
	}
}
