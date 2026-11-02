// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// fakePasswordReset knows the reset token "my_prt_good", which works once, and fails requests
// with err.
type fakePasswordReset struct {
	byEmail  bool
	err      error
	requests []appidentity.PasswordResetRequest
	used     bool
	resets   []string // new passwords
}

func (f *fakePasswordReset) PasswordResetByEmail() bool { return f.byEmail }

func (f *fakePasswordReset) RequestPasswordReset(_ context.Context, in appidentity.PasswordResetRequest) error {
	f.requests = append(f.requests, in)
	if !f.byEmail {
		return appidentity.ErrPasswordResetByEmailUnavailable
	}
	if f.err != nil {
		return f.err
	}
	return identity.ValidateEmail(in.Email)
}

func (f *fakePasswordReset) CheckPasswordResetToken(_ context.Context, token string) error {
	if token != "my_prt_good" || f.used {
		return appidentity.ErrInvalidToken
	}
	return nil
}

func (f *fakePasswordReset) ResetPassword(ctx context.Context, token, password string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if err := f.CheckPasswordResetToken(ctx, token); err != nil {
		return "", err
	}
	if err := identity.ValidatePassword(password); err != nil {
		return "", err
	}
	f.used = true
	f.resets = append(f.resets, password)
	return userAlice, nil
}

func newResetServer(t *testing.T, id authpages.Identity, reset authpages.PasswordReset, w *kernel.HumanCheckWidget) http.Handler {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	authpages.New(authpages.Options{Identity: id, Locales: &i18n.Resolver{Bundle: b}, PasswordReset: reset, HumanCheck: w}).Register(m)
	return m
}

func TestSignInLinksToPasswordReset(t *testing.T) {
	without := newBrowser(t, newServer(t, newFakeIdentity()))
	w := without.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	wantLacks(t, w.Body.String(), `href="/auth/forgot_password"`)
	if w := without.do(http.MethodGet, authpages.PathForgotPassword, nil, nil); w.Code != http.StatusNotFound {
		t.Errorf("GET forgot_password without PasswordReset = %d, want 404", w.Code)
	}

	b := newBrowser(t, newResetServer(t, newFakeIdentity(), &fakePasswordReset{byEmail: true}, nil))
	w = b.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "en"})
	wantContains(t, w.Body.String(), `<a href="/auth/forgot_password">Forgot your password?</a>`)
}

func TestForgotPassword(t *testing.T) {
	reset := &fakePasswordReset{byEmail: true}
	b := newBrowser(t, newResetServer(t, newFakeIdentity(), reset, nil))
	en := map[string]string{"Accept-Language": "en"}
	page := b.do(http.MethodGet, authpages.PathForgotPassword, nil, en)
	if page.Code != http.StatusOK {
		t.Fatalf("GET forgot_password = %d", page.Code)
	}
	wantContains(t, page.Body.String(),
		`<form class="form" method="post" action="/auth/forgot_password">`,
		`<label for="forgot-password-email">Email address</label>`,
		`autocomplete="username"`,
	)

	// A request without the anti-forgery token is refused before it is made.
	if w := b.do(http.MethodPost, authpages.PathForgotPassword, url.Values{"email": {"alice@example.com"}}, en); w.Code != http.StatusForbidden || len(reset.requests) != 0 {
		t.Errorf("POST without csrf_token = %d, %d requests; want 403 and none", w.Code, len(reset.requests))
	}
	w := b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", "not an address"), en)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("POST an invalid address = %d, want 422", w.Code)
	}
	wantContains(t, w.Body.String(), "Enter an email address.", `aria-invalid="true"`, `value="not an address"`)

	w = b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", " alice@example.com "), en)
	if w.Code != http.StatusOK {
		t.Fatalf("POST forgot_password = %d", w.Code)
	}
	body := w.Body.String()
	wantContains(t, body, `role="status"`, "If an account uses alice@example.com, a link to set a new password is on its way.", "60 minutes")
	wantLacks(t, body, "<form")
	if got := reset.requests[len(reset.requests)-1]; got.Email != "alice@example.com" || got.IP == "" {
		t.Errorf("request = %+v, want the trimmed address and the client address", got)
	}

	reset.err = &kernel.RateLimitedError{RetryAfter: 90 * time.Second}
	w = b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", "alice@example.com"), en)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "90" {
		t.Errorf("rate-limited = %d Retry-After %q, want 429 after 90", w.Code, w.Header().Get("Retry-After"))
	}
	wantContains(t, w.Body.String(), "Try again in 2 minutes.")
	reset.err = kernel.ErrHumanCheckUnavailable
	if w := b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", "alice@example.com"), en); w.Code != http.StatusServiceUnavailable {
		t.Errorf("CAPTCHA unavailable = %d, want 503 (fail closed)", w.Code)
	}
}

