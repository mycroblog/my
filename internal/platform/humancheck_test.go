// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

// siteverifyStub is the CAPTCHA service behind the App's HTTPClient. A token "pass-<action>"
// is valid for that action on example.test, "down" finds the service unreachable, and every
// other token, the empty one included, is invalid.
type siteverifyStub struct {
	mu     sync.Mutex
	tokens []string
}

func (s *siteverifyStub) Do(_ context.Context, r *kernel.HTTPRequest) (*kernel.HTTPResponse, error) {
	form, err := url.ParseQuery(string(r.Body))
	if err != nil {
		return nil, err
	}
	token := form.Get("response")
	s.mu.Lock()
	s.tokens = append(s.tokens, token)
	s.mu.Unlock()
	body := `{"success":false,"error-codes":["invalid-input-response"]}`
	switch {
	case token == "down":
		return nil, errors.New("connection refused")
	case strings.HasPrefix(token, "pass-"):
		body = `{"success":true,"hostname":"example.test","action":"` + strings.TrimPrefix(token, "pass-") + `"}`
	}
	return &kernel.HTTPResponse{StatusCode: http.StatusOK, Header: http.Header{}, URL: r.URL, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func (s *siteverifyStub) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tokens)
}

// TestHumanCheck drives sign-in and registration with captcha = turnstile against a stub of
// the service (ADR-0106): both check the token, a valid invite does not skip the check, an
// unreachable service lets sign-ins through and refuses sign-ups, and a failed check counts
// against the client's address only.
// cspDirective returns the sources of one directive of a Content-Security-Policy, nil when it is absent.
func cspDirective(csp, name string) []string {
	for directive := range strings.SplitSeq(csp, ";") {
		fields := strings.Fields(directive)
		if len(fields) > 0 && fields[0] == name {
			return fields[1:]
		}
	}
	return nil
}

