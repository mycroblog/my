// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"errors"
	"testing"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/pkg/testkit"
)

// The cases of AccountRepositoryContract about signing in without a password (FR-004):
// confirming operations with a passkey, removing the password, and never losing the last way to
// sign in.

// confirmWithPasskey answers a confirmation of the user's with p.
func (e *AccountEnv) confirmWithPasskey(t *testing.T, userID string, p FakePasskey) appidentity.Confirmation {
	t.Helper()
	begun, err := e.Identity.BeginPasskeyConfirmation(t.Context(), userID)
	if err != nil {
		t.Fatalf("BeginPasskeyConfirmation = %v", err)
	}
	if !begun.ExpiresAt.Equal(e.Clock.Now().Add(appidentity.PasskeyCeremonyTTL)) {
		t.Errorf("ExpiresAt = %v, want %v", begun.ExpiresAt, e.Clock.Now().Add(appidentity.PasskeyCeremonyTTL))
	}
	return appidentity.Confirmation{Passkey: appidentity.PasskeyAssertion{Token: begun.Token, Response: p.SignIn(begun.Options)}}
}

func (e *AccountEnv) wantHasPassword(t *testing.T, userID string, want bool) {
	t.Helper()
	if got, err := e.Identity.HasPassword(t.Context(), userID); err != nil || got != want {
		t.Errorf("HasPassword = %t, %v; want %t", got, err, want)
	}
}

