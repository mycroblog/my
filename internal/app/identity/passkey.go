// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Passkeys (FR-004): WebAuthn discoverable credentials that sign a user in instead of the
// password. A signed-in user registers one after confirming the password
// (BeginPasskeyRegistration, FinishPasskeyRegistration), lists and removes them; the sign-in
// page asks the browser for one (BeginPasskeySignIn) and SignInWithPasskey starts the session.
// A user without a password (RemovePassword) confirms a registration with a passkey instead, and
// cannot remove the last passkey.
//
// A passkey takes the place of the password only. A user with TOTP (FR-005) still enters a
// code after it, so that two-factor authentication keeps its promise: no sign-in without the
// code. The ceremonies require user verification (a PIN or biometrics on the authenticator).
//
// The state of a ceremony travels with the browser, sealed with kernel.Sealer and bound to its
// purpose (and, for a registration, to its user), so that showing the sign-in page stores
// nothing. A sign-in challenge works once: its use is recorded in the KeyValueStore until it
// expires. Failed sign-ins count against LoginIPLimit, and wrong confirmations of a registration
// against PasskeyPasswordLimit.

var (
	// ErrInvalidPasskey reports a registration or sign-in response that does not verify: a
	// wrong signature, origin, or challenge, an unknown or removed credential, or a credential
	// that is registered already.
	ErrInvalidPasskey = errors.New("identity: the passkey could not be verified")
	// ErrPasskeyCeremonyExpired reports the state of a ceremony that is malformed, expired,
	// another user's, or (for a sign-in) used: the ceremony starts again.
	ErrPasskeyCeremonyExpired = errors.New("identity: the passkey request expired")
	// ErrTooManyPasskeys reports a registration by a user who has identity.MaxPasskeysPerUser
	// passkeys.
	ErrTooManyPasskeys = errors.New("identity: too many passkeys")
)

// Passkey defaults.
const (
	// PasskeyCeremonyTTL is how long the browser has to answer a ceremony, from its options to
	// its response.
	PasskeyCeremonyTTL = 5 * time.Minute
	// PasskeyNamespace is the KeyValueStore namespace of used sign-in challenges.
	PasskeyNamespace = "passkey"
)

// PasskeyPasswordLimit limits the wrong passwords and passkeys that confirm a registration of a
// passkey, per user (ADR-0019 §5): adding a credential needs a confirmation, like deleting the
// account.
var PasskeyPasswordLimit = kernel.Limit{Name: "passkey.password", Max: 5, Window: time.Hour}

// PasskeyUser is the account that a registration ceremony makes a passkey for.
type PasskeyUser struct {
	// Handle is the WebAuthn user handle: the user's ID, which a sign-in returns.
	Handle []byte
	// Name is how authenticators list the passkey: the username at the instance's host.
	Name string
}

// NewPasskey is the credential of a registration that verified.
type NewPasskey struct {
	CredentialID   []byte
	PublicKey      []byte
	BackupEligible bool
}

// PasskeyCeremonies is the relying party's side of the WebAuthn ceremonies (W3C Web
// Authentication Level 3, §7). The options it returns are the JSON that the browser's
// PublicKeyCredential.parseCreationOptionsFromJSON and parseRequestOptionsFromJSON read, the
// responses it takes the JSON of PublicKeyCredential.toJSON, and the state the opaque record of
// a ceremony that the service keeps sealed until the response comes. The webauthn adapter
// implements it and passes testkit.PasskeyCeremoniesContract.
type PasskeyCeremonies interface {
	// BeginRegistration returns the options of a registration of a discoverable credential for
	// user that is none of exclude, and its state.
	BeginRegistration(user PasskeyUser, exclude [][]byte) (options, state []byte, err error)
	// FinishRegistration verifies the response of a registration against its state. It fails
	// with an error wrapping ErrInvalidPasskey when the response does not verify.
	FinishRegistration(user PasskeyUser, state, response []byte) (NewPasskey, error)
	// BeginSignIn returns the options of a sign-in with a discoverable credential, and its
	// state.
	BeginSignIn() (options, state []byte, err error)
	// FinishSignIn verifies the response of a sign-in against its state and the credential that
	// find returns for the response's credential ID, and returns that credential. It fails with
	// an error wrapping ErrInvalidPasskey when the response does not verify or find reports
	// ErrNotFound, and with the error of find otherwise.
	FinishSignIn(state, response []byte, find func(credentialID []byte) (identity.PasskeyCredential, error)) (identity.PasskeyCredential, error)
}

// PasskeysEnabled reports whether the service can run passkeys (Deps.Passkeys and the deps of
// two-factor authentication are set).
func (s *Service) PasskeysEnabled() bool { return s.needPasskeyDeps() == nil }

