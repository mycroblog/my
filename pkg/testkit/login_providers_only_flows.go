// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"errors"
	"net/url"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The cases of AccountRepositoryContract about limiting registration and sign-in to the login
// providers.

// limitEnv is an identity service whose login_providers_only setting and registration mode a test
// changes, with providers in the database.
type limitEnv struct {
	*accountEnv
	on   bool
	mode identity.RegistrationMode
}

// noExternalHTTP and noOIDC make external sign-in available without reaching any provider: these
// cases read which providers are enabled, not what they answer.
type noExternalHTTP struct{ kernel.HTTPClient }

type noOIDC struct{ appidentity.OIDCClient }

func newLimitEnv(e *accountEnv) *limitEnv {
	l := &limitEnv{accountEnv: e, on: true, mode: identity.RegistrationOpen}
	e.identity = e.newIdentityWith(func(d *appidentity.Deps) {
		d.HTTP, d.OIDC = noExternalHTTP{}, noOIDC{}
		d.LoginProvidersOnly = func(context.Context) (bool, error) { return l.on, nil }
		d.RegistrationMode = func(context.Context) (identity.RegistrationMode, error) { return l.mode, nil }
	})
	return l
}

// provider sets up a provider in the database in state, and returns its ID.
func (l *limitEnv) provider(t *testing.T, state identity.LoginProviderState, allowSignup bool) string {
	t.Helper()
	ctx := t.Context()
	s := identity.NewLoginProviderSettings(identity.LoginProviderOIDC)
	s.DisplayName, s.Issuer, s.ClientID, s.AllowSignup = "IdP", "https://idp.example", "client", allowSignup
	now := l.clock.Now()
	id, revID, secretID, changeID := l.ids.NewID(), l.ids.NewID(), l.ids.NewID(), l.ids.NewID()
	r := l.h.Identity
	if err := l.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := r.InsertLoginProvider(ctx, tx, id, "", now); err != nil {
			return err
		}
		if err := r.InsertLoginProviderRevision(ctx, tx, identity.LoginProviderRevision{ID: revID, ProviderID: id, Settings: s, RevisedAt: now}); err != nil {
			return err
		}
		if err := r.InsertLoginProviderSecret(ctx, tx, identity.LoginProviderSecret{ID: secretID, ProviderID: id, Sealed: []byte("sealed"), CreatedAt: now}); err != nil {
			return err
		}
		if err := r.InsertLoginProviderStateChange(ctx, tx, identity.LoginProviderStateChange{ID: changeID, ProviderID: id, State: state, ChangedAt: now}); err != nil {
			return err
		}
		return r.UpsertLoginProviderHead(ctx, tx, identity.LoginProviderHead{ProviderID: id, RevisionID: revID, State: state, SecretID: secretID})
	}); err != nil {
		t.Fatalf("set up a login provider = %v", err)
	}
	return id
}

// link links the account subject at a provider to a user and returns the link's ID.
func (l *limitEnv) link(t *testing.T, providerID, subject, userID string) string {
	t.Helper()
	ctx := t.Context()
	linkID := l.ids.NewID()
	now := l.clock.Now()
	if err := l.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		x, err := l.h.Identity.EnsureExternalIdentity(ctx, tx, identity.ExternalIdentity{ID: l.ids.NewID(), ProviderID: providerID, Subject: subject, CreatedAt: now})
		if err != nil {
			return err
		}
		return l.h.Identity.InsertExternalIdentityLink(ctx, tx, x, identity.ExternalIdentityLink{ID: linkID, ExternalIdentityID: x.ID, UserID: userID, LinkedAt: now})
	}); err != nil {
		t.Fatalf("link = %v", err)
	}
	return linkID
}

func (l *limitEnv) signInWithPassword(email, password string) error {
	_, err := l.identity.SignIn(context.Background(), appidentity.SignInInput{Email: email, Password: password, IP: "192.0.2.1"})
	return err
}

