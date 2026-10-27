// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Resetting a forgotten password and changing the password (FR-002). A reset link carries a
// one-time token (user_tokens, purpose password_reset) that works for PasswordResetTTL, once,
// and only while the password it replaces is the current one: a newer password, set by any
// means, or the removal of the password makes every earlier link stop working. A link sets a
// password for a user who removed theirs too. The sign-in page sends the link by email when
// the Mailer delivers email; an administrator can always issue one (IssuePasswordResetLink).
//
// A new password, from a reset or a change, ends the user's other sessions and grants
// (password_changed) and keeps the passkeys and the two-factor authentication, which do not
// depend on the password. The user is told by email when the Mailer delivers email
// (JobPasswordNoticeEmail).

// Password reset limits and lifetimes (ADR-0019 §4, §5).
var (
	// PasswordResetIPLimit bounds the requests for a reset link per client address; every
	// request with a well-formed address counts.
	PasswordResetIPLimit = kernel.Limit{Name: "password_reset.ip", Max: 5, Window: 30 * time.Minute}
	// PasswordResetEmailLimit bounds the reset emails per address, so that nobody can flood a
	// mailbox with them. Requests beyond it send nothing and answer as the others do.
	PasswordResetEmailLimit = kernel.Limit{Name: "password_reset.email", Max: 3, Window: time.Hour}
	// PasswordChangeLimit bounds the wrong current passwords that confirm a password change, per
	// user.
	PasswordChangeLimit = kernel.Limit{Name: "password_change.password", Max: 5, Window: time.Hour}
)

const (
	// PasswordResetTTL is how long a reset link works.
	PasswordResetTTL = time.Hour
	// PasswordResetTokenPrefix marks password reset tokens for secret scanners.
	PasswordResetTokenPrefix = "my_prt_"
	// PathResetPassword is the page that a reset link opens, with the token in the query
	// parameter "token". The authpages package serves it.
	PathResetPassword = "/auth/reset_password" //nolint:gosec // G101: a path, not a credential
	// PathForgotPassword is the page that asks for a reset link. The authpages package serves it.
	PathForgotPassword = "/auth/forgot_password" //nolint:gosec // G101: a path, not a credential
)

// ErrPasswordResetByEmailUnavailable reports a request for a reset link by email while the
// Mailer delivers none (Deps.EmailDelivery is false).
var ErrPasswordResetByEmailUnavailable = errors.New("identity: password reset by email is unavailable")

// PasswordResetByEmail reports whether users can ask for a reset link by email.
func (s *Service) PasswordResetByEmail() bool { return s.d.Mailer != nil && s.d.EmailDelivery }

// PasswordResetRequest is a request for a reset link from the sign-in pages.
type PasswordResetRequest struct {
	Email string
	// IP is the client address ("" when unknown); it keys PasswordResetIPLimit.
	IP string
	// HumanCheckToken is the token that the CAPTCHA widget put into the form; it is checked
	// only when Deps.HumanVerifier is set.
	HumanCheckToken string
}

// RequestPasswordReset enqueues the email of a reset link (JobPasswordResetEmail) when the
// address belongs to a user whose local account can sign in, and does nothing otherwise: the
// result is the same whether or not such a user exists, so that callers cannot tell. The job
// issues the link and sends it to the user's primary address.
//
// It fails with ErrPasswordResetByEmailUnavailable without email delivery, an error wrapping
// identity.ErrInvalidEmail for an address that is not one, a *kernel.RateLimitedError after
// PasswordResetIPLimit, and, with a HumanVerifier, an error wrapping kernel.ErrHumanCheckFailed
// or kernel.ErrHumanCheckUnavailable: like a sign-up, a request that the service cannot check
// is refused (fail closed, ADR-0042 §4), since it sends email.
func (s *Service) RequestPasswordReset(ctx context.Context, in PasswordResetRequest) error {
	if !s.PasswordResetByEmail() {
		return ErrPasswordResetByEmailUnavailable
	}
	if err := identity.ValidateEmail(in.Email); err != nil {
		return err
	}
	if s.d.RateLimiter == nil {
		return errors.New("identity: RequestPasswordReset needs a RateLimiter")
	}
	if key := kernel.ClientKey(in.IP); key != "" {
		d, err := s.d.RateLimiter.Take(ctx, PasswordResetIPLimit, key)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return &kernel.RateLimitedError{RetryAfter: d.RetryAfter}
		}
	}
	if err := s.checkHuman(ctx, in.HumanCheckToken, kernel.HumanCheckActionPasswordReset); err != nil {
		return err
	}
	normalized := identity.NormalizeEmail(in.Email)
	d, err := s.d.RateLimiter.Take(ctx, PasswordResetEmailLimit, normalized)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return nil
	}
	userID, err := s.d.Repository.FindUserIDByEmail(ctx, normalized)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	switch err := s.checkActive(ctx, userID); {
	case errors.Is(err, ErrInvalidCredentials):
		return nil
	case err != nil:
		return err
	}
	job := passwordResetEmail{RequestID: s.d.IDs.NewID(), UserID: userID, RequestedAt: s.d.Clock.Now()}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.enqueueMail(ctx, tx, JobPasswordResetEmail, job.RequestID, job)
	})
}

