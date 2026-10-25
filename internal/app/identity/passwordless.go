// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Signing in without a password (FR-004). A user with a passkey can remove the password
// (RemovePassword), after which only the passkeys sign in; a reset link or a change confirmed with
// a passkey sets a password again. A user never loses the last way to sign in: the password goes
// only while a passkey stays, and the last passkey only while the password stays. Both removals
// hold the user's sign-in methods (LockSignInMethods) while they check and write, so that two of
// them at once cannot remove both.
//
// The operations that a signed-in user confirms (registering a passkey, changing or removing the
// password, deleting the account) take a Confirmation: the current password, or a passkey of the
// user, whose ceremony BeginPasskeyConfirmation starts. A confirmation with a passkey works once,
// like a sign-in, and a wrong one counts against the same limit as a wrong password.

var (
	// ErrLastSignInMethod reports a removal of the password or of a passkey that would leave the
	// user no way to sign in.
	ErrLastSignInMethod = errors.New("identity: the user would have no way to sign in")
	// ErrPasskeyConfirmationExpired reports the state of a confirmation with a passkey that is
	// malformed, expired, another user's, or used: the confirmation starts again.
	ErrPasskeyConfirmationExpired = errors.New("identity: the passkey confirmation expired")
	// ErrPasskeyRequired reports a removal of the password that is not confirmed with a passkey.
	ErrPasskeyRequired = errors.New("identity: the operation needs a passkey")
)

// PasswordRemovalLimit bounds the wrong passkeys that confirm a removal of the password, per user.
var PasswordRemovalLimit = kernel.Limit{Name: "password_removal.passkey", Max: 5, Window: time.Hour}

// SelfDeletionLimit bounds the wrong passwords and passkeys that confirm the deletion of a
// user's own account, per user.
var SelfDeletionLimit = kernel.Limit{Name: "self_deletion.confirmation", Max: 5, Window: time.Hour}

// MoveConfirmationLimit bounds the wrong passwords and passkeys that confirm the move of a
// user's own account (ADR-0089), per user.
var MoveConfirmationLimit = kernel.Limit{Name: "move.confirmation", Max: 5, Window: time.Hour}

// ConfirmMove checks that a user confirms the move of their own account (FR-009, ADR-0089) with
// the current password, or with a passkey for a user without one. It fails as confirm does,
// counting the wrong ones against MoveConfirmationLimit, and changes nothing.
func (s *Service) ConfirmMove(ctx context.Context, userID string, c Confirmation) error {
	if s.d.RateLimiter == nil {
		return errors.New("identity: ConfirmMove needs a RateLimiter")
	}
	return s.confirm(ctx, MoveConfirmationLimit, userID, c)
}

// Confirmation is how a signed-in user confirms an operation: the current password, or a passkey
// when Passkey has a token.
type Confirmation struct {
	Password string
	Passkey  PasskeyAssertion
}

// PasskeyAssertion is the browser's answer to a confirmation with a passkey.
type PasskeyAssertion struct {
	// Token is the token of BeginPasskeyConfirmation.
	Token string
	// Response is the JSON of the browser's PublicKeyCredential.
	Response []byte
}

// PasskeyConfirmation is the start of a confirmation with a passkey: the options for the browser
// and the sealed state that a Confirmation carries back with the browser's response.
type PasskeyConfirmation struct {
	Options   json.RawMessage
	Token     string
	ExpiresAt time.Time
}

func passkeyConfirmationAAD(userID string) []byte { return kernel.AAD(userID, "passkey_confirmation") }

// BeginPasskeyConfirmation starts a confirmation with a passkey for a signed-in user. It stores
// nothing.
func (s *Service) BeginPasskeyConfirmation(ctx context.Context, userID string) (PasskeyConfirmation, error) {
	if err := s.needPasskeyDeps(); err != nil {
		return PasskeyConfirmation{}, err
	}
	options, state, err := s.d.Passkeys.BeginSignIn()
	if err != nil {
		return PasskeyConfirmation{}, fmt.Errorf("identity: begin a passkey confirmation: %w", err)
	}
	now := s.d.Clock.Now()
	token, err := s.sealCeremony(ctx, ceremonyToken{IssuedAt: now.UnixMilli(), State: state}, passkeyConfirmationAAD(userID))
	if err != nil {
		return PasskeyConfirmation{}, err
	}
	return PasskeyConfirmation{Options: options, Token: token, ExpiresAt: now.Add(PasskeyCeremonyTTL)}, nil
}

// HasPassword reports whether a user has a password that signs in.
func (s *Service) HasPassword(ctx context.Context, userID string) (bool, error) {
	_, err := s.d.Repository.FindCurrentPasswordCredential(ctx, userID)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrNotFound):
		return false, nil
	}
	return false, err
}

