// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// DisableTOTP ends the TOTP enrollment of a user who confirms it with a code or a recovery code.
// It fails with ErrTwoFactorNotEnabled, ErrInvalidSecondFactor, or a *kernel.RateLimitedError.
func (s *Service) DisableTOTP(ctx context.Context, userID, code string) error {
	f, now, err := s.confirmSecondFactor(ctx, userID, code)
	if err != nil {
		return err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.useRecoveryCode(ctx, tx, f, now); err != nil {
			return err
		}
		return s.d.Repository.InsertTOTPEnrollmentRevocation(ctx, tx, identity.TOTPEnrollmentRevocation{EnrollmentID: f.enrollment.ID, RevokedAt: now})
	})
	return s.afterSecondFactorTx(ctx, userID, "", err)
}

// RegenerateRecoveryCodes replaces the recovery codes of a user who confirms it with a code or
// a recovery code, and returns the new ones. The authenticator app keeps working: the secret
// moves to a new enrollment, whose codes replace those of the old one. It fails like DisableTOTP.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, code string) ([]string, error) {
	f, now, err := s.confirmSecondFactor(ctx, userID, code)
	if err != nil {
		return nil, err
	}
	e, codes, hashes, err := s.newEnrollment(ctx, userID, f.secret, now)
	if err != nil {
		return nil, err
	}
	if f.recoveryHash == nil {
		// The code stays used under the new enrollment. It is marked before the new codes
		// replace the old ones, so that a failure here leaves the old codes working.
		if _, err := s.useStep(ctx, e.ID, f.step); err != nil {
			return nil, err
		}
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.useRecoveryCode(ctx, tx, f, now); err != nil {
			return err
		}
		rev := identity.TOTPEnrollmentRevocation{EnrollmentID: f.enrollment.ID, RevokedAt: now}
		if err := s.d.Repository.InsertTOTPEnrollmentRevocation(ctx, tx, rev); err != nil {
			return err
		}
		return s.d.Repository.InsertTOTPEnrollment(ctx, tx, e, hashes)
	})
	if err := s.afterSecondFactorTx(ctx, userID, "", err); err != nil {
		return nil, err
	}
	return codes, nil
}

// confirmSecondFactor checks the rate limit and then code against the active enrollment of a
// user, for an operation in the settings.
func (s *Service) confirmSecondFactor(ctx context.Context, userID, code string) (secondFactor, time.Time, error) {
	if err := s.needTwoFactorDeps(); err != nil {
		return secondFactor{}, time.Time{}, err
	}
	if err := s.checkSecondFactorLimit(ctx, userID); err != nil {
		return secondFactor{}, time.Time{}, err
	}
	e, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return secondFactor{}, time.Time{}, ErrTwoFactorNotEnabled
	}
	if err != nil {
		return secondFactor{}, time.Time{}, err
	}
	now := s.d.Clock.Now()
	f, err := s.verifySecondFactor(ctx, e, code, now)
	if errors.Is(err, ErrInvalidSecondFactor) {
		return secondFactor{}, time.Time{}, s.secondFactorFailed(ctx, userID, "")
	}
	return f, now, err
}

// secondFactor is a code that matched an enrollment.
type secondFactor struct {
	enrollment identity.TOTPEnrollment
	secret     []byte
	// step is the TOTP step that a code matched, which verifySecondFactor marked used.
	step int64
	// recoveryHash is the digest of a recovery code, which useRecoveryCode spends in the
	// transaction of the operation; nil for a TOTP code.
	recoveryHash []byte
}

// verifySecondFactor checks code against enrollment e: a TOTP code at a step not used before,
// or something shaped like a recovery code, which only useRecoveryCode can tell is valid. It
// fails with ErrInvalidSecondFactor.
func (s *Service) verifySecondFactor(ctx context.Context, e identity.TOTPEnrollment, code string, now time.Time) (secondFactor, error) {
	secret, err := s.d.Sealer.Open(ctx, e.SecretEnc, TOTPSecretAAD(e.ID))
	if err != nil {
		return secondFactor{}, fmt.Errorf("identity: open the TOTP secret of enrollment %s: %w", e.ID, err)
	}
	f := secondFactor{enrollment: e, secret: secret}
	if normalized := identity.NormalizeTOTPCode(code); identity.IsTOTPCode(normalized) {
		step, ok := identity.MatchTOTP(secret, normalized, now)
		if !ok {
			return secondFactor{}, ErrInvalidSecondFactor
		}
		first, err := s.useStep(ctx, e.ID, step)
		if err != nil {
			return secondFactor{}, err
		}
		if !first {
			return secondFactor{}, ErrInvalidSecondFactor
		}
		f.step = step
		return f, nil
	}
	rc := identity.NormalizeRecoveryCode(code)
	if rc == "" {
		return secondFactor{}, ErrInvalidSecondFactor
	}
	f.recoveryHash = identity.HashRecoveryCode(rc)
	return f, nil
}

