// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The passkey cases of AccountRepositoryContract (FR-004).

// FakePasskeys is an identity.PasskeyCeremonies without cryptography, for tests of what the
// identity module does around the ceremonies. Its options are {"challenge", "user",
// "exclude"}, its state {"challenge", "user"}, and the responses that it accepts are those that
// FakePasskey makes: they echo the challenge, and a sign-in carries the public key of the
// credential as its "signature".
type FakePasskeys struct{}

var _ appidentity.PasskeyCeremonies = FakePasskeys{}

type fakeCeremony struct {
	Challenge []byte   `json:"challenge"`
	User      []byte   `json:"user,omitempty"`
	Exclude   [][]byte `json:"exclude,omitempty"`
}

func newFakeCeremony(user []byte, exclude [][]byte) ([]byte, []byte, error) {
	c := fakeCeremony{Challenge: make([]byte, 16), User: user, Exclude: exclude}
	_, _ = rand.Read(c.Challenge)
	options, err := json.Marshal(c)
	if err != nil {
		return nil, nil, err
	}
	state, err := json.Marshal(fakeCeremony{Challenge: c.Challenge, User: user})
	return options, state, err
}

// BeginRegistration implements identity.PasskeyCeremonies.
func (FakePasskeys) BeginRegistration(u appidentity.PasskeyUser, exclude [][]byte) ([]byte, []byte, error) {
	return newFakeCeremony(u.Handle, exclude)
}

// fakeResponse is the response of a FakePasskey.
type fakeResponse struct {
	Challenge    []byte `json:"challenge"`
	CredentialID []byte `json:"credential_id"`
	PublicKey    []byte `json:"public_key,omitempty"`
	Signature    []byte `json:"signature,omitempty"`
	UserHandle   []byte `json:"user_handle,omitempty"`
}

func readFake(state, response []byte) (fakeCeremony, fakeResponse, error) {
	var s fakeCeremony
	var r fakeResponse
	if err := json.Unmarshal(state, &s); err != nil {
		return s, r, err
	}
	if err := json.Unmarshal(response, &r); err != nil || !bytes.Equal(r.Challenge, s.Challenge) {
		return s, r, fmt.Errorf("%w: another challenge (%w)", appidentity.ErrInvalidPasskey, err)
	}
	return s, r, nil
}

// FinishRegistration implements identity.PasskeyCeremonies.
func (FakePasskeys) FinishRegistration(u appidentity.PasskeyUser, state, response []byte) (appidentity.NewPasskey, error) {
	s, r, err := readFake(state, response)
	if err != nil {
		return appidentity.NewPasskey{}, err
	}
	if !bytes.Equal(s.User, u.Handle) {
		return appidentity.NewPasskey{}, fmt.Errorf("%w: another user's registration", appidentity.ErrInvalidPasskey)
	}
	return appidentity.NewPasskey{CredentialID: r.CredentialID, PublicKey: r.PublicKey, BackupEligible: true}, nil
}

// BeginSignIn implements identity.PasskeyCeremonies.
func (FakePasskeys) BeginSignIn() ([]byte, []byte, error) { return newFakeCeremony(nil, nil) }

// FinishSignIn implements identity.PasskeyCeremonies.
func (FakePasskeys) FinishSignIn(state, response []byte, find func([]byte) (identity.PasskeyCredential, error)) (identity.PasskeyCredential, error) {
	_, r, err := readFake(state, response)
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	c, err := find(r.CredentialID)
	if errors.Is(err, appidentity.ErrNotFound) {
		return identity.PasskeyCredential{}, fmt.Errorf("%w: %w", appidentity.ErrInvalidPasskey, err)
	}
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	if !bytes.Equal(r.Signature, c.PublicKey) || !bytes.Equal(r.UserHandle, []byte(c.UserID)) {
		return identity.PasskeyCredential{}, fmt.Errorf("%w: a wrong signature", appidentity.ErrInvalidPasskey)
	}
	return c, nil
}

