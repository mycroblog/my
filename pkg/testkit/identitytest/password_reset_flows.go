// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/pkg/testkit"
)

// The password reset and password change cases of AccountRepositoryContract (FR-002).

var resetLink = regexp.MustCompile(`https://` + regexp.QuoteMeta(testkit.ContractHost) + regexp.QuoteMeta(appidentity.PathResetPassword) + `\?token=(\S+)`)

// withMailer replaces the env's identity service by one whose mailer m delivers email.
func (e *AccountEnv) withMailer(m *testkit.Mailer) {
	e.Identity = e.newIdentityWith(func(d *appidentity.Deps) { d.Mailer, d.EmailDelivery = m, true })
}

// resetToken returns the token of the newest reset email to address, failing when the newest
// email to it is not one.
func resetToken(t *testing.T, m *testkit.Mailer, address string) string {
	t.Helper()
	sent := m.Sent()
	for i := len(sent) - 1; i >= 0; i-- {
		if sent[i].To != address {
			continue
		}
		match := resetLink.FindStringSubmatch(sent[i].Text)
		if match == nil {
			t.Fatalf("the newest email to %s has no reset link:\n%s", address, sent[i].Text)
		}
		token, err := url.QueryUnescape(match[1])
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	t.Fatalf("no email to %s among %d", address, len(sent))
	return ""
}

// requestReset asks for a reset link for sessionEmail from no known address.
func (e *AccountEnv) requestReset(t *testing.T, email string) {
	t.Helper()
	if err := e.Identity.RequestPasswordReset(t.Context(), appidentity.PasswordResetRequest{Email: email}); err != nil {
		t.Fatalf("RequestPasswordReset(%s) = %v", email, err)
	}
}

// passwordResetWorksOnce resets a password with an emailed link: the answer does not tell
// whether the address has an account, the link works once and only with a valid password, and
// the new password ends every session and grant but keeps the passkeys and the two-factor
// authentication.
func passwordResetWorksOnce(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	m := &testkit.Mailer{}
	e.withMailer(m)
	reg := e.SessionUser(t)
	const address = "Alice@Example.com" // the stored address of the session user
	e.RegisterPasskey(t, reg.User.ID, "phone")
	u := e.EnableTOTP(t, reg)
	tokens, _ := e.WebTokensFor(t, reg.User.ID)
	_, err := e.Identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword})
	if !errors.Is(err, appidentity.ErrSecondFactorRequired) {
		t.Fatalf("SignIn with TOTP = %v, want ErrSecondFactorRequired", err)
	}

	// An unknown address and a known one answer alike; only the known one gets an email.
	e.requestReset(t, "nobody@example.com")
	if n := len(m.Sent()); n != 0 {
		t.Fatalf("emails after a request for an unknown address = %d, want 0", n)
	}
	if err := e.Identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: "not an address"}); !errors.Is(err, identity.ErrInvalidEmail) {
		t.Errorf("RequestPasswordReset(not an address) = %v, want ErrInvalidEmail", err)
	}
	e.requestReset(t, "ALICE@example.com")
	token := resetToken(t, m, address)
	if !strings.HasPrefix(token, appidentity.PasswordResetTokenPrefix) {
		t.Errorf("token = %q, want the prefix %s", token, appidentity.PasswordResetTokenPrefix)
	}
	stored, err := e.H.Identity.FindUserTokenByHash(ctx, appidentity.HashToken(token))
	if err != nil || stored.Purpose != identity.PurposePasswordReset || stored.UserID != reg.User.ID ||
		!stored.ExpiresAt.Equal(e.Clock.Now().Add(appidentity.PasswordResetTTL)) {
		t.Errorf("stored token = %+v, %v; want a reset token of the user for %v", stored, err, appidentity.PasswordResetTTL)
	}
	for _, bad := range []string{"", appidentity.PasswordResetTokenPrefix + "unknown", strings.TrimPrefix(token, appidentity.PasswordResetTokenPrefix), token + "x"} {
		if err := e.Identity.CheckPasswordResetToken(ctx, bad); !errors.Is(err, appidentity.ErrInvalidToken) {
			t.Errorf("CheckPasswordResetToken(%q) = %v, want ErrInvalidToken", bad, err)
		}
	}
	if err := e.Identity.CheckPasswordResetToken(ctx, token); err != nil {
		t.Fatalf("CheckPasswordResetToken = %v", err)
	}
	// An invalid password leaves the link working.
	if _, err := e.Identity.ResetPassword(ctx, token, "short"); !errors.Is(err, identity.ErrInvalidPassword) {
		t.Errorf("ResetPassword(short) = %v, want ErrInvalidPassword", err)
	}

	// A sign-in waiting for its second factor, and a code authorized in a browser session.
	pending := e.signInFirstStep(t)
	signedIn, err := e.completeSignIn(e.signInFirstStep(t), u.code(e))
	if err != nil {
		t.Fatalf("CompleteSignIn = %v", err)
	}
	sessionCode := e.CodeInSession(t, WebAuthorizationRequest(), signedIn.Session)

	// Concurrent resets with one token: exactly one wins.
	e.Clock.Advance(time.Second)
	const tries = 5
	var wg sync.WaitGroup
	errs := make([]error, tries)
	for i := range tries {
		wg.Go(func() { _, errs[i] = e.Identity.ResetPassword(ctx, token, "a brand new password") })
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, appidentity.ErrInvalidToken):
			t.Errorf("ResetPassword = %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d of %d concurrent resets succeeded, want 1", ok, tries)
	}
	if err := e.Identity.CheckPasswordResetToken(ctx, token); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("CheckPasswordResetToken after the reset = %v, want ErrInvalidToken", err)
	}

	// The grant ended, the old password no longer works, the new one does, and the passkey and
	// the two-factor authentication stay.
	e.WantAccess(t, tokens.AccessToken, false, "a token from before the reset")
	_, err = e.Identity.ExchangeAuthorizationCode(ctx, appidentity.CodeExchange{
		ClientCredentials: WebCreds, Code: sessionCode, RedirectURI: ContractWebRedirect, CodeVerifier: ContractCodeVerifier,
	})
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "exchanging a code of a session that the reset ended")
	if _, err := e.completeSignIn(pending, u.code(e)); !errors.Is(err, appidentity.ErrSecondFactorExpired) {
		t.Errorf("CompleteSignIn(a challenge from before the reset) = %v, want ErrSecondFactorExpired", err)
	}
	if _, err := e.Identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword}); !errors.Is(err, appidentity.ErrInvalidCredentials) {
		t.Errorf("SignIn with the old password = %v, want ErrInvalidCredentials", err)
	}
	if _, err := e.Identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: "a brand new password"}); !errors.Is(err, appidentity.ErrSecondFactorRequired) {
		t.Errorf("SignIn with the new password = %v, want ErrSecondFactorRequired (TOTP stays)", err)
	}
	if ps, err := e.Identity.Passkeys(ctx, reg.User.ID); err != nil || len(ps) != 1 {
		t.Errorf("Passkeys after the reset = %+v, %v; want the passkey to stay", ps, err)
	}
	if st, err := e.Identity.TwoFactorStatus(ctx, reg.User.ID); err != nil || !st.Enabled {
		t.Errorf("TwoFactorStatus after the reset = %+v, %v; want enabled", st, err)
	}
	sent := m.Sent()
	if last := sent[len(sent)-1]; last.To != address || resetLink.MatchString(last.Text) ||
		!strings.Contains(last.Text, "https://"+testkit.ContractHost+appidentity.PathForgotPassword) {
		t.Errorf("last email = %+v, want the notice of the change to %s", last, address)
	}

	// A link expires, and a newer password makes an earlier link stop working.
	e.requestReset(t, sessionEmail)
	expiring := resetToken(t, m, address)
	e.Clock.Advance(appidentity.PasswordResetTTL)
	if err := e.Identity.CheckPasswordResetToken(ctx, expiring); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("CheckPasswordResetToken(expired) = %v, want ErrInvalidToken", err)
	}
	e.requestReset(t, sessionEmail)
	older := resetToken(t, m, address)
	e.Clock.Advance(time.Second)
	if err := e.Identity.ChangePassword(ctx, appidentity.ChangePasswordInput{
		UserID: reg.User.ID, Confirmation: appidentity.Confirmation{Password: "a brand new password"}, NewPassword: "yet another password",
	}); err != nil {
		t.Fatalf("ChangePassword = %v", err)
	}
	if _, err := e.Identity.ResetPassword(ctx, older, "a third password"); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("ResetPassword with a link older than the password = %v, want ErrInvalidToken", err)
	}
}

