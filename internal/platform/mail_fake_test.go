// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform_test

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestE2EFakesMailWarns starts the test build with mail = fake and profile = development, as the
// demonstration does (ADR-0015 §6): the start warns that this is a test build.
func TestE2EFakesMailWarns(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_PROFILE": "development", "MY_MAIL": "fake", "MY_MAIL_FROM": "noreply@example.test",
	})
	logs := &syncBuffer{}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.NewTextHandler(logs, nil))})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !strings.Contains(logs.String(), "level=WARN") || !strings.Contains(logs.String(), "this is a test build") {
		t.Errorf("the start logged no warning of the test build:\n%s", logs)
	}
}

// TestE2EFakesMailCountsAsDelivering drives registration and a password reset with mail = fake:
// the fake counts as a mailer that delivers, so an open-mode sign-up waits for the user's
// confirmation of the address and not for a moderator, and the password reset is offered; each link
// is in the log, in the line of the message.
func TestE2EFakesMailCountsAsDelivering(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_PROFILE": "development", "MY_MAIL": "fake", "MY_MAIL_FROM": "noreply@example.test",
	})
	s := start(t, cfg, platform.ServeOptions{}, func(a *platform.App) {
		_, err := a.Identity.CreateAdmin(t.Context(), identity.CreateAdminInput{
			Username: "bob", Email: "bob@example.com", Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("CreateAdmin: %v", err)
		}
	})
	if _, err := s.app.Instance.SetSetting(t.Context(), appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationOpen),
	}); err != nil {
		t.Fatal(err)
	}
	newClient := func() *authClient { return &authClient{t: t, s: s, cookies: map[string]string{}} }
	// logged returns the path and the token of the link of re in the log line of a mail to address.
	logged := func(re *regexp.Regexp, address string) (string, string) {
		t.Helper()
		var m []string
		testkit.Eventually(t, 10*time.Second, func() bool {
			for _, line := range strings.Split(s.logs.String(), "\n") {
				if strings.Contains(line, "mail not sent (mail = fake)") && strings.Contains(line, address) {
					if m = re.FindStringSubmatch(line); m != nil {
						return true
					}
				}
			}
			return false
		})
		return m[1], m[2]
	}

	// Sign up in the open mode: the address is to be confirmed, and no moderator is waited for.
	alice := newClient()
	if resp, _ := alice.do(http.MethodPost, authpages.PathSignUp, alice.formAt(authpages.PathSignUp,
		"username", "alice", "email", "alice@example.com", "password", "correct horse battery"), nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("open sign-up = %d", resp.StatusCode)
	}
	_, body := alice.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	if data := authData(t, body); authAt(data, "status.pending") == true || authAt(data, "status.verifyEmail") == nil {
		t.Errorf("open sign-up status page: %v, want the confirmation of the address and no waiting for a moderator", data)
	}
	path, token := logged(verifyLink, "alice@example.com")
	if resp, body := alice.do(http.MethodPost, authpages.PathVerifyEmail, alice.formAt(path, "token", token), nil); resp.StatusCode != http.StatusOK ||
		!strings.Contains(body, `"screen":"verified"`) {
		t.Fatalf("POST verify_email = %d\n%s", resp.StatusCode, body)
	}
	if _, body := alice.do(http.MethodGet, authpages.PathSignIn, nil, nil); authText(authData(t, body), "screen") != "signed_in" {
		t.Errorf("after confirming the address: %s", body)
	}

	// Reset the password by mail.
	forgot := newClient()
	if resp, body := forgot.do(http.MethodPost, authpages.PathForgotPassword, forgot.formAt(authpages.PathForgotPassword, "email", "bob@example.com"), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST forgot_password = %d\n%s", resp.StatusCode, body)
	}
	path, token = logged(resetLink, "bob@example.com")
	if resp, body := forgot.do(http.MethodPost, authpages.PathResetPassword, forgot.formAt(path, "token", token, "password", "battery staple correct horse"), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST reset_password = %d\n%s", resp.StatusCode, body)
	}
	in := newClient()
	if resp, _ := in.do(http.MethodPost, authpages.PathSignIn, in.form("email", "bob@example.com", "password", "battery staple correct horse"), nil); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("sign-in with the new password = %d, want 303", resp.StatusCode)
	}
}