// FakePasskey is a credential that answers the ceremonies of FakePasskeys.
type FakePasskey struct {
	CredentialID []byte
	PublicKey    []byte
	UserHandle   []byte
}

// NewFakePasskey returns a credential with a random ID and key of the user with userID.
func NewFakePasskey(userID string) FakePasskey {
	p := FakePasskey{CredentialID: make([]byte, 16), PublicKey: make([]byte, 32), UserHandle: []byte(userID)}
	_, _ = rand.Read(p.CredentialID)
	_, _ = rand.Read(p.PublicKey)
	return p
}

func fakeChallenge(options []byte) []byte {
	var o fakeCeremony
	_ = json.Unmarshal(options, &o)
	return o.Challenge
}

// Register answers the options of a registration.
func (p FakePasskey) Register(options []byte) []byte {
	b, _ := json.Marshal(fakeResponse{Challenge: fakeChallenge(options), CredentialID: p.CredentialID, PublicKey: p.PublicKey})
	return b
}

// SignIn answers the options of a sign-in.
func (p FakePasskey) SignIn(options []byte) []byte {
	b, _ := json.Marshal(fakeResponse{
		Challenge: fakeChallenge(options), CredentialID: p.CredentialID, Signature: p.PublicKey, UserHandle: p.UserHandle,
	})
	return b
}

// registerPasskey registers a new FakePasskey called name for the user, confirmed with
// sessionPassword.
func (e *accountEnv) registerPasskey(t *testing.T, userID, name string) (FakePasskey, identity.PasskeyCredential) {
	t.Helper()
	return e.registerPasskeyWith(t, userID, name, appidentity.Confirmation{Password: sessionPassword})
}

// registerPasskeyWith registers a new FakePasskey called name for the user, confirmed with c.
func (e *accountEnv) registerPasskeyWith(t *testing.T, userID, name string, c appidentity.Confirmation) (FakePasskey, identity.PasskeyCredential) {
	t.Helper()
	ctx := t.Context()
	reg, err := e.identity.BeginPasskeyRegistration(ctx, userID, name, c)
	if err != nil {
		t.Fatalf("BeginPasskeyRegistration = %v", err)
	}
	p := NewFakePasskey(userID)
	cred, err := e.identity.FinishPasskeyRegistration(ctx, userID, reg.Token, p.Register(reg.Options))
	if err != nil {
		t.Fatalf("FinishPasskeyRegistration = %v", err)
	}
	return p, cred
}

// signInWithPasskey runs a sign-in with p.
func (e *accountEnv) signInWithPasskey(t *testing.T, p FakePasskey) (appidentity.SignedIn, error) {
	t.Helper()
	ctx := t.Context()
	begun, err := e.identity.BeginPasskeySignIn(ctx)
	if err != nil {
		t.Fatalf("BeginPasskeySignIn = %v", err)
	}
	return e.identity.SignInWithPasskey(ctx, appidentity.PasskeySignInInput{
		Challenge: begun.Challenge, Response: p.SignIn(begun.Options), IP: "192.0.2.7", UserAgent: "ua",
	})
}

