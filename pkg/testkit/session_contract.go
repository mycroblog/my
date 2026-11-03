// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The session cases of AccountRepositoryContract.

const (
	sessionEmail    = "alice@example.com"
	sessionPassword = "correct horse battery staple"
)

// sessionUser creates a user who signs in with sessionEmail and sessionPassword.
func (e *accountEnv) sessionUser(t *testing.T) appidentity.Registration {
	t.Helper()
	reg, err := e.identity.CreateAdmin(t.Context(), appidentity.CreateAdminInput{
		Username: "alice", Email: "Alice@Example.com", Password: sessionPassword,
	})
	if err != nil {
		t.Fatalf("CreateAdmin = %v", err)
	}
	return reg
}

func (e *accountEnv) signIn(t *testing.T, in appidentity.SignInInput) appidentity.SignedIn {
	t.Helper()
	if in.Email == "" {
		in.Email, in.Password = sessionEmail, sessionPassword
	}
	got, err := e.identity.SignIn(t.Context(), in)
	if err != nil {
		t.Fatalf("SignIn(%q) = %v", in.Email, err)
	}
	return got
}

func (e *accountEnv) wantInvalidSession(t *testing.T, token, why string) {
	t.Helper()
	if s, err := e.identity.Authenticate(t.Context(), token); !errors.Is(err, appidentity.ErrInvalidSession) {
		t.Fatalf("Authenticate(%s) = %+v, %v; want ErrInvalidSession", why, s, err)
	}
}

func sessionSignsInAndOut(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	e.clock.Advance(time.Second)
	in := appidentity.SignInInput{
		Email: "ALICE@example.com", Password: sessionPassword, IP: "192.0.2.1",
		UserAgent: strings.Repeat("é", identity.MaxUserAgentLength), // cut to whole characters
	}
	got := e.signIn(t, in)
	if !strings.HasPrefix(got.Token, appidentity.SessionTokenPrefix) || len(got.Token) < len(appidentity.SessionTokenPrefix)+43 {
		t.Errorf("token = %q, want %s and 256 bits of base64url", got.Token, appidentity.SessionTokenPrefix)
	}
	if !bytes.Equal(got.Session.TokenHash, appidentity.HashToken(got.Token)) {
		t.Error("the session does not store the token's hash")
	}
	now := e.clock.Now()
	s, err := e.identity.Authenticate(ctx, got.Token)
	if err != nil {
		t.Fatalf("Authenticate = %v", err)
	}
	if s.ID != got.Session.ID || s.UserID != reg.User.ID || !s.CreatedAt.Equal(now) ||
		!s.ExpiresAt.Equal(now.Add(appidentity.DefaultSessionTTL)) {
		t.Errorf("Authenticate = %+v, want the new session of %s created at %v", s, reg.User.ID, now)
	}
	o, err := e.h.Identity.FindSessionOrigin(ctx, s.ID)
	if err != nil || o.SessionID != s.ID || o.UserID != reg.User.ID || o.IP != "192.0.2.1" || !o.CreatedAt.Equal(now) {
		t.Errorf("FindSessionOrigin = %+v, %v; want 192.0.2.1 for the session created at %v", o, err, now)
	}
	if len(o.UserAgent) > identity.MaxUserAgentLength || o.UserAgent != strings.Repeat("é", identity.MaxUserAgentLength/2) {
		t.Errorf("stored user agent has %d bytes, want it cut to %d bytes of whole characters", len(o.UserAgent), identity.MaxUserAgentLength)
	}

	// Activity is recorded at sign-in and refreshed only after the interval.
	_, act, err := e.h.Identity.FindUnrevokedSession(ctx, got.Session.TokenHash)
	if err != nil || !act.LastSeenAt.Equal(now) {
		t.Fatalf("activity after sign-in = %+v, %v; want %v", act, err, now)
	}
	e.clock.Advance(appidentity.SessionActivityInterval - time.Millisecond)
	if _, err := e.identity.Authenticate(ctx, got.Token); err != nil {
		t.Fatal(err)
	}
	if _, act, _ := e.h.Identity.FindUnrevokedSession(ctx, got.Session.TokenHash); !act.LastSeenAt.Equal(now) {
		t.Errorf("activity within the interval = %v, want it unchanged at %v", act.LastSeenAt, now)
	}
	later := e.clock.Advance(time.Millisecond)
	if _, err := e.identity.Authenticate(ctx, got.Token); err != nil {
		t.Fatal(err)
	}
	if _, act, _ := e.h.Identity.FindUnrevokedSession(ctx, got.Session.TokenHash); !act.LastSeenAt.Equal(later) {
		t.Errorf("activity after the interval = %v, want %v", act.LastSeenAt, later)
	}

	e.wantInvalidSession(t, "", "empty token")
	e.wantInvalidSession(t, appidentity.SessionTokenPrefix+"unknown", "unknown token")
	for range 2 { // signing out twice is harmless
		if err := e.identity.SignOut(ctx, got.Token); err != nil {
			t.Fatalf("SignOut = %v", err)
		}
	}
	e.wantInvalidSession(t, got.Token, "signed-out token")
	if err := e.identity.SignOut(ctx, "unknown"); err != nil {
		t.Errorf("SignOut(unknown) = %v, want nil", err)
	}
}