// passwordResetLimitsRequests limits the reset emails per address, silently, and the requests
// per client address; without email delivery, nobody can ask for a link.
func passwordResetLimitsRequests(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	m := &testkit.Mailer{}
	e.withMailer(m)
	e.SessionUser(t)
	for range appidentity.PasswordResetEmailLimit.Max + 1 {
		e.requestReset(t, sessionEmail)
	}
	if n := int64(len(m.Sent())); n != appidentity.PasswordResetEmailLimit.Max {
		t.Errorf("emails = %d, want %d: the requests beyond the limit send nothing", n, appidentity.PasswordResetEmailLimit.Max)
	}
	for i := range appidentity.PasswordResetIPLimit.Max {
		if err := e.Identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: "nobody@example.com", IP: "192.0.2.9"}); err != nil {
			t.Fatalf("request #%d = %v", i+1, err)
		}
	}
	err := e.Identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: "nobody@example.com", IP: "192.0.2.9"})
	var limited *kernel.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Errorf("request beyond the limit of the client address = %v, want a RateLimitedError", err)
	}

	e.Identity = e.newIdentityWith(func(d *appidentity.Deps) { d.Mailer = m })
	if e.Identity.PasswordResetByEmail() {
		t.Error("PasswordResetByEmail without email delivery = true")
	}
	if err := e.Identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: sessionEmail}); !errors.Is(err, appidentity.ErrPasswordResetByEmailUnavailable) {
		t.Errorf("RequestPasswordReset without email delivery = %v, want ErrPasswordResetByEmailUnavailable", err)
	}
}

