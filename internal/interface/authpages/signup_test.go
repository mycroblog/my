// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
)

func wantLacks(t *testing.T, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("page has %s:\n%s", u, body)
		}
	}
}

func TestSignUpPageFollowsTheMode(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	for _, tc := range []struct {
		mode   identity.RegistrationMode
		status int
		// want are the flags of the form that the mode sets; inviteHint is a part of the invite
		// code's hint.
		want       map[string]bool
		inviteHint string
	}{
		// Every mode but closed takes an invite (ADR-0106 §17): optional in open and approval mode,
		// required in invite mode, and filled in from ?invite=.
		{identity.RegistrationOpen, http.StatusOK, map[string]bool{"askInvite": true}, "You can also sign up without one."},
		{identity.RegistrationApproval, http.StatusOK, map[string]bool{"askInvite": true, "askReason": true}, "without waiting for review"},
		{identity.RegistrationInvite, http.StatusOK, map[string]bool{"askInvite": true, "needInvite": true}, "This server accepts new accounts by invitation only."},
		{identity.RegistrationClosed, http.StatusForbidden, map[string]bool{"closed": true}, ""},
	} {
		id.mode = tc.mode
		w := b.do(http.MethodGet, authpages.PathSignUp+"?invite=ABCD", nil, nil)
		if w.Code != tc.status {
			t.Errorf("%s: GET sign_up = %d, want %d", tc.mode, w.Code, tc.status)
		}
		data := screenData(t, w)
		for _, flag := range []string{"closed", "askInvite", "needInvite", "askReason"} {
			if got := at(data, "signUp."+flag) == true; got != tc.want[flag] {
				t.Errorf("%s: signUp.%s = %v, want %v", tc.mode, flag, got, tc.want[flag])
			}
		}
		if tc.mode != identity.RegistrationClosed {
			if text(data, "signUp.inviteCode") != "ABCD" || !strings.Contains(text(data, "signUp.inviteHint"), tc.inviteHint) {
				t.Errorf("%s: signUp = %v, want the invite code from ?invite= and its hint", tc.mode, at(data, "signUp"))
			}
		}
	}
}

func TestSignUpPage(t *testing.T) {
	id := newFakeIdentity()
	id.mode = identity.RegistrationApproval
	b := newBrowser(t, newServer(t, id))
	w := b.do(http.MethodGet, authpages.PathSignUp, nil, map[string]string{"Accept-Language": "ja"})
	h := w.Header()
	if h.Get("Cache-Control") != "no-store" || !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("headers = %v", h)
	}
	data := screenData(t, w)
	for key, want := range map[string]any{
		"screen": "sign_up", "signUp.usernameHint": "半角英字 (a〜z)・数字・アンダースコアで 30 文字まで。あとから変更できません。",
		"signUp.usernameMax": 30.0, "signUp.passwordMin": 8.0, "signUp.passwordMax": 128.0, "signUp.reasonMax": 1000.0,
	} {
		if got := at(data, key); got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if at(data, "error") != nil || at(data, "signUp.errors") != nil || text(data, "csrfToken") == "" {
		t.Errorf("a fresh form = %v, want no error and the anti-forgery token", data)
	}
}

func signUpForm(b *browser, fields ...string) url.Values {
	b.t.Helper()
	return b.form(b.do(http.MethodGet, authpages.PathSignUp, nil, nil), fields...)
}