func (s *Service) needPasskeyDeps() error {
	if s.d.Passkeys == nil {
		return errors.New("identity: passkeys need PasskeyCeremonies")
	}
	return s.needTwoFactorDeps()
}

// Passkeys returns the passkeys of a user that sign in, oldest first.
func (s *Service) Passkeys(ctx context.Context, userID string) ([]identity.PasskeyCredential, error) {
	return s.d.Repository.ListActivePasskeyCredentials(ctx, userID)
}

// PasskeyRegistration is the start of a registration: the options for the browser and the
// sealed state that FinishPasskeyRegistration takes with the browser's response.
type PasskeyRegistration struct {
	Options   json.RawMessage
	Token     string
	ExpiresAt time.Time
}

// ceremonyToken is what the token of a ceremony seals.
type ceremonyToken struct {
	IssuedAt int64  `json:"iat"` // Unix milliseconds
	Name     string `json:"name,omitempty"`
	State    []byte `json:"state"`
}

func passkeyRegistrationAAD(userID string) []byte { return kernel.AAD(userID, "passkey_registration") }

// passkeySignInAAD binds a sign-in challenge to its purpose; it has no user yet.
var passkeySignInAAD = kernel.AAD("passkey_sign_in")

func (s *Service) sealCeremony(ctx context.Context, t ceremonyToken, aad []byte) (string, error) {
	plain, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	sealed, err := s.d.Sealer.Seal(ctx, plain, aad)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *Service) openCeremony(ctx context.Context, token string, aad []byte) (ceremonyToken, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || token == "" {
		return ceremonyToken{}, ErrPasskeyCeremonyExpired
	}
	plain, err := s.d.Sealer.Open(ctx, sealed, aad)
	if errors.Is(err, kernel.ErrDecrypt) || errors.Is(err, kernel.ErrUnknownKey) {
		return ceremonyToken{}, ErrPasskeyCeremonyExpired
	}
	if err != nil {
		return ceremonyToken{}, err
	}
	var t ceremonyToken
	if err := json.Unmarshal(plain, &t); err != nil {
		return ceremonyToken{}, ErrPasskeyCeremonyExpired
	}
	issued := time.UnixMilli(t.IssuedAt)
	now := s.d.Clock.Now()
	if !now.Before(issued.Add(PasskeyCeremonyTTL)) || now.Before(issued.Add(-time.Minute)) {
		return ceremonyToken{}, ErrPasskeyCeremonyExpired
	}
	return t, nil
}

// BeginPasskeyRegistration starts the registration of a passkey called name for a user who
// confirms it with the current password or a passkey. It fails with an error wrapping
// identity.ErrInvalidPasskeyName for a bad name, ErrWrongPassword for a wrong password, the
// errors of a confirmation with a passkey (Service.confirm), a *kernel.RateLimitedError after too
// many wrong confirmations (PasskeyPasswordLimit), ErrTooManyPasskeys when the user has as many
// passkeys as allowed, and ErrNotFound when the user has no local account.
func (s *Service) BeginPasskeyRegistration(ctx context.Context, userID, name string, c Confirmation) (PasskeyRegistration, error) {
	if err := s.needPasskeyDeps(); err != nil {
		return PasskeyRegistration{}, err
	}
	name, err := identity.NormalizePasskeyName(name)
	if err != nil {
		return PasskeyRegistration{}, err
	}
	if err := s.confirm(ctx, OperationPasskeyRegistration, userID, c); err != nil {
		return PasskeyRegistration{}, err
	}
	existing, err := s.d.Repository.ListActivePasskeyCredentials(ctx, userID)
	if err != nil {
		return PasskeyRegistration{}, err
	}
	if len(existing) >= identity.MaxPasskeysPerUser {
		return PasskeyRegistration{}, ErrTooManyPasskeys
	}
	user, err := s.passkeyUser(ctx, userID)
	if err != nil {
		return PasskeyRegistration{}, err
	}
	exclude := make([][]byte, len(existing))
	for i, c := range existing {
		exclude[i] = c.CredentialID
	}
	options, state, err := s.d.Passkeys.BeginRegistration(user, exclude)
	if err != nil {
		return PasskeyRegistration{}, fmt.Errorf("identity: begin a passkey registration: %w", err)
	}
	now := s.d.Clock.Now()
	token, err := s.sealCeremony(ctx, ceremonyToken{IssuedAt: now.UnixMilli(), Name: name, State: state}, passkeyRegistrationAAD(userID))
	if err != nil {
		return PasskeyRegistration{}, err
	}
	return PasskeyRegistration{Options: options, Token: token, ExpiresAt: now.Add(PasskeyCeremonyTTL)}, nil
}