func sessionReplacedOnSignIn(t *testing.T, e *accountEnv) {
	e.sessionUser(t)
	first := e.signIn(t, appidentity.SignInInput{})
	second := e.signIn(t, appidentity.SignInInput{CurrentToken: first.Token})
	if second.Token == first.Token || second.Session.ID == first.Session.ID {
		t.Fatal("signing in again kept the session token or ID")
	}
	e.wantInvalidSession(t, first.Token, "replaced token")
	if _, err := e.identity.Authenticate(t.Context(), second.Token); err != nil {
		t.Fatalf("Authenticate(new token) = %v", err)
	}
	// A revoked or unknown current token does not stop the sign-in.
	third := e.signIn(t, appidentity.SignInInput{CurrentToken: first.Token})
	e.signIn(t, appidentity.SignInInput{CurrentToken: "garbage"})
	if _, err := e.identity.Authenticate(t.Context(), third.Token); err != nil {
		t.Fatalf("Authenticate(third) = %v", err)
	}
	// A failed sign-in leaves the current session alone.
	if _, err := e.identity.SignIn(t.Context(), appidentity.SignInInput{
		Email: sessionEmail, Password: "wrong password", CurrentToken: third.Token,
	}); !errors.Is(err, appidentity.ErrInvalidCredentials) {
		t.Fatalf("SignIn(wrong password) = %v", err)
	}
	if _, err := e.identity.Authenticate(t.Context(), third.Token); err != nil {
		t.Fatalf("Authenticate after a failed sign-in = %v", err)
	}
}

func sessionExpires(t *testing.T, e *accountEnv) {
	e.sessionUser(t)
	got := e.signIn(t, appidentity.SignInInput{})
	e.clock.Advance(appidentity.DefaultSessionTTL - time.Millisecond)
	if _, err := e.identity.Authenticate(t.Context(), got.Token); err != nil {
		t.Fatalf("Authenticate just before expiry = %v", err)
	}
	e.clock.Advance(time.Millisecond)
	e.wantInvalidSession(t, got.Token, "expired token")
}

func sessionRejectsWrongCredentials(t *testing.T, e *accountEnv) {
	e.sessionUser(t)
	e.register(t, "nopassword") // a user without an email address cannot be signed in to by email
	for _, in := range []appidentity.SignInInput{
		{Email: sessionEmail, Password: "wrong password"},
		{Email: "nobody@example.com", Password: sessionPassword},
		{Email: "", Password: ""},
	} {
		got, err := e.identity.SignIn(t.Context(), in)
		if !errors.Is(err, appidentity.ErrInvalidCredentials) || got.Token != "" {
			t.Errorf("SignIn(%q, %q) = %+v, %v; want ErrInvalidCredentials", in.Email, in.Password, got, err)
		}
	}
}