// confirm checks the confirmation of an operation by a user, counting wrong ones against limit.
// It fails as confirmPasswordWith does for a password, and for a passkey with
// ErrPasskeyConfirmationExpired for a bad, expired, or used token, ErrInvalidPasskey for a
// response that does not verify or a passkey that is not the user's, and a
// *kernel.RateLimitedError after too many wrong ones.
func (s *Service) confirm(ctx context.Context, limit kernel.Limit, userID string, c Confirmation) error {
	if c.Passkey.Token == "" {
		return s.confirmPasswordWith(ctx, limit, userID, c.Password)
	}
	return s.confirmPasskey(ctx, limit, userID, c.Passkey)
}

func (s *Service) confirmPasskey(ctx context.Context, limit kernel.Limit, userID string, a PasskeyAssertion) error {
	if err := s.needPasskeyDeps(); err != nil {
		return err
	}
	d, err := s.d.RateLimiter.Check(ctx, limit, userID)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	t, err := s.openCeremony(ctx, a.Token, passkeyConfirmationAAD(userID))
	if errors.Is(err, ErrPasskeyCeremonyExpired) {
		return ErrPasskeyConfirmationExpired
	}
	if err != nil {
		return err
	}
	// The confirmation works once; the mark outlives the token.
	digest := sha256.Sum256([]byte(a.Token))
	n, err := s.d.KV.IncrBy(ctx, PasskeyNamespace, "used:"+hex.EncodeToString(digest[:]), 1, PasskeyCeremonyTTL+2*time.Minute)
	if err != nil {
		return fmt.Errorf("identity: record a used passkey confirmation: %w", err)
	}
	if n != 1 {
		return ErrPasskeyConfirmationExpired
	}
	_, err = s.d.Passkeys.FinishSignIn(t.State, a.Response, func(credentialID []byte) (identity.PasskeyCredential, error) {
		c, err := s.d.Repository.FindActivePasskeyCredential(ctx, credentialID)
		if err == nil && c.UserID != userID {
			return identity.PasskeyCredential{}, fmt.Errorf("%w: another user's passkey", ErrNotFound)
		}
		return c, err
	})
	if errors.Is(err, ErrInvalidPasskey) {
		if rerr := s.d.RateLimiter.Record(ctx, limit, userID); rerr != nil {
			return rerr
		}
	}
	return err
}

// RemovePasswordInput is a signed-in user's removal of their password.
type RemovePasswordInput struct {
	UserID string
	// Passkey confirms the removal: a passkey that signs in now, which stays.
	Passkey PasskeyAssertion
	// KeepFamilyID is the token family of the client that makes the removal, which keeps
	// working; "" ends every grant.
	KeepFamilyID string
}

// RemovePassword removes the password of a user who confirms it with one of their passkeys, so
// that only the passkeys sign in from then on. It ends every session of the user and every grant
// but in.KeepFamilyID (password_changed), as a change of the password does; the passkeys and the
// two-factor authentication stay. A user without a password has nothing to remove: it succeeds
// and changes nothing.
//
// It fails with ErrPasskeyRequired without a passkey, the errors of a confirmation with a passkey
// (counted against PasswordRemovalLimit), and ErrLastSignInMethod when the user has no passkey
// that signs in.
func (s *Service) RemovePassword(ctx context.Context, in RemovePasswordInput) error {
	if s.d.RateLimiter == nil {
		return errors.New("identity: RemovePassword needs a RateLimiter")
	}
	if in.Passkey.Token == "" {
		return ErrPasskeyRequired
	}
	if err := s.confirmPasskey(ctx, PasswordRemovalLimit, in.UserID, in.Passkey); err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		repo := s.d.Repository
		if err := repo.LockSignInMethods(ctx, tx, in.UserID); err != nil {
			return err
		}
		cred, err := repo.FindCurrentPasswordCredentialTx(ctx, tx, in.UserID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		passkeys, err := repo.ListActivePasskeyCredentialsTx(ctx, tx, in.UserID)
		if err != nil {
			return err
		}
		if len(passkeys) == 0 {
			return ErrLastSignInMethod
		}
		now := s.d.Clock.Now()
		if err := repo.InsertPasswordCredentialRemoval(ctx, tx, identity.PasswordCredentialRemoval{PasswordCredentialID: cred.ID, RemovedAt: now}); err != nil {
			return err
		}
		if err := s.endOtherSessions(ctx, tx, in.UserID, in.KeepFamilyID, now); err != nil {
			return err
		}
		return s.enqueuePasswordNotice(ctx, tx, in.UserID, passwordRemoved, cred.ID)
	})
}