// passwordlessConfirmsWithAPasskey confirms the operations that take the password with a passkey
// of the user instead: once per confirmation, only the user's own passkey, and wrong ones count
// against the operation's limit.
func passwordlessConfirmsWithAPasskey(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	reg := e.SessionUser(t)
	userID := reg.User.ID
	phone, _ := e.RegisterPasskey(t, userID, "phone")

	// A registration confirmed with a passkey.
	c := e.confirmWithPasskey(t, userID, phone)
	begun, err := e.Identity.BeginPasskeyRegistration(ctx, userID, "laptop", c)
	if err != nil {
		t.Fatalf("BeginPasskeyRegistration(passkey) = %v", err)
	}
	laptop := NewFakePasskey(userID)
	if _, err := e.Identity.FinishPasskeyRegistration(ctx, userID, begun.Token, laptop.Register(begun.Options)); err != nil {
		t.Fatalf("FinishPasskeyRegistration = %v", err)
	}
	// The confirmation works once.
	if _, err := e.Identity.BeginPasskeyRegistration(ctx, userID, "tablet", c); !errors.Is(err, appidentity.ErrPasskeyConfirmationExpired) {
		t.Errorf("BeginPasskeyRegistration(used confirmation) = %v, want ErrPasskeyConfirmationExpired", err)
	}
	// Another user's confirmation, a garbage token, and an expired one.
	bob := e.Register(t, "bob")
	bobs := e.confirmWithPasskey(t, bob.User.ID, phone)
	if _, err := e.Identity.BeginPasskeyRegistration(ctx, userID, "tablet", bobs); !errors.Is(err, appidentity.ErrPasskeyConfirmationExpired) {
		t.Errorf("BeginPasskeyRegistration(another user's confirmation) = %v, want ErrPasskeyConfirmationExpired", err)
	}
	garbage := appidentity.Confirmation{Passkey: appidentity.PasskeyAssertion{Token: "garbage", Response: []byte("{}")}}
	if _, err := e.Identity.BeginPasskeyRegistration(ctx, userID, "tablet", garbage); !errors.Is(err, appidentity.ErrPasskeyConfirmationExpired) {
		t.Errorf("BeginPasskeyRegistration(garbage confirmation) = %v, want ErrPasskeyConfirmationExpired", err)
	}
	expiring := e.confirmWithPasskey(t, userID, phone)
	e.Clock.Advance(appidentity.PasskeyCeremonyTTL)
	if _, err := e.Identity.BeginPasskeyRegistration(ctx, userID, "tablet", expiring); !errors.Is(err, appidentity.ErrPasskeyConfirmationExpired) {
		t.Errorf("BeginPasskeyRegistration(expired confirmation) = %v, want ErrPasskeyConfirmationExpired", err)
	}
	// Bob's passkey does not confirm an operation of alice's.
	bobPasskey, _ := e.registerPasskeyWith(t, bob.User.ID, "bob's phone", appidentity.Confirmation{Password: sessionPassword})
	if _, err := e.Identity.BeginPasskeyRegistration(ctx, userID, "tablet", e.confirmWithPasskey(t, userID, bobPasskey)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("BeginPasskeyRegistration(another user's passkey) = %v, want ErrInvalidPasskey", err)
	}

	// Changing the password and deleting the account.
	tokens, _ := e.WebTokensFor(t, userID)
	if err := e.Identity.ChangePassword(ctx, appidentity.ChangePasswordInput{
		UserID: userID, Confirmation: e.confirmWithPasskey(t, userID, laptop), NewPassword: "a brand new password", KeepFamilyID: tokens.Token.FamilyID,
	}); err != nil {
		t.Fatalf("ChangePassword(passkey) = %v", err)
	}
	e.SignIn(t, appidentity.SignInInput{Email: sessionEmail, Password: "a brand new password"})
	stranger := NewFakePasskey(userID)
	if err := e.Identity.DeleteOwnAccount(ctx, userID, e.confirmWithPasskey(t, userID, stranger)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("DeleteOwnAccount(unknown passkey) = %v, want ErrInvalidPasskey", err)
	}
	// The last administrator cannot delete their account.
	if _, err := e.Identity.CreateAdmin(ctx, appidentity.CreateAdminInput{
		Username: "other", Email: "other@example.com", Password: "a brand new password",
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Identity.DeleteOwnAccount(ctx, userID, e.confirmWithPasskey(t, userID, phone)); err != nil {
		t.Errorf("DeleteOwnAccount(passkey) = %v", err)
	}

	// Wrong passkeys count against the limit of the operation.
	for range appidentity.PasskeyPasswordLimit.Max {
		if _, err := e.Identity.BeginPasskeyRegistration(ctx, bob.User.ID, "key", e.confirmWithPasskey(t, bob.User.ID, stranger)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
			t.Fatalf("BeginPasskeyRegistration(unknown passkey) = %v, want ErrInvalidPasskey", err)
		}
	}
	var limited *kernel.RateLimitedError
	if _, err := e.Identity.BeginPasskeyRegistration(ctx, bob.User.ID, "key", e.confirmWithPasskey(t, bob.User.ID, bobPasskey)); !errors.As(err, &limited) {
		t.Errorf("BeginPasskeyRegistration after %d wrong passkeys = %v, want a *kernel.RateLimitedError", appidentity.PasskeyPasswordLimit.Max, err)
	}
}

// passwordlessRemovesThePassword removes the password with a passkey: the password no longer
// signs in, the passkeys do (with the code of TOTP), the other sessions and grants end, the last
// passkey stays until a password is set again, and a reset link sets one.
func passwordlessRemovesThePassword(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	m := &testkit.Mailer{}
	e.withMailer(m)
	reg := e.SessionUser(t)
	const address = "Alice@Example.com" // the stored address of the session user
	userID := reg.User.ID
	e.wantHasPassword(t, userID, true)
	phone, phoneCred := e.RegisterPasskey(t, userID, "phone")
	signed := e.SignIn(t, appidentity.SignInInput{})
	current, _ := e.WebTokensFor(t, userID)
	other, _ := e.WebTokensFor(t, userID)
	e.requestReset(t, sessionEmail)
	older := resetToken(t, m, address)

	if err := e.Identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: userID}); !errors.Is(err, appidentity.ErrPasskeyRequired) {
		t.Errorf("RemovePassword without a passkey = %v, want ErrPasskeyRequired", err)
	}
	stranger := NewFakePasskey(userID)
	bad := e.confirmWithPasskey(t, userID, stranger)
	if err := e.Identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: userID, Passkey: bad.Passkey}); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("RemovePassword(unknown passkey) = %v, want ErrInvalidPasskey", err)
	}
	e.wantHasPassword(t, userID, true)

	e.Clock.Advance(time.Second)
	sent := len(m.Sent())
	c := e.confirmWithPasskey(t, userID, phone)
	if err := e.Identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: userID, Passkey: c.Passkey, KeepFamilyID: current.Token.FamilyID}); err != nil {
		t.Fatalf("RemovePassword = %v", err)
	}
	e.wantHasPassword(t, userID, false)
	if got := m.Sent(); len(got) != sent+1 || got[len(got)-1].To != address {
		t.Errorf("emails after the removal = %d, want one more to %s", len(got), address)
	}
	if _, err := e.Identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword}); !errors.Is(err, appidentity.ErrInvalidCredentials) {
		t.Errorf("SignIn with the removed password = %v, want ErrInvalidCredentials", err)
	}
	e.wantInvalidSession(t, signed.Token, "a session from before the removal")
	e.WantAccess(t, current.AccessToken, true, "the token that removed the password")
	e.WantAccess(t, other.AccessToken, false, "another grant of the user")
	if _, err := e.signInWithPasskey(t, phone); err != nil {
		t.Errorf("SignInWithPasskey after the removal = %v", err)
	}
	// The password confirms nothing any more; a removal without a password changes nothing.
	if _, err := e.Identity.BeginPasskeyRegistration(ctx, userID, "laptop", appidentity.Confirmation{Password: sessionPassword}); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Errorf("BeginPasskeyRegistration(removed password) = %v, want ErrWrongPassword", err)
	}
	again := e.confirmWithPasskey(t, userID, phone)
	if err := e.Identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: userID, Passkey: again.Passkey}); err != nil {
		t.Errorf("RemovePassword without a password = %v, want nil", err)
	}
	if n := len(m.Sent()); n != sent+1 {
		t.Errorf("emails after removing no password = %d, want %d", n, sent+1)
	}

	// The last passkey stays while there is no password; another one can go.
	if err := e.Identity.RemovePasskey(ctx, userID, phoneCred.ID); !errors.Is(err, appidentity.ErrLastSignInMethod) {
		t.Errorf("RemovePasskey(the last way to sign in) = %v, want ErrLastSignInMethod", err)
	}
	laptop, laptopCred := e.registerPasskeyWith(t, userID, "laptop", e.confirmWithPasskey(t, userID, phone))
	if err := e.Identity.RemovePasskey(ctx, userID, phoneCred.ID); err != nil {
		t.Errorf("RemovePasskey(one of two) = %v", err)
	}
	if err := e.Identity.RemovePasskey(ctx, userID, laptopCred.ID); !errors.Is(err, appidentity.ErrLastSignInMethod) {
		t.Errorf("RemovePasskey(the last way to sign in) = %v, want ErrLastSignInMethod", err)
	}

	// A user with TOTP still enters a code after the passkey.
	user := e.EnableTOTP(t, reg)
	_, err := e.signInWithPasskey(t, laptop)
	var need *appidentity.SecondFactorRequiredError
	if !errors.As(err, &need) {
		t.Fatalf("SignInWithPasskey with TOTP = %v, want a *SecondFactorRequiredError", err)
	}
	if _, err := e.completeSignIn(need.Challenge, user.code(e)); err != nil {
		t.Errorf("CompleteSignIn after a passkey = %v", err)
	}

	// A link from before the removal no longer works; a new one sets a password, which lets the
	// last passkey go.
	if _, err := e.Identity.ResetPassword(ctx, older, "a brand new password"); !errors.Is(err, appidentity.ErrInvalidToken) {
		t.Errorf("ResetPassword with a link older than the removal = %v, want ErrInvalidToken", err)
	}
	e.Clock.Advance(time.Second)
	e.requestReset(t, sessionEmail)
	if _, err := e.Identity.ResetPassword(ctx, resetToken(t, m, address), "a brand new password"); err != nil {
		t.Fatalf("ResetPassword without a password = %v", err)
	}
	e.wantHasPassword(t, userID, true)
	if err := e.Identity.RemovePasskey(ctx, userID, laptopCred.ID); err != nil {
		t.Errorf("RemovePasskey with a password = %v", err)
	}
	// Without a passkey, the password cannot be removed: a passkey removed after its
	// confirmation began no longer confirms.
	p, pc := e.registerPasskeyWith(t, userID, "key", appidentity.Confirmation{Password: "a brand new password"})
	stale := e.confirmWithPasskey(t, userID, p)
	if err := e.Identity.RemovePasskey(ctx, userID, pc.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.Identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: userID, Passkey: stale.Passkey}); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("RemovePassword(removed passkey) = %v, want ErrInvalidPasskey", err)
	}
	e.wantHasPassword(t, userID, true)
}