// confirmPasswordWith checks the current password of a user who confirms an operation with it,
// counting wrong ones against limit. It fails with ErrWrongPassword for a wrong password, also
// for a user without one, and with a *kernel.RateLimitedError after too many.
func (s *Service) confirmPasswordWith(ctx context.Context, limit kernel.Limit, userID, password string) error {
	d, err := s.d.RateLimiter.Check(ctx, limit, userID)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	cred, err := s.d.Repository.FindCurrentPasswordCredential(ctx, userID)
	ok := false
	switch {
	case err == nil:
		ok, _, err = s.d.Passwords.Verify(ctx, password, cred.Hash)
		if err != nil {
			return fmt.Errorf("identity: verify the password of user %s: %w", userID, err)
		}
	case !errors.Is(err, ErrNotFound):
		return err
	}
	if !ok {
		if err := s.d.RateLimiter.Record(ctx, limit, userID); err != nil {
			return err
		}
		return ErrWrongPassword
	}
	return nil
}

func (s *Service) passkeyUser(ctx context.Context, userID string) (PasskeyUser, error) {
	acct, err := s.d.Accounts.LocalAccountOfUser(ctx, userID)
	if err != nil {
		return PasskeyUser{}, fmt.Errorf("%w: the local account of user %s: %w", ErrNotFound, userID, err)
	}
	return PasskeyUser{Handle: []byte(userID), Name: acct.Username + "@" + s.issuer()}, nil
}

// FinishPasskeyRegistration verifies the browser's response to a registration that
// BeginPasskeyRegistration started for the user and stores the passkey. It fails with
// ErrPasskeyCeremonyExpired for a bad or expired token, ErrInvalidPasskey for a response that
// does not verify or a credential that is registered already, and ErrTooManyPasskeys.
func (s *Service) FinishPasskeyRegistration(ctx context.Context, userID, token string, response []byte) (identity.PasskeyCredential, error) {
	if err := s.needPasskeyDeps(); err != nil {
		return identity.PasskeyCredential{}, err
	}
	t, err := s.openCeremony(ctx, token, passkeyRegistrationAAD(userID))
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	name, err := identity.NormalizePasskeyName(t.Name)
	if err != nil {
		return identity.PasskeyCredential{}, ErrPasskeyCeremonyExpired
	}
	existing, err := s.d.Repository.ListActivePasskeyCredentials(ctx, userID)
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	if len(existing) >= identity.MaxPasskeysPerUser {
		return identity.PasskeyCredential{}, ErrTooManyPasskeys
	}
	user, err := s.passkeyUser(ctx, userID)
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	created, err := s.d.Passkeys.FinishRegistration(user, t.State, response)
	if err != nil {
		return identity.PasskeyCredential{}, err
	}
	if len(created.CredentialID) == 0 || len(created.CredentialID) > identity.MaxCredentialIDLength {
		return identity.PasskeyCredential{}, fmt.Errorf("%w: a credential ID of %d bytes", ErrInvalidPasskey, len(created.CredentialID))
	}
	switch _, err := s.d.Repository.FindPasskeyCredential(ctx, created.CredentialID); {
	case err == nil:
		return identity.PasskeyCredential{}, fmt.Errorf("%w: the credential is registered already", ErrInvalidPasskey)
	case !errors.Is(err, ErrNotFound):
		return identity.PasskeyCredential{}, err
	}
	c := identity.PasskeyCredential{
		ID: s.d.IDs.NewID(), UserID: userID, CredentialID: created.CredentialID, PublicKey: created.PublicKey,
		BackupEligible: created.BackupEligible, Name: name, CreatedAt: s.d.Clock.Now(),
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.d.Repository.InsertPasskeyCredential(ctx, tx, c) }); err != nil {
		return identity.PasskeyCredential{}, err
	}
	return c, nil
}

// RemovePasskey ends a passkey of a user, which then no longer signs in. It fails with
// ErrNotFound when the user has no such passkey, and with ErrLastSignInMethod for the last passkey
// of a user without a password or a linked identity.
func (s *Service) RemovePasskey(ctx context.Context, userID, passkeyID string) error {
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.LockSignInMethods(ctx, tx, userID); err != nil {
			return err
		}
		existing, err := repo.ListActivePasskeyCredentialsTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		found := false
		for _, c := range existing {
			found = found || c.ID == passkeyID
		}
		if !found {
			return ErrNotFound
		}
		if len(existing) == 1 {
			switch _, err := repo.FindCurrentPasswordCredentialTx(ctx, tx, userID); {
			case errors.Is(err, ErrNotFound):
				switch links, err := s.signInLinks(ctx, tx, userID); {
				case err != nil:
					return err
				case len(links) == 0:
					return ErrLastSignInMethod
				}
			case err != nil:
				return err
			}
		}
		rev := identity.PasskeyCredentialRevocation{PasskeyID: passkeyID, RevokedAt: s.d.Clock.Now()}
		return repo.InsertPasskeyCredentialRevocation(ctx, tx, rev)
	})
}