// loginProvidersOnlyFollowsTheRule checks the one rule: the setting is on and a provider is
// enabled. A disabled or deleted provider does not count, and a change applies at once.
func loginProvidersOnlyFollowsTheRule(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	l := newLimitEnv(e)
	l.sessionUser(t)
	check := func(name string, want bool) {
		t.Helper()
		got, err := l.identity.LoginProvidersOnly(ctx)
		if err != nil || got != want {
			t.Errorf("%s: LoginProvidersOnly = %v, %v; want %v", name, got, err, want)
		}
		err = l.signInWithPassword(sessionEmail, sessionPassword)
		if want && !errors.Is(err, appidentity.ErrLoginProvidersOnly) || !want && err != nil {
			t.Errorf("%s: SignIn = %v", name, err)
		}
	}
	check("no provider, setting on", false)
	l.provider(t, identity.LoginProviderDisabled, true)
	check("a disabled provider, setting on", false)
	id := l.provider(t, identity.LoginProviderEnabled, true)
	check("an enabled provider, setting on", true)
	l.on = false
	check("an enabled provider, setting off", false)
	l.on = true
	check("the setting back on", true)
	if _, err := l.identity.DisableLoginProvider(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	check("the last provider disabled", false)
	if _, err := l.identity.EnableLoginProvider(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	check("the provider enabled again", true)
	if _, err := l.identity.DeleteLoginProvider(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	check("the last provider deleted", false)
}

// loginProvidersOnlyRefusesAlike refuses every way in with the same error, whatever is sent, and
// without counting against the rate limits.
func loginProvidersOnlyRefusesAlike(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	l := newLimitEnv(e)
	reg := l.sessionUser(t)
	key, _ := l.registerPasskey(t, reg.User.ID, "phone")
	l.on = false
	begun, err := l.identity.BeginPasskeyRegistration(ctx, reg.User.ID, "laptop", appidentity.Confirmation{Password: sessionPassword})
	if err != nil {
		t.Fatal(err)
	}
	pending := NewFakePasskey(reg.User.ID)
	reset, err := l.identity.IssuePasswordResetLink(ctx, reg.AccountID, "")
	if err != nil || reset.LoginProvidersOnly {
		t.Fatalf("IssuePasswordResetLink without the limit = %+v, %v", reset, err)
	}
	u, err := url.Parse(reset.URL)
	if err != nil {
		t.Fatal(err)
	}
	secret := u.Query().Get("token")
	totp := l.enableTOTP(t, reg)
	challenge := l.signInFirstStep(t)
	l.on = true
	l.provider(t, identity.LoginProviderEnabled, true)

	for _, tc := range []struct{ email, password string }{
		{sessionEmail, sessionPassword}, {sessionEmail, "wrong"}, {"nobody@example.com", sessionPassword}, {"", ""},
	} {
		if err := l.signInWithPassword(tc.email, tc.password); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
			t.Errorf("SignIn(%q, %q) = %v", tc.email, tc.password, err)
		}
	}
	if _, err := l.signInWithPasskey(t, key); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("SignInWithPasskey = %v", err)
	}
	if _, err := l.identity.SignInWithPasskey(ctx, appidentity.PasskeySignInInput{Challenge: "garbage", Response: []byte("garbage")}); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("SignInWithPasskey(garbage) = %v", err)
	}
	// A sign-in that began before the limit does not finish at the second factor.
	if _, err := l.completeSignIn(challenge, totp.code(e)); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("CompleteSignIn = %v", err)
	}
	for _, in := range []appidentity.SignUpInput{
		{Username: "dana", Email: "dana@example.com", Password: sessionPassword},
		{Username: "alice", Email: sessionEmail, Password: sessionPassword},
		{},
	} {
		if _, err := l.identity.SignUp(ctx, in); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
			t.Errorf("SignUp(%+v) = %v", in, err)
		}
	}
	if _, err := l.identity.ResetPassword(ctx, secret, "another good password"); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("ResetPassword = %v", err)
	}
	if _, err := l.identity.ResetPassword(ctx, "garbage", ""); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("ResetPassword(garbage) = %v", err)
	}
	if err := l.identity.ChangePassword(ctx, appidentity.ChangePasswordInput{
		UserID: reg.User.ID, Confirmation: appidentity.Confirmation{Password: sessionPassword}, NewPassword: "another good password",
	}); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("ChangePassword = %v", err)
	}
	if _, err := l.identity.BeginPasskeyRegistration(ctx, reg.User.ID, "tablet", appidentity.Confirmation{Password: sessionPassword}); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("BeginPasskeyRegistration = %v", err)
	}
	// A registration that began before the limit does not finish.
	if _, err := l.identity.FinishPasskeyRegistration(ctx, reg.User.ID, begun.Token, pending.Register(begun.Options)); !errors.Is(err, appidentity.ErrLoginProvidersOnly) {
		t.Errorf("FinishPasskeyRegistration = %v", err)
	}
	// The administrator may still issue a link, which says that it does not work yet.
	limited, err := l.identity.IssuePasswordResetLink(ctx, reg.AccountID, "")
	if err != nil || !limited.LoginProvidersOnly {
		t.Errorf("IssuePasswordResetLink under the limit = %+v, %v", limited, err)
	}

	// The refused link works once the limit ends: nothing used it up.
	l.on = false
	if _, err := l.identity.ResetPassword(ctx, secret, "another good password"); err != nil {
		t.Errorf("ResetPassword after the limit = %v", err)
	}
}

