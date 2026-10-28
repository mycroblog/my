// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Two-factor authentication with TOTP (FR-005). A user enrolls an authenticator app by scanning
// a secret that the server generated (BeginTOTPSetup) and typing its first code (EnableTOTP),
// and gets RecoveryCodeCount recovery codes, each of which works once in place of a code. Once
// enrolled, SignIn stops after the password and returns a *SecondFactorRequiredError whose
// challenge CompleteSignIn takes with a code or a recovery code to start the session.
//
// The secret is sealed with kernel.Sealer (ADR-0019 §6) bound to the enrollment's ID; recovery
// codes are stored as SHA-256 digests. A TOTP code is accepted once: the step it matched is
// recorded in the KeyValueStore until the code can no longer match. Failed codes count against
// SecondFactorLimit for the user and, on sign-in, LoginIPLimit for the client.

var (
	// ErrSecondFactorRequired matches a *SecondFactorRequiredError.
	ErrSecondFactorRequired = errors.New("identity: a second factor is required")
	// ErrInvalidSecondFactor reports a wrong, reused, or malformed code or recovery code.
	ErrInvalidSecondFactor = errors.New("identity: invalid authentication code")
	// ErrSecondFactorExpired reports a sign-in challenge that is unknown, used, or expired, or
	// whose user no longer has two-factor authentication: the sign-in starts again.
	ErrSecondFactorExpired = errors.New("identity: the sign-in expired")
	// ErrTwoFactorEnabled reports a setup of TOTP for a user who has it enabled.
	ErrTwoFactorEnabled = errors.New("identity: two-factor authentication is already enabled")
	// ErrTwoFactorNotEnabled reports an operation that needs TOTP on a user without it.
	ErrTwoFactorNotEnabled = errors.New("identity: two-factor authentication is not enabled")
	// ErrInvalidTOTPSetup reports a setup token that is malformed, another user's, or expired.
	ErrInvalidTOTPSetup = errors.New("identity: invalid or expired TOTP setup")
)

// SecondFactorRequiredError reports a sign-in whose password was right for a user who has
// two-factor authentication: no session started yet. Challenge identifies the sign-in to
// CompleteSignIn; it is a secret like a session token and expires after SecondFactorChallengeTTL.
type SecondFactorRequiredError struct {
	Challenge string
}

func (e *SecondFactorRequiredError) Error() string { return ErrSecondFactorRequired.Error() }

// Is makes errors.Is(err, ErrSecondFactorRequired) hold.
func (e *SecondFactorRequiredError) Is(target error) bool { return target == ErrSecondFactorRequired }

// Two-factor defaults.
const (
	// SecondFactorChallengeTTL is how long the second step of a sign-in may take.
	SecondFactorChallengeTTL = 5 * time.Minute
	// SecondFactorChallengePrefix marks sign-in challenges for secret scanners (ADR-0019 §4).
	SecondFactorChallengePrefix = "my_2fa_"
	// TOTPSetupTTL is how long a setup token from BeginTOTPSetup can be confirmed.
	TOTPSetupTTL = 15 * time.Minute
	// TwoFactorNamespace is the KeyValueStore namespace of sign-in challenges and used steps.
	TwoFactorNamespace = "two_factor"
)

// SecondFactorLimit limits the failed codes of a user, on sign-in and in the settings: with
// three steps accepted at a time, 10 guesses an hour leave a six-digit code a chance in 30,000
// a day (ADR-0019 §5).
var SecondFactorLimit = kernel.Limit{Name: "login.second_factor", Max: 10, Window: time.Hour}

// totpSetupAAD binds a setup token to its user.
func totpSetupAAD(userID string) []byte { return kernel.AAD(userID, "totp_setup") }

// TOTPSecretAAD binds a sealed secret to its enrollment (ADR-0019 §6).
func TOTPSecretAAD(enrollmentID string) []byte {
	return kernel.AAD(enrollmentID, identity.TOTPSecretColumn)
}

// TwoFactorStatus is where a user's two-factor authentication stands.
type TwoFactorStatus struct {
	Enabled bool
	// EnrolledAt is when the active enrollment was made; zero when not Enabled.
	EnrolledAt time.Time
	// RecoveryCodesRemaining is how many recovery codes are unused.
	RecoveryCodesRemaining int
}

// TwoFactorStatus returns where the two-factor authentication of a user stands.
func (s *Service) TwoFactorStatus(ctx context.Context, userID string) (TwoFactorStatus, error) {
	e, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return TwoFactorStatus{}, nil
	}
	if err != nil {
		return TwoFactorStatus{}, err
	}
	n, err := s.d.Repository.CountUnusedTOTPRecoveryCodes(ctx, e.ID)
	if err != nil {
		return TwoFactorStatus{}, err
	}
	return TwoFactorStatus{Enabled: true, EnrolledAt: e.EnrolledAt, RecoveryCodesRemaining: n}, nil
}

// TOTPSetup is a new secret for a user to add to an authenticator app.
type TOTPSetup struct {
	// Secret is the secret in base32, for typing it in.
	Secret string
	// URI is the otpauth:// URI of the secret, for a QR code.
	URI string
	// Token carries the secret, sealed, to EnableTOTP; nothing is stored until then.
	Token     string
	ExpiresAt time.Time
}