func TestSignUpSignsTheNewUserIn(t *testing.T) {
	id := newFakeIdentity()
	id.mode = identity.RegistrationApproval
	b := newBrowser(t, newServer(t, id))
	form := signUpForm(b, "username", " alice ", "email", " alice@example.com ", "password", "correct horse", "reason", "I like ducks.")
	w := b.do(http.MethodPost, authpages.PathSignUp, form, map[string]string{"Accept-Language": "ja", "User-Agent": "test-agent"})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Fatalf("POST sign_up = %d → %q, want 303 to the sign-in page\n%s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if len(id.signUps) != 1 {
		t.Fatalf("SignUp called %d times", len(id.signUps))
	}
	in := id.signUps[0]
	if in.Username != "alice" || in.Email != "alice@example.com" || in.Password != "correct horse" || in.Reason != "I like ducks." ||
		in.Locale != "ja" || in.IP != "192.0.2.1" || in.UserAgent != "test-agent" {
		t.Errorf("SignUp input = %+v", in)
	}
	token := b.cookies[authpages.CookieSession]
	if !id.live[token] {
		t.Fatalf("session cookie %q is not the new session", token)
	}

	// The new user is waiting for approval and email confirmation; the sign-in page says so.
	w = b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	body := w.Body.String()
	data := screenData(t, w)
	if text(data, "screen") != "status" || at(data, "status.pending") != true || text(data, "csrfToken") == "" ||
		text(data, "status.verifyEmail") != "Confirm your email address: open the link we sent to alice@example.com." {
		t.Errorf("status of a new user = %v, want pending approval and email confirmation", data)
	}
	// Cloudflare's Email Address Obfuscation leaves the address alone between these comments.
	if off, on, at := strings.Index(body, "<!--email_off-->"), strings.Index(body, "<!--/email_off-->"), strings.Index(body, "alice@example.com"); off < 0 || on < off || at < off || at > on {
		t.Errorf("the address is not between <!--email_off--> and <!--/email_off-->:\n%s", body)
	}

	// The registration page sends a signed-in browser to that page too.
	if w := b.do(http.MethodGet, authpages.PathSignUp, nil, nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Errorf("signed-in GET sign_up = %d → %q", w.Code, w.Header().Get("Location"))
	}
}

// TestSignUpTakesTheBrowserLocale checks that the new account's locale is the supported locale
// that best matches Accept-Language, and empty (the instance default) when none does.
func TestSignUpTakesTheBrowserLocale(t *testing.T) {
	id := newFakeIdentity()
	h := newServer(t, id)
	for i, tc := range []struct{ acceptLanguage, want string }{
		{"ja-JP,ja;q=0.9,en;q=0.8", "ja"},
		{"en-US,en;q=0.9", "en"},
		{"it-IT, nl;q=0.5", ""},
		{"", ""},
	} {
		b := newBrowser(t, h)
		name := "user" + strconv.Itoa(i)
		form := signUpForm(b, "username", name, "email", name+"@example.com", "password", "correct horse")
		w := b.do(http.MethodPost, authpages.PathSignUp, form, map[string]string{"Accept-Language": tc.acceptLanguage})
		if w.Code != http.StatusSeeOther || len(id.signUps) != i+1 {
			t.Fatalf("Accept-Language %q: POST sign_up = %d with %d sign-ups\n%s", tc.acceptLanguage, w.Code, len(id.signUps), w.Body.String())
		}
		if got := id.signUps[i].Locale; got != tc.want {
			t.Errorf("Accept-Language %q: SignUp locale = %q, want %q", tc.acceptLanguage, got, tc.want)
		}
	}
}