func passkeyRegistersListsAndRemoves(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	userID := reg.User.ID
	if !e.identity.PasskeysEnabled() {
		t.Fatal("PasskeysEnabled = false with FakePasskeys")
	}
	if _, err := e.identity.BeginPasskeyRegistration(ctx, userID, "phone", appidentity.Confirmation{Password: "wrong password"}); !errors.Is(err, appidentity.ErrWrongPassword) {
		t.Errorf("BeginPasskeyRegistration(wrong password) = %v, want ErrWrongPassword", err)
	}
	if _, err := e.identity.BeginPasskeyRegistration(ctx, userID, " \t", appidentity.Confirmation{Password: sessionPassword}); !errors.Is(err, identity.ErrInvalidPasskeyName) {
		t.Errorf("BeginPasskeyRegistration(blank name) = %v, want ErrInvalidPasskeyName", err)
	}

	begun, err := e.identity.BeginPasskeyRegistration(ctx, userID, "  phone ", appidentity.Confirmation{Password: sessionPassword})
	if err != nil {
		t.Fatal(err)
	}
	if !begun.ExpiresAt.Equal(e.clock.Now().Add(appidentity.PasskeyCeremonyTTL)) {
		t.Errorf("ExpiresAt = %v, want %v", begun.ExpiresAt, e.clock.Now().Add(appidentity.PasskeyCeremonyTTL))
	}
	phone := NewFakePasskey(userID)
	other := e.register(t, "mallory")
	if _, err := e.identity.FinishPasskeyRegistration(ctx, other.User.ID, begun.Token, phone.Register(begun.Options)); !errors.Is(err, appidentity.ErrPasskeyCeremonyExpired) {
		t.Errorf("FinishPasskeyRegistration(another user's token) = %v, want ErrPasskeyCeremonyExpired", err)
	}
	if _, err := e.identity.FinishPasskeyRegistration(ctx, userID, "garbage", phone.Register(begun.Options)); !errors.Is(err, appidentity.ErrPasskeyCeremonyExpired) {
		t.Errorf("FinishPasskeyRegistration(garbage token) = %v, want ErrPasskeyCeremonyExpired", err)
	}
	if _, err := e.identity.FinishPasskeyRegistration(ctx, userID, begun.Token, []byte(`{"challenge":"AAAA"}`)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("FinishPasskeyRegistration(another challenge) = %v, want ErrInvalidPasskey", err)
	}
	e.clock.Advance(time.Second)
	got, err := e.identity.FinishPasskeyRegistration(ctx, userID, begun.Token, phone.Register(begun.Options))
	if err != nil {
		t.Fatalf("FinishPasskeyRegistration = %v", err)
	}
	if got.Name != "phone" || got.UserID != userID || !bytes.Equal(got.CredentialID, phone.CredentialID) || !got.BackupEligible ||
		!got.CreatedAt.Equal(e.clock.Now()) {
		t.Errorf("FinishPasskeyRegistration = %+v", got)
	}

	// The second registration excludes the first credential, and the same credential is refused.
	again, err := e.identity.BeginPasskeyRegistration(ctx, userID, "laptop", appidentity.Confirmation{Password: sessionPassword})
	if err != nil {
		t.Fatal(err)
	}
	var o fakeCeremony
	if err := json.Unmarshal(again.Options, &o); err != nil || len(o.Exclude) != 1 || !bytes.Equal(o.Exclude[0], phone.CredentialID) {
		t.Errorf("options of the second registration = %s, want the first credential excluded", again.Options)
	}
	if _, err := e.identity.FinishPasskeyRegistration(ctx, userID, again.Token, phone.Register(again.Options)); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("FinishPasskeyRegistration(a registered credential) = %v, want ErrInvalidPasskey", err)
	}
	laptop := NewFakePasskey(userID)
	e.clock.Advance(appidentity.PasskeyCeremonyTTL)
	if _, err := e.identity.FinishPasskeyRegistration(ctx, userID, again.Token, laptop.Register(again.Options)); !errors.Is(err, appidentity.ErrPasskeyCeremonyExpired) {
		t.Errorf("FinishPasskeyRegistration(expired) = %v, want ErrPasskeyCeremonyExpired", err)
	}
	_, second := e.registerPasskey(t, userID, "laptop")

	list, err := e.identity.Passkeys(ctx, userID)
	if err != nil || len(list) != 2 || list[0].ID != got.ID || list[1].ID != second.ID {
		t.Fatalf("Passkeys = %+v, %v; want phone and laptop", list, err)
	}
	if list, err := e.identity.Passkeys(ctx, other.User.ID); err != nil || len(list) != 0 {
		t.Errorf("Passkeys(another user) = %+v, %v; want none", list, err)
	}
	if err := e.identity.RemovePasskey(ctx, other.User.ID, got.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("RemovePasskey(another user's) = %v, want ErrNotFound", err)
	}
	if err := e.identity.RemovePasskey(ctx, userID, got.ID); err != nil {
		t.Fatalf("RemovePasskey = %v", err)
	}
	if err := e.identity.RemovePasskey(ctx, userID, got.ID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("RemovePasskey(removed) = %v, want ErrNotFound", err)
	}
	if list, err := e.identity.Passkeys(ctx, userID); err != nil || len(list) != 1 || list[0].ID != second.ID {
		t.Errorf("Passkeys after the removal = %+v, %v; want laptop", list, err)
	}
	// A removed credential stays registered: it cannot be registered again.
	if _, err := e.h.Identity.FindPasskeyCredential(ctx, phone.CredentialID); err != nil {
		t.Errorf("FindPasskeyCredential(removed) = %v", err)
	}
	if _, err := e.h.Identity.FindActivePasskeyCredential(ctx, phone.CredentialID); !errors.Is(err, appidentity.ErrNotFound) {
		t.Errorf("FindActivePasskeyCredential(removed) = %v, want ErrNotFound", err)
	}
}