func sessionRateLimitsFailures(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	e.sessionUser(t)
	fail := func(email, ip string) error {
		_, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: email, Password: "wrong password", IP: ip})
		return err
	}
	// Five failures from one IP address (IPv6 addresses count per /64) close it for everyone.
	for i := range appidentity.LoginIPLimit.Max {
		ip := fmt.Sprintf("2001:db8::%d", i+1)
		if err := fail("nobody@example.com", ip); !errors.Is(err, appidentity.ErrInvalidCredentials) {
			t.Fatalf("failure #%d = %v, want ErrInvalidCredentials", i+1, err)
		}
	}
	_, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword, IP: "2001:db8::ff"})
	var limited *kernel.RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("SignIn from the limited /64 = %v, want a RateLimitedError with a Retry-After", err)
	}
	e.signIn(t, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword, IP: "192.0.2.1"})

	// Ten failures for one address, from any IP addresses, close the address.
	for i := range appidentity.LoginAccountLimit.Max {
		ip := fmt.Sprintf("198.51.100.%d", i+1)
		if err := fail(sessionEmail, ip); !errors.Is(err, appidentity.ErrInvalidCredentials) {
			t.Fatalf("account failure #%d = %v, want ErrInvalidCredentials", i+1, err)
		}
	}
	_, err = e.identity.SignIn(ctx, appidentity.SignInInput{Email: "ALICE@example.com", Password: sessionPassword, IP: "203.0.113.1"})
	if !errors.Is(err, kernel.ErrRateLimited) {
		t.Fatalf("SignIn to the limited address = %v, want ErrRateLimited", err)
	}
	e.clock.Advance(2 * appidentity.LoginAccountLimit.Window)
	e.signIn(t, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword, IP: "203.0.113.1"})
}

func sessionRehashes(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	before, err := e.h.Identity.FindCurrentPasswordCredential(ctx, reg.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	stronger := cheapArgon2
	stronger.Iterations++
	hasher := appidentity.NewPasswordHasher(stronger, 0)
	e.clock.Advance(time.Second)
	if _, err := e.newIdentity(hasher).SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword}); err != nil {
		t.Fatalf("SignIn with stronger parameters = %v", err)
	}
	after, err := e.h.Identity.FindCurrentPasswordCredential(ctx, reg.User.ID)
	if err != nil || after.ID == before.ID {
		t.Fatalf("current credential = %+v, %v; want a new one", after, err)
	}
	if ok, rehash, err := hasher.Verify(ctx, sessionPassword, after.Hash); !ok || rehash || err != nil {
		t.Errorf("new hash Verify = %v, %v, %v; want true, false, nil", ok, rehash, err)
	}
}

func sessionRejectsInactiveAccounts(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	_, wrongErr := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: "wrong password"})
	record := func(kind account.LifecycleKind) {
		t.Helper()
		e.clock.Advance(time.Second)
		if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
			AccountID: reg.AccountID, Kind: kind, InitiatedBy: account.InitiatedBySelf,
		}); err != nil {
			t.Fatalf("RecordLifecycleEvent(%s) = %v", kind, err)
		}
	}
	// A deactivated account signs in, to the server-rendered status page only.
	record(account.LifecycleDeactivated)
	signedIn, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword})
	if err != nil {
		t.Fatalf("SignIn to a deactivated account = %v", err)
	}
	a, err := e.identity.UserAccess(ctx, signedIn.Session.UserID)
	if err != nil || a.Allowed() || !a.Deactivated || a.DeletionDueAt == nil || a.DeletionDate == "" {
		t.Fatalf("UserAccess of a deactivated account = %+v, %v", a, err)
	}
	record(account.LifecycleReactivated)
	if _, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword}); err != nil {
		t.Fatalf("SignIn after reactivation = %v", err)
	}
	// Past the date of its deletion, also before the job records the deletion, the right password
	// fails exactly like a wrong one.
	record(account.LifecycleDeactivated)
	e.clock.Advance(account.DeactivationPeriod)
	_, err = e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword})
	if !errors.Is(err, appidentity.ErrInvalidCredentials) || err.Error() != wrongErr.Error() {
		t.Fatalf("SignIn past the date = %v, want the wrong-password error %v", err, wrongErr)
	}
}