func TestSignUpShowsEveryFieldError(t *testing.T) {
	id := newFakeIdentity()
	id.mode = identity.RegistrationInvite
	b := newBrowser(t, newServer(t, id))
	form := signUpForm(b, "username", "al ice", "email", "alice", "password", "short", "invite_code", " ")
	w := b.do(http.MethodPost, authpages.PathSignUp, form, nil)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("POST sign_up = %d, want 422", w.Code)
	}
	data := screenData(t, w)
	for key, want := range map[string]string{ //nolint:gosec // G101: field errors of the form, not credentials
		"error":                     "The account was not created. Check the fields marked below.",
		"signUp.errors.username":    "Use 1 to 30 letters (a to z), digits, and underscores.",
		"signUp.errors.email":       "Enter an email address such as alice@example.com.",
		"signUp.errors.password":    "Use 8 to 128 characters.",
		"signUp.errors.invite_code": "Enter your invite code.",
		"signUp.username":           "al ice",
		"signUp.email":              "alice",
	} {
		if got := text(data, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	wantLacks(t, w.Body.String(), `"short"`)
	if len(id.signUps) != 0 {
		t.Error("an invalid form reached SignUp")
	}
}

func TestSignUpReportsWhatTheServiceRejects(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		// key is where the data says it, and want what it says (a prefix of it).
		key, want string
	}{
		{appaccount.ErrUsernameTaken, http.StatusUnprocessableEntity, "signUp.errors.username", "This username is already taken."},
		{appidentity.ErrEmailTaken, http.StatusUnprocessableEntity, "signUp.errors.email", "This email address is already in use."},
		{appidentity.ErrInvalidInvite, http.StatusUnprocessableEntity, "signUp.errors.invite_code", "This invite code is not valid."},
		{appidentity.ErrRegistrationClosed, http.StatusForbidden, "signUp.closed", ""},
		{appidentity.ErrSignUpRefused, http.StatusForbidden, "error", "This server cannot accept this sign-up."},
		{&kernel.RateLimitedError{RetryAfter: 90 * time.Second}, http.StatusTooManyRequests,
			"error", "Too many sign-up attempts from your network. Try again in 2 minutes."},
	} {
		id := newFakeIdentity()
		id.mode = identity.RegistrationInvite
		id.signUpErr = tc.err
		b := newBrowser(t, newServer(t, id))
		form := signUpForm(b, "username", "alice", "email", "alice@example.com", "password", "correct horse", "invite_code", "ABCD")
		w := b.do(http.MethodPost, authpages.PathSignUp, form, nil)
		if w.Code != tc.status {
			t.Errorf("%v: POST sign_up = %d, want %d", tc.err, w.Code, tc.status)
		}
		data := screenData(t, w)
		if tc.want == "" {
			if at(data, tc.key) != true {
				t.Errorf("%v: %s = %v, want true", tc.err, tc.key, at(data, tc.key))
			}
		} else if got := text(data, tc.key); !strings.HasPrefix(got, tc.want) {
			t.Errorf("%v: %s = %q, want %q", tc.err, tc.key, got, tc.want)
		}
		if _, ok := b.cookies[authpages.CookieSession]; ok {
			t.Errorf("%v: a failed sign-up set a session", tc.err)
		}
		if tc.status == http.StatusTooManyRequests && w.Header().Get("Retry-After") != "90" {
			t.Errorf("Retry-After = %q", w.Header().Get("Retry-After"))
		}
	}
}

func TestSignUpPostsAreProtected(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	form := signUpForm(b, "username", "alice", "email", "alice@example.com", "password", "correct horse")
	if w := b.do(http.MethodPost, authpages.PathSignUp, form, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}); w.Code != http.StatusForbidden {
		t.Errorf("cross-origin POST sign_up = %d, want 403", w.Code)
	}
	bad := url.Values{}
	for k, v := range form {
		bad[k] = v
	}
	bad.Set("csrf_token", strings.Repeat("A", 43))
	w := b.do(http.MethodPost, authpages.PathSignUp, bad, map[string]string{"Sec-Fetch-Site": "", "Origin": ""})
	if w.Code != http.StatusForbidden || !strings.Contains(text(screenData(t, w), "error"), "This form has expired.") {
		t.Errorf("POST sign_up with a wrong token = %d", w.Code)
	}
	if len(id.signUps) != 0 {
		t.Error("a refused post reached SignUp")
	}
}

func TestSignInLinksToSignUpUnlessClosed(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	if at(screenData(t, b.do(http.MethodGet, authpages.PathSignIn, nil, nil)), "canSignUp") != true {
		t.Error("the sign-in screen does not link to the registration")
	}
	id.mode = identity.RegistrationClosed
	if at(screenData(t, b.do(http.MethodGet, authpages.PathSignIn, nil, nil)), "canSignUp") != nil {
		t.Error("the sign-in screen links to a closed registration")
	}
}

