// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"html"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	domainidentity "github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
)

var loggedLink = regexp.MustCompile(`https://example\.test(/auth/verify_email\?token=[A-Za-z0-9_%-]+)`)

// TestSignUpScreens drives registration in each mode on the real database, with mail = none:
// verification emails go to the log, and a moderator's approval stands in for verification.
func TestSignUpScreens(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	ctx := t.Context()
	setMode := func(mode instance.RegistrationMode) {
		t.Helper()
		_, err := s.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: instance.SettingRegistrationMode, Value: string(mode)})
		if err != nil {
			t.Fatal(err)
		}
	}
	signUp := func(fields ...string) (*authClient, *http.Response) {
		t.Helper()
		c := &authClient{t: t, s: s, cookies: map[string]string{}}
		resp, _ := c.do(http.MethodPost, authpages.PathSignUp, c.formAt(authpages.PathSignUp, fields...), nil)
		return c, resp
	}
	status := func(c *authClient) string {
		t.Helper()
		_, body := c.do(http.MethodGet, authpages.PathSignIn, nil, nil)
		return body
	}
	userID := func(email string) string {
		t.Helper()
		reg, err := s.app.Identity.ListRegistrations(ctx, domainidentity.RegistrationPending, 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range reg {
			if r.Email == email {
				return r.Application.UserID
			}
		}
		t.Fatalf("no pending registration of %s in %+v", email, reg)
		return ""
	}

	// closed: the page says so and the form is refused.
	setMode(instance.RegistrationClosed)
	c := &authClient{t: t, s: s, cookies: map[string]string{}}
	if resp, body := c.do(http.MethodGet, authpages.PathSignUp, nil, nil); resp.StatusCode != http.StatusForbidden ||
		!strings.Contains(body, "not accepting new accounts") {
		t.Fatalf("closed GET sign_up = %d\n%s", resp.StatusCode, body)
	}

	// open: without email delivery, the new user waits for a moderator (FR-002).
	setMode(instance.RegistrationOpen)
	alice, resp := signUp("username", "alice", "email", "alice@example.com", "password", "correct horse battery")
	if resp.StatusCode != http.StatusSeeOther || alice.cookies[authpages.CookieSession] == "" {
		t.Fatalf("open sign-up = %d", resp.StatusCode)
	}
	if body := status(alice); !strings.Contains(body, "waiting for a moderator") || strings.Contains(body, "verify_email/resend") {
		t.Errorf("open sign-up status page:\n%s", body)
	}

	// The verification email went to the log from its job, and only there: the access log and
	// the job's log have no token.
	logs := s.logs.String()
	m := loggedLink.FindStringSubmatch(logs)
	for deadline := time.Now().Add(10 * time.Second); m == nil && time.Now().Before(deadline); m = loggedLink.FindStringSubmatch(logs) {
		time.Sleep(20 * time.Millisecond)
		logs = s.logs.String()
	}
	if m == nil || !strings.Contains(logs, "mail not sent (mail = none)") {
		t.Fatalf("no verification link in the log:\n%s", logs)
	}
	link := m[1]
	token := strings.TrimPrefix(link, "/auth/verify_email?token=")
	if strings.Count(logs, token) != 1 {
		t.Errorf("the token appears %d times in the log", strings.Count(logs, token))
	}
	resp, body := alice.do(http.MethodGet, link, nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `name="token" value="`+html.EscapeString(token)+`"`) {
		t.Fatalf("GET %s = %d\n%s", link, resp.StatusCode, body)
	}
	if resp, body := alice.do(http.MethodPost, authpages.PathVerifyEmail, alice.formAt(link, "token", token), nil); resp.StatusCode != http.StatusOK ||
		!strings.Contains(body, "Your email address is confirmed.") {
		t.Fatalf("POST verify_email = %d\n%s", resp.StatusCode, body)
	}
	if resp, _ := alice.do(http.MethodPost, authpages.PathVerifyEmail, alice.formAt(link, "token", token), nil); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("second POST verify_email = %d, want 422", resp.StatusCode)
	}
	if strings.Count(s.logs.String(), token) != 1 {
		t.Error("confirming the email logged the token")
	}

	// approval: the reason reaches the moderator, whose approval lets the user in.
	setMode(instance.RegistrationApproval)
	bob, resp := signUp("username", "bob", "email", "bob@example.com", "password", "correct horse battery", "reason", "Friend of alice")
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("approval sign-up = %d", resp.StatusCode)
	}
	if body := status(bob); !strings.Contains(body, "waiting for a moderator") {
		t.Errorf("approval sign-up status page:\n%s", body)
	}
	pending, err := s.app.Identity.ListRegistrations(ctx, domainidentity.RegistrationPending, 10)
	if err != nil || len(pending) != 2 || pending[1].Username != "bob" || pending[1].Application.Reason != "Friend of alice" {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	_, err = s.app.Identity.DecideRegistration(ctx, identity.DecideInput{UserID: userID("bob@example.com"), Decision: domainidentity.DecisionApproved})
	if err != nil {
		t.Fatal(err)
	}
	if body := status(bob); !strings.Contains(body, "You are signed in") {
		t.Errorf("approved user's page:\n%s", body)
	}

	// invite: a valid invite admits the user at once.
	setMode(instance.RegistrationInvite)
	_, resp = signUp("username", "carol", "email", "carol@example.com", "password", "correct horse battery", "invite_code", "WRONGCODE")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("sign-up with a wrong invite = %d, want 422", resp.StatusCode)
	}
	inv, err := s.app.Identity.CreateInvite(ctx, identity.CreateInviteInput{MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	carol, resp := signUp("username", "carol", "email", "carol@example.com", "password", "correct horse battery", "invite_code", inv.Code)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("invite sign-up = %d", resp.StatusCode)
	}
	if body := status(carol); !strings.Contains(body, "You are signed in") {
		t.Errorf("invited user's page:\n%s", body)
	}

	// The fifth attempt from this address within the window is the last one allowed.
	_, resp = signUp("username", "dave", "email", "dave@example.com", "password", "correct horse battery", "invite_code", inv.Code)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("sign-up with a used-up invite = %d, want 422", resp.StatusCode)
	}
	_, resp = signUp("username", "erin", "email", "erin@example.com", "password", "correct horse battery", "invite_code", inv.Code)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("sixth sign-up = %d, want 429", resp.StatusCode)
	}
}