// issueResetToken stores a new reset token of a user, with audit in the same transaction when
// it is not nil, and returns its secret.
func (s *Service) issueResetToken(ctx context.Context, userID string, audit *appmoderation.AuditRecorded) (string, error) {
	now := s.d.Clock.Now()
	secret, hash := NewToken(PasswordResetTokenPrefix)
	t := identity.UserToken{
		ID: s.d.IDs.NewID(), UserID: userID, Purpose: identity.PurposePasswordReset,
		TokenHash: hash, ExpiresAt: now.Add(PasswordResetTTL), IssuedAt: now,
	}
	if audit != nil {
		audit.Entry.SourceEventID, audit.Entry.OccurredAt = t.ID, now
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertUserToken(ctx, tx, t); err != nil {
			return err
		}
		if audit == nil {
			return nil
		}
		return tx.Emit(ctx, *audit)
	})
	if err != nil {
		return "", err
	}
	return secret, nil
}

// PasswordResetLink is a reset link that an administrator issued.
type PasswordResetLink struct {
	URL       string
	ExpiresAt time.Time
}

// IssuePasswordResetLink issues a reset link for the user of a local account for byUserID
// ("" for the CLI), for an administrator to hand to the user, such as when the Mailer delivers
// no email, and records it in the audit log (password_reset_link_issued) in the same
// transaction. It fails with account.ErrNotFound for an unknown account and with
// ErrNotLocalAccount for an account without a user.
func (s *Service) IssuePasswordResetLink(ctx context.Context, accountID, byUserID string) (PasswordResetLink, error) {
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return PasswordResetLink{}, err
	}
	if actor.Local == nil || actor.Local.UserID == "" {
		return PasswordResetLink{}, ErrNotLocalAccount
	}
	kind, user := appmoderation.AuditActor(byUserID)
	audit := appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: kind, ActorUserID: user, Action: appmoderation.AuditPasswordResetLinkIssued,
		TargetKind: appmoderation.AuditTargetAccount, TargetID: accountID, TargetLabel: "@" + actor.Local.Username,
	}}
	secret, err := s.issueResetToken(ctx, actor.Local.UserID, &audit)
	if err != nil {
		return PasswordResetLink{}, err
	}
	return PasswordResetLink{URL: s.link(PathResetPassword, secret), ExpiresAt: audit.Entry.OccurredAt.Add(PasswordResetTTL)}, nil
}

// resetToken returns the reset token of secret while it works: issued as a reset token, not
// expired, not used, and not older than the last change of the user's password. It fails with
// ErrInvalidToken otherwise.
func (s *Service) resetToken(ctx context.Context, secret string) (identity.UserToken, error) {
	if !strings.HasPrefix(secret, PasswordResetTokenPrefix) {
		return identity.UserToken{}, ErrInvalidToken
	}
	t, err := s.d.Repository.FindUserTokenByHash(ctx, HashToken(secret))
	if errors.Is(err, ErrNotFound) {
		return identity.UserToken{}, ErrInvalidToken
	}
	if err != nil {
		return identity.UserToken{}, err
	}
	if t.Purpose != identity.PurposePasswordReset || t.Expired(s.d.Clock.Now()) {
		return identity.UserToken{}, ErrInvalidToken
	}
	switch used, err := s.d.Repository.UserTokenRedeemed(ctx, t.ID); {
	case err != nil:
		return identity.UserToken{}, err
	case used:
		return identity.UserToken{}, ErrInvalidToken
	}
	switch changed, err := s.d.Repository.LastPasswordChange(ctx, t.UserID); {
	case err == nil:
		if t.IssuedAt.Before(changed) {
			return identity.UserToken{}, ErrInvalidToken
		}
	case !errors.Is(err, ErrNotFound):
		return identity.UserToken{}, err
	}
	return t, nil
}

// CheckPasswordResetToken reports whether a reset link's token works, without using it: nil, or
// ErrInvalidToken.
func (s *Service) CheckPasswordResetToken(ctx context.Context, secret string) error {
	_, err := s.resetToken(ctx, secret)
	return err
}

