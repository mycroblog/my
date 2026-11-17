// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"fmt"
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
	if at(screenData(t, w), "canResetPassword") != nil {
		t.Error("the sign-in screen links to the password reset without PasswordReset")
	}
	if w := without.do(http.MethodGet, authpages.PathForgotPassword, nil, nil); w.Code != http.StatusNotFound {
		t.Errorf("GET forgot_password without PasswordReset = %d, want 404", w.Code)
	}

	b := newBrowser(t, newResetServer(t, newFakeIdentity(), &fakePasswordReset{byEmail: true}, nil))
	w = b.do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "en"})
	if at(screenData(t, w), "canResetPassword") != true {
		t.Error("the sign-in screen does not link to the password reset")
	}
}

func TestForgotPassword(t *testing.T) {
	reset := &fakePasswordReset{byEmail: true}
	b := newBrowser(t, newResetServer(t, newFakeIdentity(), reset, nil))
	en := map[string]string{"Accept-Language": "en"}
	page := b.do(http.MethodGet, authpages.PathForgotPassword, nil, en)
	if page.Code != http.StatusOK {
		t.Fatalf("GET forgot_password = %d", page.Code)
	}
	if data := screenData(t, page); text(data, "screen") != "forgot_password" || text(data, "csrfToken") == "" ||
		at(data, "forgotPassword.unavailable") != nil || at(data, "forgotPassword.sent") != nil {
		t.Errorf("GET forgot_password data = %v, want the form", data)
	}

	// A request without the anti-forgery token is refused before it is made.
	if w := b.do(http.MethodPost, authpages.PathForgotPassword, url.Values{"email": {"alice@example.com"}}, en); w.Code != http.StatusForbidden || len(reset.requests) != 0 {
		t.Errorf("POST without csrf_token = %d, %d requests; want 403 and none", w.Code, len(reset.requests))
	}
	w := b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", "not an address"), en)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("POST an invalid address = %d, want 422", w.Code)
	}
	if data := screenData(t, w); !strings.Contains(text(data, "error"), "Enter an email address.") || text(data, "email") != "not an address" {
		t.Errorf("POST an invalid address: %v", data)
	}

	w = b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", " alice@example.com "), en)
	if w.Code != http.StatusOK {
		t.Fatalf("POST forgot_password = %d", w.Code)
	}
	data := screenData(t, w)
	if notice := text(data, "notice"); !strings.Contains(notice, "If an account uses alice@example.com, a link to set a new password is on its way.") ||
		!strings.Contains(notice, "60 minutes") {
		t.Errorf("notice = %q", notice)
	}
	if at(data, "forgotPassword.sent") != true || at(data, "csrfToken") != nil {
		t.Errorf("the screen after the request offers the form again: %v", data)
	}
	if got := reset.requests[len(reset.requests)-1]; got.Email != "alice@example.com" || got.IP == "" {
		t.Errorf("request = %+v, want the trimmed address and the client address", got)
	}

	reset.err = &kernel.RateLimitedError{RetryAfter: 90 * time.Second}
	w = b.do(http.MethodPost, authpages.PathForgotPassword, b.form(page, "email", "alice@example.com"), en)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "90" {
		t.Errorf("rate-limited = %d Retry-After %q, want 429 after 90", w.Code, w.Header().Get("Retry-After"))
	}
	if !strings.Contains(text(screenData(t, w), "error"), "Try again in 2 minutes.") {
		t.Errorf("rate-limited error = %q", text(screenData(t, w), "error"))
	}
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
	if data := screenData(t, w); at(data, "forgotPassword.unavailable") != true || at(data, "csrfToken") != nil || at(data, "humanCheck") != nil {
		t.Errorf("forgot_password without email: %v, want the note and no form", data)
	}
}

func TestForgotPasswordHumanCheck(t *testing.T) {
	reset := &fakePasswordReset{byEmail: true}
	b := newBrowser(t, newResetServer(t, newFakeIdentity(), reset, &turnstile))
	page := b.do(http.MethodGet, authpages.PathForgotPassword, nil, nil)
	if data := screenData(t, page); text(data, "humanCheck.siteKey") != "turnstile-site-key" || text(data, "humanCheck.action") != "password_reset" {
		t.Errorf("humanCheck = %v", at(data, "humanCheck"))
	}
	if src := directive(page.Header().Get("Content-Security-Policy"), "script-src"); fmt.Sprint(src) != "['self' https://challenges.cloudflare.com]" {
		t.Errorf("script-src = %v, want this site and the service", src)
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
		if got := text(screenData(t, w), "error"); !strings.Contains(got, "This link is not valid.") {
			t.Errorf("GET %s error = %q", target, got)
		}
	}
	page := b.do(http.MethodGet, authpages.PathResetPassword+"?token=my_prt_good", nil, en)
	if page.Code != http.StatusOK {
		t.Fatalf("GET reset_password = %d", page.Code)
	}
	if data := screenData(t, page); text(data, "screen") != "reset_password" || text(data, "token") != "my_prt_good" ||
		at(data, "newPassword.min") != 8.0 || at(data, "newPassword.max") != 128.0 || text(data, "newPassword.hint") == "" {
		t.Errorf("GET reset_password data = %v", data)
	}
	if reset.used {
		t.Fatal("GET used the token")
	}

	w := b.do(http.MethodPost, authpages.PathResetPassword, b.form(page, "token", "my_prt_good", "password", "short"), en)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("POST a short password = %d, want 422", w.Code)
	}
	if data := screenData(t, w); text(data, "newPassword.error") == "" || text(data, "token") != "my_prt_good" {
		t.Errorf("POST a short password data = %v, want the field's error and the token", data)
	}

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
	if got := text(screenData(t, w), "screen"); got != "password_reset" {
		t.Errorf("screen after the reset = %q, want password_reset", got)
	}
	if _, ok := b.cookies[authpages.CookieSession]; ok {
		t.Error("the session cookie stays after the reset")
	}
	w = b.do(http.MethodPost, authpages.PathResetPassword, b.form(page, "token", "my_prt_good", "password", "a new password"), en)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(text(screenData(t, w), "error"), "This link is not valid.") {
		t.Errorf("POST a used token = %d, want 422 and the invalid link", w.Code)
	}
}
