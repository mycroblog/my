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

func wantContains(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s:\n%s", want, body)
		}
	}
}

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
		mode         identity.RegistrationMode
		status       int
		want, unwant []string
	}{
		// Every mode but closed takes an invite (ADR-0108 §4): optional in open and approval mode,
		// required in invite mode, and filled in from ?invite=.
		{identity.RegistrationOpen, http.StatusOK, []string{
			`name="username"`, `name="email"`, `name="password"`,
			`name="invite_code" type="text" value="ABCD" autocomplete="off" autocapitalize="characters" spellcheck="false" aria-describedby`,
			"You can also sign up without one.",
		}, []string{`name="reason"`}},
		{identity.RegistrationApproval, http.StatusOK, []string{
			`<textarea id="sign-up-reason" name="reason"`, `maxlength="1000" dir="auto"`,
			`name="invite_code" type="text" value="ABCD" autocomplete="off" autocapitalize="characters" spellcheck="false" aria-describedby`,
			"without waiting for review",
		}, nil},
		{identity.RegistrationInvite, http.StatusOK, []string{
			`id="sign-up-invite" name="invite_code" type="text" value="ABCD" autocomplete="off" autocapitalize="characters" spellcheck="false" required`,
			"This server accepts new accounts by invitation only.",
		}, []string{`name="reason"`}},
		{identity.RegistrationClosed, http.StatusForbidden, []string{"This server is not accepting new accounts."}, []string{"<form", `name="username"`}},
	} {
		id.mode = tc.mode
		w := b.do(http.MethodGet, authpages.PathSignUp+"?invite=ABCD", nil, nil)
		if w.Code != tc.status {
			t.Errorf("%s: GET sign_up = %d, want %d", tc.mode, w.Code, tc.status)
		}
		wantContains(t, w.Body.String(), tc.want...)
		wantLacks(t, w.Body.String(), tc.unwant...)
	}
}

func TestSignUpPageIsAccessibleHTML(t *testing.T) {
	id := newFakeIdentity()
	id.mode = identity.RegistrationApproval
	b := newBrowser(t, newServer(t, id))
	w := b.do(http.MethodGet, authpages.PathSignUp, nil, map[string]string{"Accept-Language": "ja"})
	h := w.Header()
	if h.Get("Cache-Control") != "no-store" || !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("headers = %v", h)
	}
	body := w.Body.String()
	wantContains(t, body,
		`<html lang="ja" dir="ltr">`,
		`<title>アカウントを作成</title>`,
		`<label for="sign-up-username">ユーザー名</label>`,
		`aria-describedby="sign-up-username-hint"`,
		`<p id="sign-up-username-hint" class="hint">半角英字 (a–z)・数字・アンダースコアで 30 文字まで。あとから変更できません。</p>`,
		`<label for="sign-up-email">メールアドレス</label>`,
		`autocomplete="username"`,
		`<label for="sign-up-password">パスワード</label>`,
		`autocomplete="new-password" required minlength="8" maxlength="128"`,
		`<label for="sign-up-reason">参加したい理由 (任意)</label>`,
		`<form class="form" method="post" action="/auth/sign_up">`,
		`<a href="/auth/sign_in">ログイン</a>`,
	)
	wantLacks(t, body, "<script", "style=", "aria-invalid", `role="alert"`)
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
	wantContains(t, body,
		"<h1>Your account is not ready yet</h1>",
		"Your registration is waiting for a moderator&#39;s approval.",
		"Confirm your email address: open the link we sent to alice@example.com.",
		`<form class="form" method="post" action="/auth/verify_email/resend">`,
		`<form class="form" method="post" action="/auth/sign_out">`,
	)
	wantLacks(t, body, "Continue")

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
		{"fr-FR, de;q=0.5", ""},
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
	body := w.Body.String()
	wantContains(t, body,
		`<p id="sign-up-error" class="alert" role="alert">The account was not created. Check the fields marked below.</p>`,
		`<p id="sign-up-username-error" class="field-error">Use 1 to 30 letters (a–z), digits, and underscores.</p>`,
		`aria-describedby="sign-up-username-hint sign-up-username-error" aria-invalid="true"`,
		`<p id="sign-up-email-error" class="field-error">Enter an email address such as alice@example.com.</p>`,
		`aria-describedby="sign-up-email-hint sign-up-email-error" aria-invalid="true"`,
		`<p id="sign-up-password-error" class="field-error">Use 8 to 128 characters.</p>`,
		`aria-describedby="sign-up-password-hint sign-up-password-error" aria-invalid="true"`,
		`<p id="sign-up-invite-error" class="field-error">Enter your invite code.</p>`,
		`value="al ice"`, `value="alice"`,
	)
	wantLacks(t, body, `value="short"`)
	if len(id.signUps) != 0 {
		t.Error("an invalid form reached SignUp")
	}
}

