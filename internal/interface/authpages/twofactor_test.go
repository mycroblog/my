// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// bob@example.com has two-factor authentication: "correct horse" gets bobChallenge, which
// CompleteSignIn takes once with the code "123456".
const bobChallenge = "my_2fa_bob"

func (f *fakeIdentity) CompleteSignIn(_ context.Context, in appidentity.SecondFactorInput) (appidentity.SignedIn, error) {
	f.second = append(f.second, in)
	if f.completeErr != nil {
		return appidentity.SignedIn{}, f.completeErr
	}
	switch {
	case f.limited:
		return appidentity.SignedIn{}, &kernel.RateLimitedError{RetryAfter: 61 * time.Second}
	case in.Challenge != bobChallenge || f.challengeUsed:
		return appidentity.SignedIn{}, appidentity.ErrSecondFactorExpired
	case in.Code != "123456":
		return appidentity.SignedIn{}, appidentity.ErrInvalidSecondFactor
	}
	f.challengeUsed = true
	delete(f.live, in.CurrentToken)
	token := f.newToken(userBob)
	return appidentity.SignedIn{Token: token, Session: identity.Session{UserID: userBob, ExpiresAt: time.Now().Add(time.Hour)}}, nil
}

func TestTwoFactorSignIn(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page,
		"email", "bob@example.com", "password", "correct horse", "return_to", "/home"), nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignInTwoFactor+"?return_to=%2Fhome" {
		t.Fatalf("POST sign_in = %d → %q, want 303 to the code form", w.Code, w.Header().Get("Location"))
	}
	if _, ok := b.cookies[authpages.CookieSession]; ok {
		t.Fatal("the password alone started a session")
	}
	var challenge *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == authpages.CookieTwoFactor {
			challenge = c
		}
	}
	if challenge == nil || challenge.Value != bobChallenge || !challenge.Secure || !challenge.HttpOnly || challenge.Path != "/" ||
		challenge.MaxAge != int(appidentity.SecondFactorChallengeTTL.Seconds()) {
		t.Fatalf("challenge cookie = %+v", challenge)
	}

	page = b.do(http.MethodGet, w.Header().Get("Location"), nil, map[string]string{"Accept-Language": "ja"})
	body := page.Body.String()
	for _, want := range []string{
		`<title>二要素認証</title>`,
		`<label for="two-factor-code">認証コード</label>`,
		`autocomplete="one-time-code"`,
		`<form class="form" method="post" action="/auth/sign_in/two_factor">`,
		`name="return_to" value="/home"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("code form lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, bobChallenge) {
		t.Error("the page shows the challenge, which stays in the HttpOnly cookie")
	}

	w = b.do(http.MethodPost, authpages.PathSignInTwoFactor, b.form(page, "code", "000000", "return_to", "/home"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `aria-invalid="true"`) ||
		!strings.Contains(w.Body.String(), "The code is incorrect") {
		t.Errorf("wrong code = %d:\n%s", w.Code, w.Body.String())
	}
	w = b.do(http.MethodPost, authpages.PathSignInTwoFactor, b.form(page, "code", " ", "return_to", "/home"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Enter the code.") {
		t.Errorf("empty code = %d", w.Code)
	}
	w = b.do(http.MethodPost, authpages.PathSignInTwoFactor, b.form(page, "code", "123456", "return_to", "/home"),
		map[string]string{"User-Agent": "test-agent"})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/home" {
		t.Fatalf("right code = %d → %q, want 303 to return_to", w.Code, w.Header().Get("Location"))
	}
	if _, ok := b.cookies[authpages.CookieSession]; !ok {
		t.Error("the code started no session")
	}
	if _, ok := b.cookies[authpages.CookieTwoFactor]; ok {
		t.Error("the challenge cookie outlived the sign-in")
	}
	last := id.second[len(id.second)-1]
	if last.Challenge != bobChallenge || last.IP != "192.0.2.1" || last.UserAgent != "test-agent" {
		t.Errorf("CompleteSignIn input = %+v", last)
	}

	// Without a pending sign-in, the code form sends the browser back to the sign-in form.
	w = b.do(http.MethodGet, authpages.PathSignInTwoFactor+"?return_to=%2Fhome", nil, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != authpages.PathSignIn+"?return_to=%2Fhome" {
		t.Errorf("GET code form without a challenge = %d → %q", w.Code, w.Header().Get("Location"))
	}
}

func TestTwoFactorSignInExpiresAndRateLimits(t *testing.T) {
	id := newFakeIdentity()
	b := newBrowser(t, newServer(t, id))
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", "bob@example.com", "password", "correct horse"), nil)
	page = b.do(http.MethodGet, authpages.PathSignInTwoFactor, nil, nil)

	id.limited = true
	w := b.do(http.MethodPost, authpages.PathSignInTwoFactor, b.form(page, "code", "123456"), nil)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "61" {
		t.Errorf("rate limited = %d (Retry-After %q)", w.Code, w.Header().Get("Retry-After"))
	}
	id.limited = false
	id.challengeUsed = true
	w = b.do(http.MethodPost, authpages.PathSignInTwoFactor, b.form(page, "code", "123456"), nil)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `action="/auth/sign_in"`) ||
		!strings.Contains(w.Body.String(), "Sign in again") {
		t.Errorf("expired challenge = %d, want the sign-in form:\n%s", w.Code, w.Body.String())
	}
	if _, ok := b.cookies[authpages.CookieTwoFactor]; ok {
		t.Error("an expired challenge stays in its cookie")
	}
	if _, ok := b.cookies[authpages.CookieSession]; ok {
		t.Error("an expired challenge started a session")
	}
}
