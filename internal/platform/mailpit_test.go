// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	mailsmtp "github.com/mycroblog/my/internal/adapter/mail/smtp"
	"github.com/mycroblog/my/internal/app/identity"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/authpages"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit/mailpittest"
)

var (
	verifyLink = regexp.MustCompile(`https://example\.test(/auth/verify_email\?token=([A-Za-z0-9_%-]+))`)
	resetLink  = regexp.MustCompile(`https://example\.test(/auth/reset_password\?token=([A-Za-z0-9_%-]+))`)
)

// TestEmailLinksThroughMailpit drives registration and a password reset with mail = smtp on a
// Mailpit server (pkg/testkit/mailpittest), which runs only with TEST_MAIL=container
// (scripts/task.sh go:test-mail; CI in the slow suite): the jobs send the emails over SMTP, and
// the links that Mailpit received confirm the address and set a new password.
func TestEmailLinksThroughMailpit(t *testing.T) {
	mp := mailpittest.New(t, mailpittest.Options{STARTTLS: true, Username: "mailer", Password: "s3cret"})
	mailer, err := mailsmtp.New(mailsmtp.Options{
		Host: mp.Host, Port: mp.Port, Username: "mailer", Password: "s3cret", From: "noreply@example.test", TLSConfig: mp.TLSConfig,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_MAIL": "smtp", "MY_MAIL_FROM": "noreply@example.test",
		"MY_MAIL_SMTP_HOST": mp.Host, "MY_MAIL_SMTP_PORT": strconv.Itoa(mp.Port), "MY_MAIL_SMTP_USERNAME": "mailer", "MY_MAIL_SMTP_PASSWORD": "s3cret",
	})
	s := startApp(t, cfg, platform.AppOptions{Mailer: mailer}, platform.ServeOptions{}, nil)
	if _, err := s.app.Instance.SetSetting(t.Context(), appinstance.SetSettingInput{
		Key: instance.SettingRegistrationMode, Value: string(instance.RegistrationOpen),
	}); err != nil {
		t.Fatal(err)
	}
	const email, password, newPassword = "alice@example.com", "correct horse battery", "battery staple correct horse"
	newClient := func() *authClient { return &authClient{t: t, s: s, cookies: map[string]string{}} }
	signedIn := func(c *authClient) bool {
		_, body := c.do(http.MethodGet, authpages.PathSignIn, nil, nil)
		return strings.Contains(body, "You are signed in")
	}
	// link returns the path and the token of the link that re finds in the text of m, and
	// checks that the HTML version carries the same link.
	link := func(m mailpittest.Message, re *regexp.Regexp) (string, string) {
		t.Helper()
		found := re.FindStringSubmatch(m.Text)
		if found == nil {
			t.Fatalf("no link in the email %q:\n%s", m.Subject, m.Text)
		}
		if !strings.Contains(m.HTML, `href="`+html.EscapeString(found[0])+`"`) {
			t.Errorf("the HTML version of %q has no link to %s:\n%s", m.Subject, found[0], m.HTML)
		}
		if m.From.Address != "noreply@example.test" || len(m.To) != 1 || m.To[0].Address != email {
			t.Errorf("the email %q is from %+v to %+v", m.Subject, m.From, m.To)
		}
		return found[1], found[2]
	}

	// Sign up, and confirm the address with the link of the verification email.
	alice := newClient()
	if resp, _ := alice.do(http.MethodPost, authpages.PathSignUp, alice.formAt(authpages.PathSignUp,
		"username", "alice", "email", email, "password", password), nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-up = %d", resp.StatusCode)
	}
	if signedIn(alice) {
		t.Fatal("the new user is in before confirming the address")
	}
	path, token := link(mp.WaitFor(t, email, 1, 30*time.Second), verifyLink)
	if resp, body := alice.do(http.MethodGet, path, nil, nil); resp.StatusCode != http.StatusOK ||
		!strings.Contains(body, `name="token" value="`+html.EscapeString(token)+`"`) {
		t.Fatalf("GET %s = %d\n%s", path, resp.StatusCode, body)
	}
	if resp, body := alice.do(http.MethodPost, authpages.PathVerifyEmail, alice.formAt(path, "token", token), nil); resp.StatusCode != http.StatusOK ||
		!strings.Contains(body, "Your email address is confirmed.") {
		t.Fatalf("POST verify_email = %d\n%s", resp.StatusCode, body)
	}

	// Sign in on another browser.
	signIn := func(password string) (*authClient, int) {
		t.Helper()
		c := newClient()
		resp, _ := c.do(http.MethodPost, authpages.PathSignIn, c.form("email", email, "password", password), nil)
		return c, resp.StatusCode
	}
	if c, status := signIn(password); status != http.StatusSeeOther || !signedIn(c) {
		t.Fatalf("sign-in after confirming the address = %d", status)
	}

	// Ask for a password reset link by email, and set a new password with it.
	forgot := newClient()
	if resp, body := forgot.do(http.MethodPost, authpages.PathForgotPassword, forgot.formAt(authpages.PathForgotPassword, "email", email), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST forgot_password = %d\n%s", resp.StatusCode, body)
	}
	path, token = link(mp.WaitFor(t, email, 2, 30*time.Second), resetLink)
	if resp, body := forgot.do(http.MethodGet, path, nil, nil); resp.StatusCode != http.StatusOK ||
		!strings.Contains(body, `name="token" value="`+html.EscapeString(token)+`"`) {
		t.Fatalf("GET %s = %d\n%s", path, resp.StatusCode, body)
	}
	if resp, body := forgot.do(http.MethodPost, authpages.PathResetPassword, forgot.formAt(path, "token", token, "password", newPassword), nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST reset_password = %d\n%s", resp.StatusCode, body)
	}
	if _, status := signIn(password); status == http.StatusSeeOther {
		t.Error("the old password still signs in")
	}
	if c, status := signIn(newPassword); status != http.StatusSeeOther || !signedIn(c) {
		t.Errorf("sign-in with the new password = %d", status)
	}
}