// passwordChangeKeepsTheCurrentGrant changes a password with the current one: it ends every
// session and every grant but the one that made the change.
func passwordChangeKeepsTheCurrentGrant(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	signed := e.SignIn(t, appidentity.SignInInput{})
	current, _ := e.WebTokensFor(t, reg.User.ID)
	other, _ := e.WebTokensFor(t, reg.User.ID)
	bob := e.Register(t, "bob")
	bobTokens, _ := e.WebTokensFor(t, bob.User.ID)
	change := func(currentPassword, newPassword string) error {
		return e.Identity.ChangePassword(ctx, appidentity.ChangePasswordInput{
			UserID: reg.User.ID, Confirmation: appidentity.Confirmation{Password: currentPassword}, NewPassword: newPassword, KeepFamilyID: current.Token.FamilyID,
		})
	}
	if err := change(sessionPassword, "short"); !errors.Is(err, identity.ErrInvalidPassword) {
		t.Errorf("ChangePassword(short) = %v, want ErrInvalidPassword", err)
	}
	if err := change("wrong password", "a brand new password"); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Errorf("ChangePassword(wrong) = %v, want ErrWrongPassword", err)
	}
	e.WantAccess(t, other.AccessToken, true, "a token before the change")
	e.Clock.Advance(time.Second)
	if err := change(sessionPassword, "a brand new password"); err != nil {
		t.Fatalf("ChangePassword = %v", err)
	}
	e.wantInvalidSession(t, signed.Token, "a session from before the change")
	e.WantAccess(t, current.AccessToken, true, "the token that made the change")
	e.WantAccess(t, other.AccessToken, false, "another grant of the user")
	e.WantAccess(t, bobTokens.AccessToken, true, "another user's token")
	if _, err := e.Identity.RefreshWebClient(ctx, current.RefreshToken); err != nil {
		t.Errorf("RefreshWebClient with the grant that made the change = %v", err)
	}
	e.SignIn(t, appidentity.SignInInput{Email: sessionEmail, Password: "a brand new password"})

	// Wrong current passwords are limited per user.
	for range appidentity.PasswordChangeLimit.Max - 1 {
		if err := change("wrong password", "another password"); !errors.Is(err, appidentity.ErrWrongPassword) {
			t.Fatalf("ChangePassword(wrong) = %v", err)
		}
	}
	var limited *kernel.RateLimitedError
	if err := change("a brand new password", "another password"); !errors.As(err, &limited) {
		t.Errorf("ChangePassword beyond the limit = %v, want a RateLimitedError", err)
	}
}