// ResetPassword sets a new password with the token of a reset link, which then stops working,
// and returns the user. It ends every session and grant of the user (password_changed); the
// passkeys and the two-factor authentication stay, so a user with two-factor authentication
// still enters a code to sign in. It fails with ErrInvalidToken for a token that does not work
// (CheckPasswordResetToken), also when a concurrent reset used it first, and with an error
// wrapping identity.ErrInvalidPassword for an invalid password.
func (s *Service) ResetPassword(ctx context.Context, secret, password string) (userID string, err error) {
	t, err := s.resetToken(ctx, secret)
	if err != nil {
		return "", err
	}
	if err := identity.ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := s.d.Passwords.Hash(ctx, password)
	if err != nil {
		return "", err
	}
	now := s.d.Clock.Now()
	cred := identity.PasswordCredential{ID: s.d.IDs.NewID(), UserID: t.UserID, Hash: hash, SetAt: now}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.d.Repository.InsertUserTokenRedemption(ctx, tx, identity.UserTokenRedemption{TokenID: t.ID, RedeemedAt: now}); err != nil {
			return err
		}
		if err := s.writePassword(ctx, tx, cred, ""); err != nil {
			return err
		}
		return s.enqueuePasswordNotice(ctx, tx, t.UserID, passwordChanged, cred.ID)
	})
	if err != nil {
		return "", err
	}
	return t.UserID, nil
}

// ChangePasswordInput is a signed-in user's change of their password.
type ChangePasswordInput struct {
	UserID string
	// Confirmation is the current password, or a passkey of the user; a user without a password
	// confirms with a passkey and sets a password.
	Confirmation Confirmation
	NewPassword  string
	// KeepFamilyID is the token family of the client that makes the change, which keeps
	// working; "" ends every grant.
	KeepFamilyID string
}

// ChangePassword replaces a user's password once the current one or a passkey of the user
// confirms it; a user without a password sets one. It ends every session of the user and every
// grant but in.KeepFamilyID (password_changed); the passkeys and the two-factor authentication
// stay. It fails with ErrWrongPassword for a wrong current password (also for a user without
// one), the errors of a confirmation with a passkey (Service.confirm), a *kernel.RateLimitedError
// after too many wrong confirmations (PasswordChangeLimit), and an error wrapping
// identity.ErrInvalidPassword for an invalid new password.
func (s *Service) ChangePassword(ctx context.Context, in ChangePasswordInput) error {
	if s.d.RateLimiter == nil {
		return errors.New("identity: ChangePassword needs a RateLimiter")
	}
	if err := identity.ValidatePassword(in.NewPassword); err != nil {
		return err
	}
	if err := s.confirm(ctx, PasswordChangeLimit, in.UserID, in.Confirmation); err != nil {
		return err
	}
	hash, err := s.d.Passwords.Hash(ctx, in.NewPassword)
	if err != nil {
		return err
	}
	cred := identity.PasswordCredential{ID: s.d.IDs.NewID(), UserID: in.UserID, Hash: hash, SetAt: s.d.Clock.Now()}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := s.writePassword(ctx, tx, cred, in.KeepFamilyID); err != nil {
			return err
		}
		return s.enqueuePasswordNotice(ctx, tx, in.UserID, passwordChanged, cred.ID)
	})
}

// writePassword records a new password and ends the user's sessions and grants but the grant
// keepFamilyID ("" for none).
func (s *Service) writePassword(ctx context.Context, tx uow.Tx, cred identity.PasswordCredential, keepFamilyID string) error {
	repo := s.d.Repository
	if err := repo.InsertPasswordCredential(ctx, tx, cred); err != nil {
		return err
	}
	return s.endOtherSessions(ctx, tx, cred.UserID, keepFamilyID, cred.SetAt)
}

// endOtherSessions ends, for a change of the password at at, the user's sessions and grants but
// the grant keepFamilyID ("" for none).
func (s *Service) endOtherSessions(ctx context.Context, tx uow.Tx, userID, keepFamilyID string, at time.Time) error {
	repo := s.d.Repository
	if err := repo.LockTokenGrants(ctx, tx, userID); err != nil {
		return err
	}
	if err := repo.RevokeSessionsOfUser(ctx, tx, userID, identity.RevokedPasswordChanged, at); err != nil {
		return err
	}
	if keepFamilyID == "" {
		return repo.RevokeTokensOfUser(ctx, tx, userID, identity.TokenRevokedPasswordChanged, at)
	}
	return repo.RevokeOtherTokensOfUser(ctx, tx, userID, keepFamilyID, identity.TokenRevokedPasswordChanged, at)
}
