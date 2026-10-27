// SPDX-License-Identifier: AGPL-3.0-only

package testkit

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
)

// The password reset and password change cases of AccountRepositoryContract (FR-002).

var resetLink = regexp.MustCompile(`https://` + regexp.QuoteMeta(contractHost) + regexp.QuoteMeta(appidentity.PathResetPassword) + `\?token=(\S+)`)

// withMailer replaces the env's identity service by one whose mailer m delivers email.
func (e *accountEnv) withMailer(m *Mailer) {
	e.identity = e.newIdentityWith(func(d *appidentity.Deps) { d.Mailer, d.EmailDelivery = m, true })
}

// resetToken returns the token of the newest reset email to address, failing when the newest
// email to it is not one.
func resetToken(t *testing.T, m *Mailer, address string) string {
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
func (e *accountEnv) requestReset(t *testing.T, email string) {
	t.Helper()
	if err := e.identity.RequestPasswordReset(t.Context(), appidentity.PasswordResetRequest{Email: email}); err != nil {
		t.Fatalf("RequestPasswordReset(%s) = %v", email, err)
	}
}

// passwordResetWorksOnce resets a password with an emailed link: the answer does not tell
// whether the address has an account, the link works once and only with a valid password, and
// the new password ends every session and grant but keeps the passkeys and the two-factor
// authentication.
func passwordResetWorksOnce(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := &Mailer{}
	e.withMailer(m)
	reg := e.sessionUser(t)
	const address = "Alice@Example.com" // the stored address of the session user
	e.registerPasskey(t, reg.User.ID, "phone")
	u := e.enableTOTP(t, reg)
	tokens, _ := e.webTokensFor(t, reg.User.ID)
	_, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword})
	if !errors.Is(err, appidentity.ErrSecondFactorRequired) {
		t.Fatalf("SignIn with TOTP = %v, want ErrSecondFactorRequired", err)
	}

	// An unknown address and a known one answer alike; only the known one gets an email.
	e.requestReset(t, "nobody@example.com")
	if n := len(m.Sent()); n != 0 {
		t.Fatalf("emails after a request for an unknown address = %d, want 0", n)
	}
	if err := e.identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: "not an address"}); !errors.Is(err, identity.ErrInvalidEmail) {
		t.Errorf("RequestPasswordReset(not an address) = %v, want ErrInvalidEmail", err)
	}
	e.requestReset(t, "ALICE@example.com")
	token := resetToken(t, m, address)
	if !strings.HasPrefix(token, appidentity.PasswordResetTokenPrefix) {
		t.Errorf("token = %q, want the prefix %s", token, appidentity.PasswordResetTokenPrefix)
	}
	stored, err := e.h.Identity.FindUserTokenByHash(ctx, appidentity.HashToken(token))
	if err != nil || stored.Purpose != identity.PurposePasswordReset || stored.UserID != reg.User.ID ||
		!stored.ExpiresAt.Equal(e.clock.Now().Add(appidentity.PasswordResetTTL)) {
		t.Errorf("stored token = %+v, %v; want a reset token of the user for %v", stored, err, appidentity.PasswordResetTTL)
	}
	for _, bad := range []string{"", appidentity.PasswordResetTokenPrefix + "unknown", strings.TrimPrefix(token, appidentity.PasswordResetTokenPrefix), token + "x"} {
		if err := e.identity.CheckPasswordResetToken(ctx, bad); !errors.Is(err, appidentity.ErrInvalidToken) {
			t.Errorf("CheckPasswordResetToken(%q) = %v, want ErrInvalidToken", bad, err)
		}
	}
	if err := e.identity.CheckPasswordResetToken(ctx, token); err != nil {
		t.Fatalf("CheckPasswordResetToken = %v", err)
	}
	// An invalid password leaves the link working.
	if _, err := e.identity.ResetPassword(ctx, token, "short"); !errors.Is(err, identity.ErrInvalidPassword) {
		t.Errorf("ResetPassword(short) = %v, want ErrInvalidPassword", err)
	}

	// A sign-in waiting for its second factor, and a code authorized in a browser session.
	pending := e.signInFirstStep(t)
	signedIn, err := e.completeSignIn(e.signInFirstStep(t), u.code(e))
	if err != nil {
		t.Fatalf("CompleteSignIn = %v", err)
	}
	sessionCode := e.codeInSession(t, webAuthorizationRequest(), signedIn.Session)

	// Concurrent resets with one token: exactly one wins.
	e.clock.Advance(time.Second)
	const tries = 5
	var wg sync.WaitGroup
	errs := make([]error, tries)
	for i := range tries {
		wg.Go(func() { _, errs[i] = e.identity.ResetPassword(ctx, token, "a brand new password") })
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
	if err := e.identity.CheckPasswordResetToken(ctx, token); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("CheckPasswordResetToken after the reset = %v, want ErrInvalidToken", err)
	}

	// The grant ended, the old password no longer works, the new one does, and the passkey and
	// the two-factor authentication stay.
	e.wantAccess(t, tokens.AccessToken, false, "a token from before the reset")
	_, err = e.identity.ExchangeAuthorizationCode(ctx, appidentity.CodeExchange{
		ClientCredentials: webCreds, Code: sessionCode, RedirectURI: contractWebRedirect, CodeVerifier: contractCodeVerifier,
	})
	wantOAuthError(t, err, appidentity.OAuthInvalidGrant, "exchanging a code of a session that the reset ended")
	if _, err := e.completeSignIn(pending, u.code(e)); !errors.Is(err, appidentity.ErrSecondFactorExpired) {
		t.Errorf("CompleteSignIn(a challenge from before the reset) = %v, want ErrSecondFactorExpired", err)
	}
	if _, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword}); !errors.Is(err, appidentity.ErrInvalidCredentials) {
		t.Errorf("SignIn with the old password = %v, want ErrInvalidCredentials", err)
	}
	if _, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: "a brand new password"}); !errors.Is(err, appidentity.ErrSecondFactorRequired) {
		t.Errorf("SignIn with the new password = %v, want ErrSecondFactorRequired (TOTP stays)", err)
	}
	if ps, err := e.identity.Passkeys(ctx, reg.User.ID); err != nil || len(ps) != 1 {
		t.Errorf("Passkeys after the reset = %+v, %v; want the passkey to stay", ps, err)
	}
	if st, err := e.identity.TwoFactorStatus(ctx, reg.User.ID); err != nil || !st.Enabled {
		t.Errorf("TwoFactorStatus after the reset = %+v, %v; want enabled", st, err)
	}
	sent := m.Sent()
	if last := sent[len(sent)-1]; last.To != address || resetLink.MatchString(last.Text) ||
		!strings.Contains(last.Text, "https://"+contractHost+appidentity.PathForgotPassword) {
		t.Errorf("last email = %+v, want the notice of the change to %s", last, address)
	}

	// A link expires, and a newer password makes an earlier link stop working.
	e.requestReset(t, sessionEmail)
	expiring := resetToken(t, m, address)
	e.clock.Advance(appidentity.PasswordResetTTL)
	if err := e.identity.CheckPasswordResetToken(ctx, expiring); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("CheckPasswordResetToken(expired) = %v, want ErrInvalidToken", err)
	}
	e.requestReset(t, sessionEmail)
	older := resetToken(t, m, address)
	e.clock.Advance(time.Second)
	if err := e.identity.ChangePassword(ctx, appidentity.ChangePasswordInput{
		UserID: reg.User.ID, Confirmation: appidentity.Confirmation{Password: "a brand new password"}, NewPassword: "yet another password",
	}); err != nil {
		t.Fatalf("ChangePassword = %v", err)
	}
	if _, err := e.identity.ResetPassword(ctx, older, "a third password"); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("ResetPassword with a link older than the password = %v, want ErrInvalidToken", err)
	}
}