func passkeyLimitsRegistrations(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	userID := e.sessionUser(t).User.ID
	for i := range identity.MaxPasskeysPerUser {
		e.registerPasskey(t, userID, fmt.Sprintf("key %d", i))
	}
	if _, err := e.identity.BeginPasskeyRegistration(ctx, userID, "one more", appidentity.Confirmation{Password: sessionPassword}); !errors.Is(err, appidentity.ErrTooManyPasskeys) {
		t.Errorf("BeginPasskeyRegistration over the limit = %v, want ErrTooManyPasskeys", err)
	}
	// Wrong passwords count against the user: the next attempt waits even with the right one.
	other := e.register(t, "bob")
	for range appidentity.PasskeyPasswordLimit.Max {
		if _, err := e.identity.BeginPasskeyRegistration(ctx, other.User.ID, "key", appidentity.Confirmation{Password: "wrong password"}); !errors.Is(err, appidentity.ErrWrongPassword) {
			t.Fatalf("BeginPasskeyRegistration(wrong password) = %v, want ErrWrongPassword", err)
		}
	}
	var limited *kernel.RateLimitedError
	if _, err := e.identity.BeginPasskeyRegistration(ctx, other.User.ID, "key", appidentity.Confirmation{Password: "correct horse battery staple"}); !errors.As(err, &limited) {
		t.Errorf("BeginPasskeyRegistration after %d wrong passwords = %v, want a *kernel.RateLimitedError", appidentity.PasskeyPasswordLimit.Max, err)
	}
}

func passkeySignsIn(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	p, _ := e.registerPasskey(t, reg.User.ID, "phone")

	got, err := e.signInWithPasskey(t, p)
	if err != nil {
		t.Fatalf("SignInWithPasskey = %v", err)
	}
	if s, err := e.identity.Authenticate(ctx, got.Token); err != nil || s.UserID != reg.User.ID {
		t.Errorf("Authenticate(passkey session) = %+v, %v; want a session of %s", s, err, reg.User.ID)
	}
	if o, err := e.h.Identity.FindSessionOrigin(ctx, got.Session.ID); err != nil || o.IP != "192.0.2.7" || o.UserAgent != "ua" {
		t.Errorf("FindSessionOrigin = %+v, %v; want the sign-in's origin", o, err)
	}

	// A challenge works once.
	begun, err := e.identity.BeginPasskeySignIn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	in := appidentity.PasskeySignInInput{Challenge: begun.Challenge, Response: p.SignIn(begun.Options), IP: "192.0.2.7"}
	if _, err := e.identity.SignInWithPasskey(ctx, in); err != nil {
		t.Fatalf("SignInWithPasskey = %v", err)
	}
	if _, err := e.identity.SignInWithPasskey(ctx, in); !errors.Is(err, appidentity.ErrPasskeyCeremonyExpired) {
		t.Errorf("SignInWithPasskey(used challenge) = %v, want ErrPasskeyCeremonyExpired", err)
	}
	begun, err = e.identity.BeginPasskeySignIn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(appidentity.PasskeyCeremonyTTL)
	if _, err := e.identity.SignInWithPasskey(ctx, appidentity.PasskeySignInInput{Challenge: begun.Challenge, Response: p.SignIn(begun.Options)}); !errors.Is(err, appidentity.ErrPasskeyCeremonyExpired) {
		t.Errorf("SignInWithPasskey(expired challenge) = %v, want ErrPasskeyCeremonyExpired", err)
	}

	// Another user's credential under this user's handle, and an unknown credential.
	stranger := NewFakePasskey(reg.User.ID)
	if _, err := e.signInWithPasskey(t, stranger); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("SignInWithPasskey(unknown credential) = %v, want ErrInvalidPasskey", err)
	}

	// A user with TOTP still enters a code after the passkey.
	user := e.enableTOTP(t, reg)
	_, err = e.signInWithPasskey(t, p)
	var need *appidentity.SecondFactorRequiredError
	if !errors.As(err, &need) {
		t.Fatalf("SignInWithPasskey with TOTP = %v, want a *SecondFactorRequiredError", err)
	}
	if _, err := e.completeSignIn(need.Challenge, user.code(e)); err != nil {
		t.Errorf("CompleteSignIn after a passkey = %v", err)
	}
}