// useStep marks a TOTP step of an enrollment used and reports whether it was the first use.
// The mark outlives every time at which the step can match.
func (s *Service) useStep(ctx context.Context, enrollmentID string, step int64) (bool, error) {
	ttl := time.Duration(2*identity.TOTPSkew+2) * identity.TOTPPeriod
	n, err := s.d.KV.IncrBy(ctx, TwoFactorNamespace, "step:"+enrollmentID+":"+strconv.FormatInt(step, 10), 1, ttl)
	if err != nil {
		return false, fmt.Errorf("identity: record a used TOTP step: %w", err)
	}
	return n == 1, nil
}

// useRecoveryCode spends the recovery code of f in tx, failing with ErrInvalidSecondFactor when
// the enrollment has no such unused code. It does nothing for a TOTP code.
func (s *Service) useRecoveryCode(ctx context.Context, tx uow.Tx, f secondFactor, now time.Time) error {
	if f.recoveryHash == nil {
		return nil
	}
	ok, err := s.d.Repository.UseTOTPRecoveryCode(ctx, tx, identity.TOTPRecoveryCodeUse{
		EnrollmentID: f.enrollment.ID, CodeHash: f.recoveryHash, UsedAt: now,
	})
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidSecondFactor
	}
	return nil
}

// afterSecondFactorTx counts a failure when the transaction of an operation found its recovery
// code invalid.
func (s *Service) afterSecondFactorTx(ctx context.Context, userID, ipKey string, err error) error {
	if errors.Is(err, ErrInvalidSecondFactor) {
		return s.secondFactorFailed(ctx, userID, ipKey)
	}
	return err
}

func (s *Service) checkSecondFactorLimit(ctx context.Context, userID string) error {
	d, err := s.d.RateLimiter.Check(ctx, SecondFactorLimit, userID)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
	}
	return nil
}

// secondFactorFailed counts a wrong code against the user and, when ipKey is set, the client,
// and returns ErrInvalidSecondFactor.
func (s *Service) secondFactorFailed(ctx context.Context, userID, ipKey string) error {
	if err := s.d.RateLimiter.Record(ctx, SecondFactorLimit, userID); err != nil {
		return err
	}
	if ipKey != "" {
		if err := s.d.RateLimiter.Record(ctx, LoginIPLimit, ipKey); err != nil {
			return err
		}
	}
	return ErrInvalidSecondFactor
}

func (s *Service) needTwoFactorDeps() error {
	if s.d.Sealer == nil || s.d.KV == nil || s.d.RateLimiter == nil {
		return errors.New("identity: two-factor authentication needs a Sealer, a KeyValueStore, and a RateLimiter")
	}
	return nil
}

// newChallenge stores a sign-in challenge of userID and returns it. The challenge keeps when it
// was issued, so that a later change of the password makes it stop working.
func (s *Service) newChallenge(ctx context.Context, userID string) (string, error) {
	if err := s.needTwoFactorDeps(); err != nil {
		return "", err
	}
	token, hash := NewToken(SecondFactorChallengePrefix)
	v := userID + challengeSeparator + strconv.FormatInt(s.d.Clock.Now().UnixNano(), 10)
	if err := s.d.KV.Set(ctx, TwoFactorNamespace, challengeKey(hash), []byte(v), SecondFactorChallengeTTL); err != nil {
		return "", fmt.Errorf("identity: store a sign-in challenge: %w", err)
	}
	return token, nil
}

func challengeKey(hash []byte) string { return "challenge:" + hex.EncodeToString(hash) }

// challengeSeparator separates the user ID of a stored challenge from when it was issued.
const challengeSeparator = " "

// parseChallenge returns the user and the time of issue of a stored challenge; ok is false for a
// malformed value.
func parseChallenge(v []byte) (userID string, issuedAt time.Time, ok bool) {
	userID, nanos, found := strings.Cut(string(v), challengeSeparator)
	if !found || userID == "" {
		return "", time.Time{}, false
	}
	n, err := strconv.ParseInt(nanos, 10, 64)
	if err != nil {
		return "", time.Time{}, false
	}
	return userID, time.Unix(0, n), true
}

// SecondFactorInput is the second step of a sign-in.
type SecondFactorInput struct {
	// Challenge is that of the *SecondFactorRequiredError of the first step.
	Challenge string
	// Code is a TOTP code or a recovery code.
	Code string
	// IP, UserAgent, and CurrentToken are as in SignInInput.
	IP           string
	UserAgent    string
	CurrentToken string
}