func TestSignInOfAPendingUserShowsTheStatus(t *testing.T) {
	id := newFakeIdentity()
	id.access[userAlice] = appidentity.Access{Registration: identity.RegistrationRejected, Email: "alice@example.com", EmailVerificationRequired: true}
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse", "return_to", "/oauth/authorize"), nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn {
		t.Fatalf("sign-in of a rejected user = %d → %q, want the status page", w.Code, w.Header().Get("Location"))
	}
	data := screenData(t, b.do(http.MethodGet, authpages.PathSignIn, nil, nil))
	if text(data, "screen") != "status" || at(data, "status.rejected") != true || at(data, "status.pending") != nil ||
		at(data, "status.verifyEmail") != nil {
		t.Errorf("status of a rejected user = %v, want the rejection only", at(data, "status"))
	}
}

func TestSignUpOfAReadyUserGoesToTheWelcomeStep(t *testing.T) {
	id := newFakeIdentity()
	id.mode = identity.RegistrationOpen
	id.signUpReady = true
	b := newBrowser(t, newServer(t, id))
	form := signUpForm(b, "username", "alice", "email", "alice@example.com", "password", "correct horse")
	w := b.do(http.MethodPost, authpages.PathSignUp, form, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathWelcome {
		t.Fatalf("POST sign_up = %d → %q, want 303 to %s\n%s", w.Code, w.Header().Get("Location"), authpages.PathWelcome, w.Body.String())
	}
}

func TestVerifyEmail(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathVerifyEmail+"?token=my_evt_good", nil, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("GET verify_email = %d", page.Code)
	}
	if data := screenData(t, page); text(data, "screen") != "verify_email" || text(data, "token") != "my_evt_good" {
		t.Errorf("GET verify_email data = %v", data)
	}
	if id.verified["my_evt_good"] {
		t.Fatal("GET redeemed the token")
	}
	w := b.do(http.MethodPost, authpages.PathVerifyEmail, b.form(page, "token", "my_evt_good"), nil)
	// Continuing signs in to the app, which opens its welcome step.
	if data := screenData(t, w); w.Code != http.StatusOK || text(data, "screen") != "verified" || text(data, "returnTo") != authpages.PathWelcome {
		t.Fatalf("POST verify_email = %d %s", w.Code, w.Body.String())
	}
	w = b.do(http.MethodPost, authpages.PathVerifyEmail, b.form(page, "token", "my_evt_good"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(text(screenData(t, w), "error"), "This link is not valid.") {
		t.Errorf("second POST verify_email = %d %s", w.Code, w.Body.String())
	}
	if w := b.do(http.MethodGet, authpages.PathVerifyEmail, nil, nil); w.Code != http.StatusBadRequest {
		t.Errorf("GET verify_email without a token = %d", w.Code)
	}
}

func TestResendVerification(t *testing.T) {
	id := newFakeIdentity()
	id.access[userAlice] = appidentity.Access{Registration: identity.RegistrationApproved, Email: "alice@example.com", EmailVerificationRequired: true}
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "alice@example.com", "password", "correct horse"), nil)
	page = b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	if data := screenData(t, page); !strings.Contains(text(data, "status.verifyEmail"), "open the link we sent to alice@example.com") ||
		at(data, "status.pending") != nil {
		t.Errorf("status = %v, want the email confirmation only", at(data, "status"))
	}

	w := b.do(http.MethodPost, authpages.PathResendVerification, b.form(page), nil)
	if w.Code != http.StatusOK || id.resent != 1 {
		t.Fatalf("POST resend = %d, resent %d", w.Code, id.resent)
	}
	if got := text(screenData(t, w), "notice"); got != "We sent a new link to alice@example.com." {
		t.Errorf("notice = %q", got)
	}

	id.resendErr = &kernel.RateLimitedError{RetryAfter: time.Minute}
	w = b.do(http.MethodPost, authpages.PathResendVerification, b.form(page), nil)
	if w.Code != http.StatusTooManyRequests || text(screenData(t, w), "error") != "You asked for the link too many times. Try again in 1 minute." {
		t.Errorf("rate-limited resend = %d %s", w.Code, w.Body.String())
	}

	delete(b.cookies, authpages.CookieSession)
	if w := b.do(http.MethodPost, authpages.PathResendVerification, b.form(page), nil); w.Code != http.StatusSeeOther {
		t.Errorf("resend without a session = %d, want a redirect to sign in", w.Code)
	}
}