// TestSignUpTakesTheBrowserLocale checks on the real database that a new account's locale is
// the supported locale that best matches its registration's Accept-Language, and the current
// default_locale setting when none does (FR-007).
func TestSignUpTakesTheBrowserLocale(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	s := start(t, cfg, platform.ServeOptions{}, nil)
	ctx := t.Context()
	for key, value := range map[instance.SettingKey]string{
		instance.SettingRegistrationMode: string(instance.RegistrationOpen),
		instance.SettingDefaultLocale:    "ja",
	} {
		if _, err := s.app.Instance.SetSetting(ctx, appinstance.SetSettingInput{Key: key, Value: value}); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct{ username, acceptLanguage, want string }{
		{"alice", "ja-JP,ja;q=0.9,en;q=0.8", "ja"},
		{"bob", "en-US,en;q=0.9", "en"},
		{"carol", "fr-FR,de;q=0.5", "ja"},
		{"dave", "", "ja"},
	}
	for _, tt := range tests {
		c := &authClient{t: t, s: s, cookies: map[string]string{}}
		form := c.formAt(authpages.PathSignUp, "username", tt.username, "email", tt.username+"@example.com", "password", "correct horse battery")
		if resp, body := c.do(http.MethodPost, authpages.PathSignUp, form, map[string]string{"Accept-Language": tt.acceptLanguage}); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("%s: sign-up = %d\n%s", tt.username, resp.StatusCode, body)
		}
	}
	pending, err := s.app.Identity.ListRegistrations(ctx, domainidentity.RegistrationPending, 10)
	if err != nil || len(pending) != len(tests) {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	for _, tt := range tests {
		i := slices.IndexFunc(pending, func(r identity.RegistrationEntry) bool { return r.Username == tt.username })
		if i < 0 {
			t.Fatalf("no registration of %s in %+v", tt.username, pending)
		}
		if got, err := s.app.Identity.UserLocale(ctx, pending[i].Application.UserID); err != nil || got != tt.want {
			t.Errorf("Accept-Language %q: locale = %q, %v, want %q", tt.acceptLanguage, got, err, tt.want)
		}
	}
}