// BeginTOTPSetup generates a secret for a user to enroll. It fails with ErrTwoFactorEnabled
// when the user has TOTP enabled, and with ErrNotFound when the user has no local account.
func (s *Service) BeginTOTPSetup(ctx context.Context, userID string) (TOTPSetup, error) {
	if err := s.needTwoFactorDeps(); err != nil {
		return TOTPSetup{}, err
	}
	switch _, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, userID); {
	case err == nil:
		return TOTPSetup{}, ErrTwoFactorEnabled
	case !errors.Is(err, ErrNotFound):
		return TOTPSetup{}, err
	}
	acct, err := s.d.Accounts.LocalAccountOfUser(ctx, userID)
	if err != nil {
		return TOTPSetup{}, fmt.Errorf("%w: the local account of user %s: %w", ErrNotFound, userID, err)
	}
	secret := make([]byte, identity.TOTPSecretSize)
	_, _ = rand.Read(secret) // crypto/rand.Read never returns an error
	now := s.d.Clock.Now()
	plain := binary.BigEndian.AppendUint64(nil, uint64(now.UnixMilli()))
	plain = append(plain, secret...)
	sealed, err := s.d.Sealer.Seal(ctx, plain, totpSetupAAD(userID))
	if err != nil {
		return TOTPSetup{}, err
	}
	issuer := s.issuer()
	encoded := identity.EncodeTOTPSecret(secret)
	q := url.Values{}
	q.Set("secret", encoded)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", strconv.Itoa(identity.TOTPDigits))
	q.Set("period", strconv.Itoa(int(identity.TOTPPeriod/time.Second)))
	uri := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + issuer + ":" + acct.Username + "@" + issuer, RawQuery: q.Encode()}
	return TOTPSetup{
		Secret:    encoded,
		URI:       uri.String(),
		Token:     base64.RawURLEncoding.EncodeToString(sealed),
		ExpiresAt: now.Add(TOTPSetupTTL),
	}, nil
}

// issuer names the instance in authenticator apps: the host of BaseURL.
func (s *Service) issuer() string {
	if u, err := url.Parse(s.d.BaseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "my"
}

// EnableTOTP enrolls the secret of a setup token once code shows that the user's app has it,
// and returns the new recovery codes, which are never shown again. It fails with
// ErrInvalidTOTPSetup for a bad or expired token, ErrInvalidSecondFactor for a wrong code,
// ErrTwoFactorEnabled when the user has TOTP enabled, and a *kernel.RateLimitedError after too
// many wrong codes.
func (s *Service) EnableTOTP(ctx context.Context, userID, setupToken, code string) ([]string, error) {
	if err := s.needTwoFactorDeps(); err != nil {
		return nil, err
	}
	if err := s.checkSecondFactorLimit(ctx, userID); err != nil {
		return nil, err
	}
	switch _, err := s.d.Repository.FindActiveTOTPEnrollment(ctx, userID); {
	case err == nil:
		return nil, ErrTwoFactorEnabled
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}
	secret, err := s.openSetup(ctx, userID, setupToken)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	step, ok := identity.MatchTOTP(secret, code, now)
	if !ok {
		return nil, s.secondFactorFailed(ctx, userID, "")
	}
	e, codes, hashes, err := s.newEnrollment(ctx, userID, secret, now)
	if err != nil {
		return nil, err
	}
	// The code that confirmed the setup cannot sign in too. It is marked before the enrollment
	// is stored, so that a failure here enrolls nothing rather than codes nobody has seen.
	if _, err := s.useStep(ctx, e.ID, step); err != nil {
		return nil, err
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Repository.InsertTOTPEnrollment(ctx, tx, e, hashes)
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) openSetup(ctx context.Context, userID, token string) ([]byte, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidTOTPSetup
	}
	plain, err := s.d.Sealer.Open(ctx, sealed, totpSetupAAD(userID))
	if errors.Is(err, kernel.ErrDecrypt) || errors.Is(err, kernel.ErrUnknownKey) {
		return nil, ErrInvalidTOTPSetup
	}
	if err != nil {
		return nil, err
	}
	if len(plain) != 8+identity.TOTPSecretSize {
		return nil, ErrInvalidTOTPSetup
	}
	issued := time.UnixMilli(int64(binary.BigEndian.Uint64(plain))) //nolint:gosec // G115: written by BeginTOTPSetup
	if !s.d.Clock.Now().Before(issued.Add(TOTPSetupTTL)) {
		return nil, ErrInvalidTOTPSetup
	}
	return plain[8:], nil
}

// newEnrollment seals secret for a new enrollment of userID and generates its recovery codes.
func (s *Service) newEnrollment(ctx context.Context, userID string, secret []byte, now time.Time) (identity.TOTPEnrollment, []string, [][]byte, error) {
	e := identity.TOTPEnrollment{ID: s.d.IDs.NewID(), UserID: userID, EnrolledAt: now}
	sealed, err := s.d.Sealer.Seal(ctx, secret, TOTPSecretAAD(e.ID))
	if err != nil {
		return identity.TOTPEnrollment{}, nil, nil, err
	}
	e.SecretEnc = sealed
	codes := make([]string, identity.RecoveryCodeCount)
	hashes := make([][]byte, identity.RecoveryCodeCount)
	b := make([]byte, identity.RecoveryCodeLength)
	for i := range codes {
		_, _ = rand.Read(b) // crypto/rand.Read never returns an error
		codes[i] = identity.RecoveryCodeFromBytes(b)
		hashes[i] = identity.HashRecoveryCode(identity.NormalizeRecoveryCode(codes[i]))
	}
	return e, codes, hashes, nil
}