func TestHumanCheck(t *testing.T) {
	t.Parallel()
	stub := &siteverifyStub{}
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true),
		"MY_CAPTCHA":      "turnstile", "MY_CAPTCHA_SITE_KEY": "site-key", "MY_CAPTCHA_SECRET_KEY": "secret-key",
	})
	s := startApp(t, cfg, platform.AppOptions{HTTPClient: stub}, platform.ServeOptions{}, func(a *platform.App) {
		_, err := a.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
			Username: "alice", Email: "alice@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("CreateAdmin: %v", err)
		}
	})
	ctx := t.Context()
	const field = "cf-turnstile-response"

	t.Run("sign-in", func(t *testing.T) {
		c := &authClient{t: t, s: s, cookies: map[string]string{}}
		resp, body := c.do(http.MethodGet, authpages.PathSignIn, nil, nil)
		// The page also admits this site's scripts when the embedded web build has the passkey script, which depends
		// on whether the web client was built before the test.
		if csp := resp.Header.Get("Content-Security-Policy"); !slices.Contains(cspDirective(csp, "script-src"), "https://challenges.cloudflare.com") ||
			!slices.Equal(cspDirective(csp, "frame-src"), []string{"https://challenges.cloudflare.com"}) || strings.Contains(csp, "unsafe-") {
			t.Errorf("Content-Security-Policy = %q", csp)
		}
		if !strings.Contains(body, `<div class="cf-turnstile" data-sitekey="site-key" data-action="sign_in" data-language="en" data-theme="auto"></div>`) {
			t.Errorf("sign-in page lacks the widget:\n%s", body)
		}
		signIn := func(token string) int {
			t.Helper()
			fields := []string{"email", "alice@example.com", "password", "correct horse battery staple"}
			if token != "" {
				fields = append(fields, field, token)
			}
			resp, _ := c.do(http.MethodPost, authpages.PathSignIn, c.form(fields...), nil)
			return resp.StatusCode
		}
		for _, token := range []string{"", "invalid", "pass-sign_up"} {
			if got := signIn(token); got != http.StatusUnprocessableEntity {
				t.Errorf("sign-in with the token %q = %d, want 422", token, got)
			}
		}
		if c.cookies[authpages.CookieSession] != "" {
			t.Fatal("a failed check started a session")
		}
		// An unreachable service lets the sign-in through, with a warning (fail open).
		if got := signIn("down"); got != http.StatusSeeOther || c.cookies[authpages.CookieSession] == "" {
			t.Fatalf("sign-in while the service is down = %d", got)
		}
		if logs := s.logs.String(); !strings.Contains(logs, "the CAPTCHA service could not check a sign-in") {
			t.Errorf("no warning in the log:\n%s", logs)
		}
		if got := signIn("pass-sign_in"); got != http.StatusSeeOther {
			t.Errorf("sign-in with a valid token = %d, want 303", got)
		}
	})

	t.Run("sign-up", func(t *testing.T) {
		if _, err := s.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{
			Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationInvite),
		}); err != nil {
			t.Fatal(err)
		}
		inv, err := s.app.Identity.CreateInvite(ctx, identity.CreateInviteInput{})
		if err != nil {
			t.Fatal(err)
		}
		c := &authClient{t: t, s: s, cookies: map[string]string{}}
		resp, body := c.do(http.MethodGet, authpages.PathSignUp, nil, nil)
		if !slices.Contains(cspDirective(resp.Header.Get("Content-Security-Policy"), "script-src"), "https://challenges.cloudflare.com") ||
			!strings.Contains(body, `data-action="sign_up"`) {
			t.Errorf("sign-up page lacks the widget: %v\n%s", resp.Header, body)
		}
		signUp := func(token string) (int, string) {
			t.Helper()
			fields := []string{"username", "bob", "email", "bob@example.com", "password", "correct horse battery", "invite_code", inv.Code}
			if token != "" {
				fields = append(fields, field, token)
			}
			resp, body := c.do(http.MethodPost, authpages.PathSignUp, c.formAt(authpages.PathSignUp, fields...), nil)
			return resp.StatusCode, body
		}
		// A valid invite does not skip the check.
		for _, token := range []string{"", "invalid", "pass-sign_in"} {
			if got, body := signUp(token); got != http.StatusUnprocessableEntity || !strings.Contains(body, "Complete the check that you are not a bot") {
				t.Errorf("sign-up with the token %q = %d, want 422\n%s", token, got, body)
			}
		}
		// An unreachable service refuses the sign-up (fail closed).
		if got, body := signUp("down"); got != http.StatusServiceUnavailable || !strings.Contains(body, "Try again later.") {
			t.Errorf("sign-up while the service is down = %d, want 503\n%s", got, body)
		}
		if c.cookies[authpages.CookieSession] != "" {
			t.Fatal("a failed check created an account")
		}
		if got, _ := signUp("pass-sign_up"); got != http.StatusSeeOther || c.cookies[authpages.CookieSession] == "" {
			t.Errorf("sign-up with a valid token = %d, want 303", got)
		}
	})

	t.Run("counting", func(t *testing.T) {
		signIn := func(ip, token string) error {
			_, err := s.app.Identity.SignIn(ctx, identity.SignInInput{
				Email: "alice@example.com", Password: "correct horse battery staple", IP: ip, HumanCheckToken: token,
			})
			return err
		}
		// Failed checks from many addresses do not lock the account.
		for i := range identity.LoginAccountLimit.Max + 1 {
			if err := signIn("192.0.2."+strconv.FormatInt(i+1, 10), "invalid"); !errors.Is(err, kernel.ErrHumanCheckFailed) {
				t.Fatalf("sign-in #%d with an invalid token = %v", i+1, err)
			}
		}
		if err := signIn("198.51.100.1", "pass-sign_in"); err != nil {
			t.Fatalf("sign-in after failed checks from other addresses = %v", err)
		}
		// They count against their address like a wrong password.
		for range identity.LoginIPLimit.Max - 1 {
			if err := signIn("192.0.2.1", "invalid"); !errors.Is(err, kernel.ErrHumanCheckFailed) {
				t.Fatalf("sign-in with an invalid token = %v", err)
			}
		}
		var limited *kernel.RateLimitedError
		if err := signIn("192.0.2.1", "pass-sign_in"); !errors.As(err, &limited) {
			t.Errorf("sign-in after %d failed checks = %v, want rate limited", identity.LoginIPLimit.Max, err)
		}
		// A sign-up with a failed check counts as an attempt.
		before := stub.count()
		for i := range identity.SignUpIPLimit.Max {
			_, err := s.app.Identity.SignUp(ctx, identity.SignUpInput{
				Username: "carol" + strconv.FormatInt(i, 10), Email: "carol" + strconv.FormatInt(i, 10) + "@example.com", Password: "correct horse battery",
				InviteCode: "unused", IP: "203.0.113.1", HumanCheckToken: "invalid",
			})
			if !errors.Is(err, kernel.ErrHumanCheckFailed) {
				t.Fatalf("sign-up #%d with an invalid token = %v", i+1, err)
			}
		}
		_, err := s.app.Identity.SignUp(ctx, identity.SignUpInput{
			Username: "carol", Email: "carol@example.com", Password: "correct horse battery",
			InviteCode: "unused", IP: "203.0.113.1", HumanCheckToken: "pass-sign_up",
		})
		if !errors.As(err, &limited) {
			t.Errorf("sign-up after %d failed checks = %v, want rate limited", identity.SignUpIPLimit.Max, err)
		}
		// The rate limit comes first: the limited attempt reached no service.
		if got := int64(stub.count() - before); got != identity.SignUpIPLimit.Max {
			t.Errorf("the service got %d tokens, want %d", got, identity.SignUpIPLimit.Max)
		}
	})
}