func passkeyRejectsRemovedAndInactive(t *testing.T, e *accountEnv) {
	ctx := t.Context()
	reg := e.sessionUser(t)
	p, c := e.registerPasskey(t, reg.User.ID, "phone")
	if err := e.identity.RemovePasskey(ctx, reg.User.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.signInWithPasskey(t, p); !errors.Is(err, appidentity.ErrInvalidPasskey) {
		t.Errorf("SignInWithPasskey(removed passkey) = %v, want ErrInvalidPasskey", err)
	}

	p, _ = e.registerPasskey(t, reg.User.ID, "laptop")
	e.clock.Advance(time.Second)
	if _, err := e.accounts.RecordLifecycleEvent(ctx, appaccount.LifecycleInput{
		AccountID: reg.AccountID, Kind: account.LifecycleDeactivated, InitiatedBy: account.InitiatedBySelf,
	}); err != nil {
		t.Fatal(err)
	}
	// A deactivated account signs in, to the server-rendered status page only (ADR-0089 §14).
	if _, err := e.signInWithPasskey(t, p); err != nil {
		t.Errorf("SignInWithPasskey(deactivated account) = %v", err)
	}
	e.clock.Advance(account.DeactivationPeriod)
	if _, err := e.signInWithPasskey(t, p); !errors.Is(err, appidentity.ErrInvalidCredentials) {
		t.Errorf("SignInWithPasskey past the date of the deletion = %v, want ErrInvalidCredentials", err)
	}
}

func passkeyRateLimitsFailedSignIns(t *testing.T, e *accountEnv) {
	reg := e.sessionUser(t)
	p, _ := e.registerPasskey(t, reg.User.ID, "phone")
	stranger := NewFakePasskey(reg.User.ID)
	for range appidentity.LoginIPLimit.Max {
		if _, err := e.signInWithPasskey(t, stranger); !errors.Is(err, appidentity.ErrInvalidPasskey) {
			t.Fatalf("SignInWithPasskey(unknown credential) = %v, want ErrInvalidPasskey", err)
		}
	}
	var limited *kernel.RateLimitedError
	if _, err := e.signInWithPasskey(t, p); !errors.As(err, &limited) {
		t.Errorf("SignInWithPasskey after %d failures = %v, want a *kernel.RateLimitedError", appidentity.LoginIPLimit.Max, err)
	}
	// Wrong passwords from the same address count against the same limit.
	if _, err := e.identity.SignIn(t.Context(), appidentity.SignInInput{Email: sessionEmail, Password: sessionPassword, IP: "192.0.2.7"}); !errors.As(err, &limited) {
		t.Errorf("SignIn from the address = %v, want a *kernel.RateLimitedError", err)
	}
}