// loginProvidersOnlyKeepsConfirmations confirms operations with a password and a passkey while
// limited: that is how a user without a link links a provider. Removing a credential stays too.
func loginProvidersOnlyKeepsConfirmations(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	l := newLimitEnv(e)
	reg := l.sessionUser(t)
	l.on = false
	key, _ := l.registerPasskey(t, reg.User.ID, "phone")
	l.on = true
	l.provider(t, identity.LoginProviderEnabled, true)

	if err := l.identity.ConfirmMove(ctx, reg.User.ID, appidentity.Confirmation{Password: sessionPassword}); err != nil {
		t.Errorf("a confirmation with the password = %v", err)
	}
	if err := l.identity.ConfirmMove(ctx, reg.User.ID, appidentity.Confirmation{Password: "wrong"}); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Errorf("a confirmation with a wrong password = %v", err)
	}
	assertion := func() appidentity.PasskeyAssertion {
		t.Helper()
		c, err := l.identity.BeginPasskeyConfirmation(ctx, reg.User.ID)
		if err != nil {
			t.Fatal(err)
		}
		return appidentity.PasskeyAssertion{Token: c.Token, Response: key.SignIn(c.Options)}
	}
	if err := l.identity.ConfirmMove(ctx, reg.User.ID, appidentity.Confirmation{Passkey: assertion()}); err != nil {
		t.Errorf("a confirmation with a passkey = %v", err)
	}
	if err := l.identity.RemovePassword(ctx, appidentity.RemovePasswordInput{UserID: reg.User.ID, Passkey: assertion()}); err != nil {
		t.Errorf("RemovePassword = %v", err)
	}
	// The last passkey of a user with nothing else stays: it is what confirms a link.
	passkeys, err := l.identity.Passkeys(ctx, reg.User.ID)
	if err != nil || len(passkeys) != 1 {
		t.Fatalf("Passkeys = %+v, %v", passkeys, err)
	}
	if err := l.identity.RemovePasskey(ctx, reg.User.ID, passkeys[0].ID); !errors.Is(err, appidentity.ErrLastSignInMethod) {
		t.Errorf("RemovePasskey of the last one = %v", err)
	}
}

// loginProvidersOnlyGuardsTheLastLink refuses to unlink the last link to an enabled provider while
// limited, and leaves links that sign nobody in alone.
func loginProvidersOnlyGuardsTheLastLink(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	l := newLimitEnv(e)
	reg := l.sessionUser(t)
	user := reg.User.ID
	one := l.provider(t, identity.LoginProviderEnabled, true)
	two := l.provider(t, identity.LoginProviderEnabled, true)
	off := l.provider(t, identity.LoginProviderDisabled, true)
	linkOne := l.link(t, one, "s1", user)
	linkTwo := l.link(t, two, "s2", user)
	linkOff := l.link(t, off, "s3", user)

	if err := l.identity.UnlinkExternalIdentity(ctx, user, linkOff); err != nil {
		t.Errorf("unlink a disabled provider = %v", err)
	}
	if err := l.identity.UnlinkExternalIdentity(ctx, user, linkOne); err != nil {
		t.Errorf("unlink with another enabled link = %v", err)
	}
	// The password does not sign in while limited, so it does not stand in for the link.
	if err := l.identity.UnlinkExternalIdentity(ctx, user, linkTwo); !errors.Is(err, appidentity.ErrLastSignInMethod) {
		t.Errorf("unlink the last enabled link = %v, want ErrLastSignInMethod", err)
	}
	l.on = false
	if err := l.identity.UnlinkExternalIdentity(ctx, user, linkTwo); err != nil {
		t.Errorf("unlink without the limit, with the password = %v", err)
	}
}