// passwordResetLimitsRequests limits the reset emails per address, silently, and the requests
// per client address; without email delivery, nobody can ask for a link.
func passwordResetLimitsRequests(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	m := &Mailer{}
	e.withMailer(m)
	e.sessionUser(t)
	for range appidentity.PasswordResetEmailLimit.Max + 1 {
		e.requestReset(t, sessionEmail)
	}
	if n := int64(len(m.Sent())); n != appidentity.PasswordResetEmailLimit.Max {
		t.Errorf("emails = %d, want %d: the requests beyond the limit send nothing", n, appidentity.PasswordResetEmailLimit.Max)
	}
	for i := range appidentity.PasswordResetIPLimit.Max {
		if err := e.identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: "nobody@example.com", IP: "192.0.2.9"}); err != nil {
			t.Fatalf("request #%d = %v", i+1, err)
		}
	}
	err := e.identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: "nobody@example.com", IP: "192.0.2.9"})
	var limited *kernel.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Errorf("request beyond the limit of the client address = %v, want a RateLimitedError", err)
	}

	e.identity = e.newIdentityWith(func(d *appidentity.Deps) { d.Mailer = m })
	if e.identity.PasswordResetByEmail() {
		t.Error("PasswordResetByEmail without email delivery = true")
	}
	if err := e.identity.RequestPasswordReset(ctx, appidentity.PasswordResetRequest{Email: sessionEmail}); !errors.Is(err, appidentity.ErrPasswordResetByEmailUnavailable) {
		t.Errorf("RequestPasswordReset without email delivery = %v, want ErrPasswordResetByEmailUnavailable", err)
	}
}

// passwordChangeKeepsTheCurrentGrant changes a password with the current one: it ends every
// session and every grant but the one that made the change.
func passwordChangeKeepsTheCurrentGrant(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	signed := e.signIn(t, appidentity.SignInInput{})
	current, _ := e.webTokensFor(t, reg.User.ID)
	other, _ := e.webTokensFor(t, reg.User.ID)
	bob := e.register(t, "bob")
	bobTokens, _ := e.webTokensFor(t, bob.User.ID)
	change := func(currentPassword, newPassword string) error {
		return e.identity.ChangePassword(ctx, appidentity.ChangePasswordInput{
			UserID: reg.User.ID, Confirmation: appidentity.Confirmation{Password: currentPassword}, NewPassword: newPassword, KeepFamilyID: current.Token.FamilyID,
		})
	}
	if err := change(sessionPassword, "short"); !errors.Is(err, identity.ErrInvalidPassword) {
		t.Errorf("ChangePassword(short) = %v, want ErrInvalidPassword", err)
	}
	if err := change("wrong password", "a brand new password"); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Errorf("ChangePassword(wrong) = %v, want ErrWrongPassword", err)
	}
	e.wantAccess(t, other.AccessToken, true, "a token before the change")
	e.clock.Advance(time.Second)
	if err := change(sessionPassword, "a brand new password"); err != nil {
		t.Fatalf("ChangePassword = %v", err)
	}
	e.wantInvalidSession(t, signed.Token, "a session from before the change")
	e.wantAccess(t, current.AccessToken, true, "the token that made the change")
	e.wantAccess(t, other.AccessToken, false, "another grant of the user")
	e.wantAccess(t, bobTokens.AccessToken, true, "another user's token")
	if _, err := e.identity.RefreshWebClient(ctx, current.RefreshToken); err != nil {
		t.Errorf("RefreshWebClient with the grant that made the change = %v", err)
	}
	e.signIn(t, appidentity.SignInInput{Email: sessionEmail, Password: "a brand new password"})

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