func TestForgotPasswordWithoutEmail(t *testing.T) {
	b := newBrowser(t, newResetServer(t, newFakeIdentity(), &fakePasswordReset{}, &turnstile))
	w := b.do(http.MethodGet, authpages.PathForgotPassword, nil, map[string]string{"Accept-Language": "en"})
	if w.Code != http.StatusOK {
		t.Fatalf("GET forgot_password = %d", w.Code)
	}
	wantContains(t, w.Body.String(), "This server does not send email", `href="/auth/sign_in"`)
	wantLacks(t, w.Body.String(), "<form", "data-sitekey", "<script")
}

func TestForgotPasswordHumanCheck(t *testing.T) {
	reset := &fakePasswordReset{byEmail: true}
	b := newBrowser(t, newResetServer(t, newFakeIdentity(), reset, &turnstile))
	page := b.do(http.MethodGet, authpages.PathForgotPassword, nil, nil)
	wantContains(t, page.Body.String(), `data-sitekey="turnstile-site-key"`, `data-action="password_reset"`)
	if src := directive(page.Header().Get("Content-Security-Policy"), "script-src"); len(src) != 1 || src[0] != "https://challenges.cloudflare.com" {
		t.Errorf("script-src = %v, want the service only", src)
	}
	b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", "alice@example.com", "cf-turnstile-response", "tok"), nil)
	if len(reset.requests) != 1 || reset.requests[0].HumanCheckToken != "tok" {
		t.Errorf("requests = %+v, want the widget's token", reset.requests)
	}
}

func TestResetPassword(t *testing.T) {
	reset := &fakePasswordReset{byEmail: true}
	id := newFakeIdentity()
	b := newBrowser(t, newResetServer(t, id, reset, nil))
	en := map[string]string{"Accept-Language": "en"}
	for _, target := range []string{authpages.PathResetPassword, authpages.PathResetPassword + "?token=my_prt_bad"} {
		w := b.do(http.MethodGet, target, nil, en)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", target, w.Code)
		}
		wantContains(t, w.Body.String(), "This link is not valid.")
	}
	page := b.do(http.MethodGet, authpages.PathResetPassword+"?token=my_prt_good", nil, en)
	if page.Code != http.StatusOK {
		t.Fatalf("GET reset_password = %d", page.Code)
	}
	wantContains(t, page.Body.String(),
		`<form class="form" method="post" action="/auth/reset_password">`,
		`<input type="hidden" name="token" value="my_prt_good">`,
		`<label for="reset-password-password">New password</label>`,
		`autocomplete="new-password"`, `minlength="8"`, `maxlength="128"`,
	)
	if reset.used {
		t.Fatal("GET used the token")
	}

	w := b.do(http.MethodPost, authpages.PathResetPassword, b.form(page, "token", "my_prt_good", "password", "short"), en)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("POST a short password = %d, want 422", w.Code)
	}
	wantContains(t, w.Body.String(), `id="reset-password-password-error"`, `aria-invalid="true"`, `value="my_prt_good"`)

	// A signed-in browser loses its session, which the new password ended.
	signIn := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	b.do(http.MethodPost, authpages.PathSignIn, b.form(signIn, "email", "alice@example.com", "password", "correct horse"), nil)
	if b.cookies[authpages.CookieSession] == "" {
		t.Fatal("sign-in set no session cookie")
	}
	w = b.do(http.MethodPost, authpages.PathResetPassword, b.form(page, "token", "my_prt_good", "password", "a new password"), en)
	if w.Code != http.StatusOK || len(reset.resets) != 1 || reset.resets[0] != "a new password" {
		t.Fatalf("POST reset_password = %d, resets %v", w.Code, reset.resets)
	}
	wantContains(t, w.Body.String(), "Your password is changed", `href="/auth/sign_in"`)
	if _, ok := b.cookies[authpages.CookieSession]; ok {
		t.Error("the session cookie stays after the reset")
	}
	w = b.do(http.MethodPost, authpages.PathResetPassword, b.form(page, "token", "my_prt_good", "password", "a new password"), en)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "This link is not valid.") {
		t.Errorf("POST a used token = %d, want 422 and the invalid link", w.Code)
	}
}