// PasskeySignIn is the start of a sign-in with a passkey: the options for the browser and the
// sealed challenge that SignInWithPasskey takes with the browser's response.
type PasskeySignIn struct {
	Options   json.RawMessage
	Challenge string
}

// BeginPasskeySignIn starts a sign-in with a passkey. It stores nothing, so the sign-in page
// can offer one on every view.
func (s *Service) BeginPasskeySignIn(ctx context.Context) (PasskeySignIn, error) {
	if err := s.needPasskeyDeps(); err != nil {
		return PasskeySignIn{}, err
	}
	options, state, err := s.d.Passkeys.BeginSignIn()
	if err != nil {
		return PasskeySignIn{}, fmt.Errorf("identity: begin a passkey sign-in: %w", err)
	}
	challenge, err := s.sealCeremony(ctx, ceremonyToken{IssuedAt: s.d.Clock.Now().UnixMilli(), State: state}, passkeySignInAAD)
	if err != nil {
		return PasskeySignIn{}, err
	}
	return PasskeySignIn{Options: options, Challenge: challenge}, nil
}

// PasskeySignInInput is the browser's answer to a sign-in with a passkey.
type PasskeySignInInput struct {
	// Challenge is that of BeginPasskeySignIn.
	Challenge string
	// Response is the JSON of the browser's PublicKeyCredential.
	Response []byte
	// IP, UserAgent, and CurrentToken are as in SignInInput.
	IP           string
	UserAgent    string
	CurrentToken string
}

// SignInWithPasskey verifies the response to a sign-in with a passkey and starts the session of
// the passkey's user, as SignIn does after a right password: a user with TOTP gets a
// *SecondFactorRequiredError whose challenge CompleteSignIn takes with the code.
//
// It fails with ErrPasskeyCeremonyExpired when the challenge is malformed, expired, or used;
// with ErrInvalidPasskey when the response does not verify or its passkey is unknown or
// removed; with ErrInvalidCredentials when the user's account is not active; and with a
// *kernel.RateLimitedError when too many sign-ins failed from the client (LoginIPLimit). A
// response that does not verify and an inactive account count against that limit.
func (s *Service) SignInWithPasskey(ctx context.Context, in PasskeySignInInput) (SignedIn, error) {
	if err := s.needPasskeyDeps(); err != nil {
		return SignedIn{}, err
	}
	ipKey := kernel.ClientKey(in.IP)
	if err := s.checkLoginLimits(ctx, ipKey, ""); err != nil {
		return SignedIn{}, err
	}
	t, err := s.openCeremony(ctx, in.Challenge, passkeySignInAAD)
	if err != nil {
		return SignedIn{}, err
	}
	// The challenge works once; the mark outlives the challenge.
	digest := sha256.Sum256([]byte(in.Challenge))
	n, err := s.d.KV.IncrBy(ctx, PasskeyNamespace, "used:"+hex.EncodeToString(digest[:]), 1, PasskeyCeremonyTTL+2*time.Minute)
	if err != nil {
		return SignedIn{}, fmt.Errorf("identity: record a used passkey challenge: %w", err)
	}
	if n != 1 {
		return SignedIn{}, ErrPasskeyCeremonyExpired
	}
	cred, err := s.d.Passkeys.FinishSignIn(t.State, in.Response, func(credentialID []byte) (identity.PasskeyCredential, error) {
		return s.d.Repository.FindActivePasskeyCredential(ctx, credentialID)
	})
	if err == nil {
		err = s.checkActive(ctx, cred.UserID)
	}
	if errors.Is(err, ErrInvalidPasskey) || errors.Is(err, ErrInvalidCredentials) {
		if rerr := s.recordLoginFailure(ctx, ipKey, ""); rerr != nil {
			return SignedIn{}, rerr
		}
		return SignedIn{}, err
	}
	if err != nil {
		return SignedIn{}, err
	}
	switch _, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, cred.UserID); {
	case err == nil:
		challenge, err := s.newChallenge(ctx, cred.UserID)
		if err != nil {
			return SignedIn{}, err
		}
		return SignedIn{}, &SecondFactorRequiredError{Challenge: challenge}
	case !errors.Is(err, ErrNotFound):
		return SignedIn{}, err
	}
	session, err := s.prepareSession(ctx, cred.UserID, in.IP, in.UserAgent, in.CurrentToken, s.d.Clock.Now())
	if err != nil {
		return SignedIn{}, err
	}
	if err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return s.writeSession(ctx, tx, session) }); err != nil {
		return SignedIn{}, err
	}
	return session.signedIn(), nil
}