// CompleteSignIn takes the second factor of a sign-in and starts its session, as SignIn does
// for a user without two-factor authentication. A recovery code is spent in the transaction
// that starts the session.
//
// It fails with ErrSecondFactorExpired when the challenge is unknown, used, or expired, the
// password of the user changed after it was issued, or the user no longer has TOTP; with ErrInvalidSecondFactor for a wrong code; with
// ErrInvalidCredentials when the account stopped being active; and with a
// *kernel.RateLimitedError when too many codes failed for the user (SecondFactorLimit) or
// sign-ins failed from the client (LoginIPLimit). A wrong code counts against both.
func (s *Service) CompleteSignIn(ctx context.Context, in SecondFactorInput) (SignedIn, error) {
	if err := s.needTwoFactorDeps(); err != nil {
		return SignedIn{}, err
	}
	ipKey := kernel.ClientKey(in.IP)
	if err := s.checkLoginLimits(ctx, ipKey, ""); err != nil {
		return SignedIn{}, err
	}
	if in.Challenge == "" {
		return SignedIn{}, ErrSecondFactorExpired
	}
	key := challengeKey(HashToken(in.Challenge))
	v, err := s.d.KV.Get(ctx, TwoFactorNamespace, key)
	if errors.Is(err, kernel.ErrKeyNotFound) {
		return SignedIn{}, ErrSecondFactorExpired
	}
	if err != nil {
		return SignedIn{}, err
	}
	userID, issuedAt, ok := parseChallenge(v)
	if !ok {
		s.dropChallenge(ctx, key)
		return SignedIn{}, ErrSecondFactorExpired
	}
	if err := s.checkSecondFactorLimit(ctx, userID); err != nil {
		return SignedIn{}, err
	}
	if err := s.checkActive(ctx, userID); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			s.dropChallenge(ctx, key)
		}
		return SignedIn{}, err
	}
	// The first step checked the password of then; a reset or change since ends the challenge.
	switch changed, err := s.d.Repository.LastPasswordChange(ctx, userID); {
	case err == nil:
		if changed.After(issuedAt) {
			s.dropChallenge(ctx, key)
			return SignedIn{}, ErrSecondFactorExpired
		}
	case !errors.Is(err, ErrNotFound):
		return SignedIn{}, err
	}
	e, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		s.dropChallenge(ctx, key)
		return SignedIn{}, ErrSecondFactorExpired
	}
	if err != nil {
		return SignedIn{}, err
	}
	now := s.d.Clock.Now()
	f, err := s.verifySecondFactor(ctx, e, in.Code, now)
	if errors.Is(err, ErrInvalidSecondFactor) {
		return SignedIn{}, s.secondFactorFailed(ctx, userID, ipKey)
	}
	if err != nil {
		return SignedIn{}, err
	}
	session, err := s.prepareSession(ctx, userID, in.IP, in.UserAgent, in.CurrentToken, now)
	if err != nil {
		return SignedIn{}, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.useRecoveryCode(ctx, tx, f, now); err != nil {
			return err
		}
		return s.writeSession(ctx, tx, session)
	})
	if err := s.afterSecondFactorTx(ctx, userID, ipKey, err); err != nil {
		return SignedIn{}, err
	}
	s.dropChallenge(ctx, key)
	return session.signedIn(), nil
}

// dropChallenge deletes a challenge that has served its purpose. A failure only leaves it to
// expire.
func (s *Service) dropChallenge(ctx context.Context, key string) {
	if err := s.d.KV.Delete(ctx, TwoFactorNamespace, key); err != nil {
		s.logger().WarnContext(ctx, "delete a sign-in challenge", slog.Any("error", err))
	}
}

// DisableTwoFactor removes the two-factor authentication of a local user's account for
// byUserID ("" for the CLI), for a user who lost their authenticator and recovery codes, and
// records it in the audit log (two_factor_disabled) in the same transaction. It fails with
// account.ErrNotFound for an unknown account, ErrNotLocalAccount for an account without a user,
// and ErrTwoFactorNotEnabled when the user has none.
func (s *AccountAdmin) DisableTwoFactor(ctx context.Context, accountID, byUserID string) error {
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if actor.Local == nil || actor.Local.UserID == "" {
		return ErrNotLocalAccount
	}
	e, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, actor.Local.UserID)
	if errors.Is(err, ErrNotFound) {
		return ErrTwoFactorNotEnabled
	}
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	actorKind, actorUser := appmoderation.AuditActor(byUserID)
	audit := appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: actorKind, ActorUserID: actorUser, Action: appmoderation.AuditTwoFactorDisabled,
		TargetKind: appmoderation.AuditTargetAccount, TargetID: accountID, TargetLabel: "@" + actor.Local.Username,
		OccurredAt: now,
	}}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		rev := identity.TOTPEnrollmentRevocation{EnrollmentID: e.ID, RevokedAt: now}
		if err := s.d.Repository.InsertTOTPEnrollmentRevocation(ctx, tx, rev); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
}