// loginPolicy is an identity.LoginPolicy that forbids the accounts in it to log in.
type loginPolicy map[string]bool

func (p loginPolicy) LoginRestricted(_ context.Context, accountID string) (bool, error) {
	return p[accountID], nil
}

func (loginPolicy) Banned(context.Context, string) (bool, error) { return false, nil }

// banPolicy is an identity.LoginPolicy with a ban in effect on the accounts in it, which also
// forbids them to log in.
type banPolicy map[string]bool

func (p banPolicy) LoginRestricted(_ context.Context, accountID string) (bool, error) {
	return p[accountID], nil
}

func (p banPolicy) Banned(_ context.Context, accountID string) (bool, error) {
	return p[accountID], nil
}

// sessionBannedSignsInUntilDeletion checks that an account whose deletion a ban scheduled signs
// in to the server-rendered screens only, so that the user can appeal (FR-707), while one whose
// deletion is scheduled without a ban does not, and that the deletion ends its sessions and
// tokens and keeps it from signing in again (data-model §4.1).
func sessionBannedSignsInUntilDeletion(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	appidentity.RegisterLifecycleProjectors(e.projectors, e.h.Identity, e.accounts, e.clock)
	reg := e.sessionUser(t)
	_, wrongErr := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: "wrong password"})
	due := e.clock.Now().Add(30 * 24 * time.Hour)
	if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: reg.AccountID, Kind: account.LifecycleDeletionScheduled, InitiatedBy: account.InitiatedByModerator, DeletionDueAt: &due,
	}); err != nil {
		t.Fatalf("RecordLifecycleEvent(deletion_scheduled) = %v", err)
	}
	// Without a ban in effect (the owner's own deletion, or a login restriction of another
	// kind), the account does not sign in, exactly like with a wrong password.
	for name, policy := range map[string]appidentity.LoginPolicy{"no policy": nil, "a login restriction": loginPolicy{reg.AccountID: true}} {
		_, err := e.newIdentityWith(func(d *appidentity.Deps) { d.Logins = policy }).SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword})
		if !errors.Is(err, appidentity.ErrInvalidCredentials) || err.Error() != wrongErr.Error() {
			t.Errorf("SignIn to an account scheduled for deletion with %s = %v, want the wrong-password error %v", name, err, wrongErr)
		}
	}

	e.identity = e.newIdentityWith(func(d *appidentity.Deps) { d.Logins = banPolicy{reg.AccountID: true} })
	signed := e.signIn(t, appidentity.SignInInput{})
	if _, err := e.identity.Authenticate(ctx, signed.Token); err != nil {
		t.Errorf("Authenticate the banned account's session = %v", err)
	}
	if a, err := e.identity.UserAccess(ctx, reg.User.ID); err != nil || a.Allowed() || !a.LoginRestricted || a.AccountID != reg.AccountID || a.AccountActive {
		t.Errorf("UserAccess of the banned account = %+v, %v; want restricted, inactive, and not allowed", a, err)
	}
	tokens, _ := e.webTokensFor(t, reg.User.ID)
	if _, err := e.identity.AuthenticateBearer(ctx, tokens.AccessToken); !errors.Is(err, appidentity.ErrAccountUnavailable) {
		t.Errorf("AuthenticateBearer of the banned account = %v, want ErrAccountUnavailable", err)
	}

	// The deletion ends the session and the tokens and keeps the account from signing in again.
	e.clock.Set(due)
	if n, err := e.accounts.ExecuteDueDeletions(ctx); err != nil || n != 1 {
		t.Fatalf("ExecuteDueDeletions = %d, %v; want 1", n, err)
	}
	e.wantInvalidSession(t, signed.Token, "a session of a deleted account")
	if _, err := e.identity.RefreshWebClient(ctx, tokens.RefreshToken); err == nil {
		t.Error("RefreshWebClient with a token of a deleted account = nil, want an error")
	}
	if _, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword}); !errors.Is(err, appidentity.ErrInvalidCredentials) {
		t.Errorf("SignIn to the deleted account = %v, want ErrInvalidCredentials", err)
	}
}