// loginProvidersOnlyClosesRegistration reads as closed when no provider lets new users sign up.
func loginProvidersOnlyClosesRegistration(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	l := newLimitEnv(e)
	check := func(name string, want identity.RegistrationMode) {
		t.Helper()
		if got, err := l.identity.RegistrationMode(ctx); err != nil || got != want {
			t.Errorf("%s: RegistrationMode = %q, %v; want %q", name, got, err, want)
		}
	}
	check("no provider", identity.RegistrationOpen)
	l.provider(t, identity.LoginProviderEnabled, false)
	check("a provider without sign-ups", identity.RegistrationClosed)
	if _, err := l.identity.SignUp(ctx, appidentity.SignUpInput{Username: "dana", Email: "dana@example.com", Password: sessionPassword}); !errors.Is(err, appidentity.ErrRegistrationClosed) {
		t.Errorf("SignUp = %v", err)
	}
	l.on = false
	check("the setting off", identity.RegistrationOpen)
	l.on = true
	l.provider(t, identity.LoginProviderEnabled, true)
	check("a provider with sign-ups", identity.RegistrationOpen)
}

// loginProvidersOnlyCounts counts the active local users without a link to an enabled provider.
func loginProvidersOnlyCounts(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	l := newLimitEnv(e)
	alice := l.sessionUser(t)
	bob := l.register(t, "bob")
	carol := l.register(t, "carol")
	gone := l.register(t, "gone")

	// With no provider, a new one starts the limit, and nobody can have a link to it.
	none, err := l.identity.LoginProvidersOnlyImpact(ctx, alice.User.ID, "")
	if err != nil || none.EnabledProviders != 0 || none.UsersWithoutProvider != 4 || !none.IncludesViewer || none.Limited() || !none.StartsLimit {
		t.Errorf("impact without a provider = %+v, %v", none, err)
	}
	if _, err := l.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: gone.AccountID, Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatal(err)
	}
	enabled := l.provider(t, identity.LoginProviderDisabled, true)
	disabled := l.provider(t, identity.LoginProviderDisabled, true)
	l.link(t, enabled, "bob", bob.User.ID)
	l.link(t, disabled, "carol", carol.User.ID)
	// Only the disabled provider so far: enabling it starts the limit, and counts its links.
	if got, err := l.identity.LoginProvidersOnlyImpact(ctx, carol.User.ID, disabled); err != nil || !got.StartsLimit || got.EnabledProviders != 0 || got.UsersWithoutProvider != 2 || got.IncludesViewer {
		t.Errorf("impact of enabling the disabled provider, as carol = %+v, %v; want alice and bob without a link", got, err)
	}
	if got, err := l.identity.LoginProvidersOnlyImpact(ctx, alice.User.ID, disabled); err != nil || !got.IncludesViewer {
		t.Errorf("impact of enabling the disabled provider, as alice = %+v, %v", got, err)
	}
	if _, err := l.identity.LoginProvidersOnlyImpact(ctx, alice.User.ID, l.ids.NewID()); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("impact of an unknown provider = %v, want ErrNotFound", err)
	}
	if _, err := l.identity.EnableLoginProvider(ctx, enabled, ""); err != nil {
		t.Fatal(err)
	}
	got, err := l.identity.LoginProvidersOnlyImpact(ctx, alice.User.ID, "")
	if err != nil || !got.Limited() || got.EnabledProviders != 1 || got.UsersWithoutProvider != 2 || !got.IncludesViewer || got.StartsLimit {
		t.Errorf("impact as alice = %+v, %v; want alice and carol without a link, and the limit in force", got, err)
	}
	// With a provider in force, enabling another starts nothing.
	if got, err := l.identity.LoginProvidersOnlyImpact(ctx, alice.User.ID, disabled); err != nil || got.StartsLimit {
		t.Errorf("impact of a second provider = %+v, %v", got, err)
	}
	if got, err = l.identity.LoginProvidersOnlyImpact(ctx, bob.User.ID, ""); err != nil || got.IncludesViewer {
		t.Errorf("impact as bob = %+v, %v", got, err)
	}
	l.on = false
	if got, err := l.identity.LoginProvidersOnlyImpact(ctx, alice.User.ID, ""); err != nil || got.StartsLimit {
		t.Errorf("impact with the setting off = %+v, %v", got, err)
	}
	if got, err = l.identity.LoginProvidersOnlyImpact(ctx, "", ""); err != nil || got.SettingOn || got.Limited() || got.UsersWithoutProvider != 2 || got.IncludesViewer {
		t.Errorf("impact with the setting off, for the CLI = %+v, %v", got, err)
	}
}