func TestSignUpReportsWhatTheServiceRejects(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		want   string
	}{
		{appaccount.ErrUsernameTaken, http.StatusUnprocessableEntity, `<p id="sign-up-username-error" class="field-error">This username is already taken.</p>`},
		{appidentity.ErrEmailTaken, http.StatusUnprocessableEntity, `<p id="sign-up-email-error" class="field-error">This email address is already in use.</p>`},
		{appidentity.ErrInvalidInvite, http.StatusUnprocessableEntity, `<p id="sign-up-invite-error" class="field-error">This invite code is not valid.`},
		{appidentity.ErrRegistrationClosed, http.StatusForbidden, "This server is not accepting new accounts."},
		{appidentity.ErrSignUpRefused, http.StatusForbidden, `<p id="sign-up-error" class="alert" role="alert">This server cannot accept this sign-up.</p>`},
		{&kernel.RateLimitedError{RetryAfter: 90 * time.Second}, http.StatusTooManyRequests,
			`<p id="sign-up-error" class="alert" role="alert">Too many sign-up attempts from your network. Try again in 2 minutes.</p>`},
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
		wantContains(t, w.Body.String(), tc.want)
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
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "This form has expired.") {
		t.Errorf("POST sign_up with a wrong token = %d", w.Code)
	}
	if len(id.signUps) != 0 {
		t.Error("a refused post reached SignUp")
	}
}

func TestSignInLinksToSignUpUnlessClosed(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	wantContains(t, b.do(http.MethodGet, authpages.PathSignIn, nil, nil).Body.String(), `<a href="/auth/sign_up">Create an account</a>`)
	id.mode = identity.RegistrationClosed
	wantLacks(t, b.do(http.MethodGet, authpages.PathSignIn, nil, nil).Body.String(), `/auth/sign_up`)
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
	body := b.do(http.MethodGet, authpages.PathSignIn, nil, nil).Body.String()
	wantContains(t, body, "Your registration was not approved")
	wantLacks(t, body, "verify_email/resend", "waiting for a moderator")
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
	wantContains(t, page.Body.String(), `<input type="hidden" name="token" value="my_evt_good">`, `<button class="button" type="submit">Confirm email address</button>`)
	if id.verified["my_evt_good"] {
		t.Fatal("GET redeemed the token")
	}
	w := b.do(http.MethodPost, authpages.PathVerifyEmail, b.form(page, "token", "my_evt_good"), nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Your email address is confirmed.") {
		t.Fatalf("POST verify_email = %d %s", w.Code, w.Body.String())
	}
	// Continuing signs in to the app, which opens its welcome step.
	wantContains(t, w.Body.String(), `<a href="/login?return_to=%2Fwelcome">`)
	w = b.do(http.MethodPost, authpages.PathVerifyEmail, b.form(page, "token", "my_evt_good"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "This link is not valid.") {
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
	wantContains(t, page.Body.String(), "open the link we sent to alice@example.com")
	wantLacks(t, page.Body.String(), "waiting for a moderator")

	w := b.do(http.MethodPost, authpages.PathResendVerification, b.form(page), nil)
	if w.Code != http.StatusOK || id.resent != 1 {
		t.Fatalf("POST resend = %d, resent %d", w.Code, id.resent)
	}
	wantContains(t, w.Body.String(), `<p class="notice" role="status">We sent a new link to alice@example.com.</p>`)

	id.resendErr = &kernel.RateLimitedError{RetryAfter: time.Minute}
	w = b.do(http.MethodPost, authpages.PathResendVerification, b.form(page), nil)
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), `<p class="alert" role="alert">You asked for the link too many times. Try again in 1 minute.</p>`) {
		t.Errorf("rate-limited resend = %d %s", w.Code, w.Body.String())
	}

	delete(b.cookies, authpages.CookieSession)
	if w := b.do(http.MethodPost, authpages.PathResendVerification, b.form(page), nil); w.Code != http.StatusSeeOther {
		t.Errorf("resend without a session = %d, want a redirect to sign in", w.Code)
	}
}