// sessionModerationEndsLogins checks the login restriction (data-model §5.2): the policy keeps
// a restricted account from using the app and its tokens while it still signs in to the
// server-rendered screens, and revoking a user's
// sessions and tokens ends those of that user only, once.
func sessionModerationEndsLogins(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	signed := e.signIn(t, appidentity.SignInInput{})
	tokens, _ := e.webTokensFor(t, reg.User.ID)
	bob := e.register(t, "bob")
	bobTokens, _ := e.webTokensFor(t, bob.User.ID)

	policy := loginPolicy{reg.AccountID: true}
	e.identity = e.newIdentityWith(func(d *appidentity.Deps) { d.Logins = policy })
	// The restricted user signs in to the server-rendered screens only, which show the actions;
	// the user may not use the app.
	if _, err := e.identity.SignIn(ctx, appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword}); err != nil {
		t.Errorf("SignIn while login is restricted = %v, want a session for the screens", err)
	}
	if a, err := e.identity.UserAccess(ctx, reg.User.ID); err != nil || a.Allowed() || !a.LoginRestricted || a.AccountID != reg.AccountID || !a.AccountActive {
		t.Errorf("UserAccess while login is restricted = %+v, %v; want restricted and not allowed", a, err)
	}
	if a, err := e.identity.UserAccess(ctx, bob.User.ID); err != nil || !a.Allowed() || a.LoginRestricted {
		t.Errorf("UserAccess of another user = %+v, %v; want allowed", a, err)
	}
	if _, err := e.identity.AuthenticateBearer(ctx, tokens.AccessToken); !errors.Is(err, appidentity.ErrAccountUnavailable) {
		t.Errorf("AuthenticateBearer while login is restricted = %v, want ErrAccountUnavailable", err)
	}
	if _, err := e.identity.AuthenticateBearer(ctx, bobTokens.AccessToken); err != nil {
		t.Errorf("AuthenticateBearer of another user = %v", err)
	}

	revoke := func() {
		t.Helper()
		err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			now := e.clock.Now()
			if err := e.h.Identity.RevokeSessionsOfUser(ctx, tx, reg.User.ID, identity.RevokedModeration, now); err != nil {
				return err
			}
			return e.h.Identity.RevokeTokensOfUser(ctx, tx, reg.User.ID, identity.TokenRevokedModeration, now)
		})
		if err != nil {
			t.Fatalf("revoke the sessions and tokens of a user = %v", err)
		}
	}
	revoke()
	revoke() // revoking again changes nothing
	e.wantInvalidSession(t, signed.Token, "a session revoked by moderation")
	e.wantAccess(t, tokens.AccessToken, false, "a token revoked by moderation")
	e.wantAccess(t, bobTokens.AccessToken, true, "another user's token")
	if _, err := e.identity.RefreshWebClient(ctx, tokens.RefreshToken); err == nil {
		t.Error("RefreshWebClient with a token revoked by moderation = nil, want an error")
	}

	// Once the restriction ends, the user signs in again.
	delete(policy, reg.AccountID)
	again := e.signIn(t, appidentity.SignInInput{})
	if _, err := e.identity.Authenticate(ctx, again.Token); err != nil {
		t.Errorf("Authenticate after the restriction ended = %v", err)
	}
}