// TestDeactivationEmailThroughMailpit deactivates an account through the client API with
// mail = smtp on a Mailpit server: the job sends the one email over SMTP, whose text and HTML state
// the date of the deletion and link to the sign-in page that undoes it.
func TestDeactivationEmailThroughMailpit(t *testing.T) {
	mp := mailpittest.New(t, mailpittest.Options{STARTTLS: true, Username: "mailer", Password: "s3cret"})
	mailer, err := mailsmtp.New(mailsmtp.Options{
		Host: mp.Host, Port: mp.Port, Username: "mailer", Password: "s3cret", From: "noreply@example.test", TLSConfig: mp.TLSConfig,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_MAIL": "smtp", "MY_MAIL_FROM": "noreply@example.test",
		"MY_MAIL_SMTP_HOST": mp.Host, "MY_MAIL_SMTP_PORT": strconv.Itoa(mp.Port), "MY_MAIL_SMTP_USERNAME": "mailer", "MY_MAIL_SMTP_PASSWORD": "s3cret",
	})
	s := startApp(t, cfg, platform.AppOptions{Mailer: mailer}, platform.ServeOptions{}, nil)
	ctx := t.Context()
	const email, password = "alice@example.com", "correct horse battery"
	if _, err := s.app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{Username: "root", Email: "root@example.com", Password: password}); err != nil {
		t.Fatal(err)
	}
	alice, err := s.app.Identity.CreateAdmin(ctx, identity.CreateAdminInput{Username: "alice", Email: email, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url+"/api/v1/accounts/deactivation", strings.NewReader(`{"password":"`+password+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+issueTokens(t, s.app, alice.User.ID, "").AccessToken)
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var d clientapi.Deactivation
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /accounts/deactivation = %d, %v", resp.StatusCode, err)
	}

	m := mp.WaitFor(t, email, 1, 30*time.Second)
	if m.From.Address != "noreply@example.test" || len(m.To) != 1 || m.To[0].Address != email {
		t.Errorf("the email is from %+v to %+v", m.From, m.To)
	}
	date := identity.DeletionDateText(d.DeletionDueAt, "")
	link := "https://example.test" + authpages.PathSignIn
	if !strings.Contains(m.Text, date) || !strings.Contains(m.Text, link) {
		t.Errorf("the text does not state %q or link to %s:\n%s", date, link, m.Text)
	}
	if !strings.Contains(html.UnescapeString(m.HTML), date) || !strings.Contains(m.HTML, `href="`+html.EscapeString(link)+`"`) {
		t.Errorf("the HTML does not state %q or link to %s:\n%s", date, link, m.HTML)
	}
	if n := len(mp.Messages(t)); n != 1 {
		t.Errorf("%d emails in Mailpit, want the 1", n)
	}
}
