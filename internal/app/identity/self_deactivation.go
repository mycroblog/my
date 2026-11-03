// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// Deactivating one's own account (ADR-0145). A user who confirms it with the password, a code of
// the second factor, a passkey, or a linked login provider hides their account for
// account.DeactivationPeriod: every session and token ends in the transaction
// (RegisterLifecycleProjectors), and one email states the date of the deletion where the server
// sends mail. The user may sign in, which reaches the server-rendered status page only, and
// reactivate or delete the account there. Past the date the deletion job deletes it.

// SelfDeactivationLimit bounds the wrong passwords, passkeys, and provider sign-ins that confirm
// the deactivation of a user's own account, per user. Wrong codes count against
// SecondFactorLimit.
var SelfDeactivationLimit = kernel.Limit{Name: "self_deactivation.confirmation", Max: 5, Window: time.Hour}

// PathSignIn is the path of the sign-in page.
const PathSignIn = "/auth/sign_in"

// JobDeactivationEmail tells a user that their account is deactivated and when it is deleted.
const JobDeactivationEmail = "identity.deactivation_email"

// DeleteConfirmationFreshness is how old a session may be that confirms the deletion of a
// deactivated account of a user without a password: the sign-in is the confirmation.
const DeleteConfirmationFreshness = 15 * time.Minute

// ErrSignInAgain reports a deletion from the status page by a user without a password whose sign-in
// is too old to confirm it.
var ErrSignInAgain = errors.New("identity: sign in again to confirm")

// LiveSubscriptions tells whether a user has a live subscription to a supporter plan, which keeps
// them from deactivating the account (ADR-0145 §1). The composition root implements it on the
// supporters module, which is built after this one.
type LiveSubscriptions interface {
	HasLiveSubscription(ctx context.Context, userID string) (bool, error)
}

// DeactivationPreview is what the settings screen says before the user confirms.
type DeactivationPreview struct {
	// DeletionDueAt is when the account would be deleted if it were deactivated now.
	DeletionDueAt time.Time
	// Refused is why it cannot be deactivated now; nil when it can.
	Refused *account.DeactivationRefusedError
}

// PreviewDeactivation returns what deactivating the account of a user would do now.
func (s *Service) PreviewDeactivation(ctx context.Context, userID string) (DeactivationPreview, error) {
	access, err := s.UserAccess(ctx, userID)
	if err != nil {
		return DeactivationPreview{}, err
	}
	out := DeactivationPreview{DeletionDueAt: kernel.Normalize(s.d.Clock.Now()).Add(account.DeactivationPeriod)}
	var refused *account.DeactivationRefusedError
	switch err := s.deactivationRefused(ctx, userID, access); {
	case errors.As(err, &refused):
		out.Refused = refused
	case err != nil:
		return DeactivationPreview{}, err
	}
	return out, nil
}

// deactivationRefused returns the *account.DeactivationRefusedError of a user who cannot deactivate
// their account now, nil for one who can.
func (s *Service) deactivationRefused(ctx context.Context, userID string, access Access) error {
	switch {
	case access.AccountID == "":
		return fmt.Errorf("identity: user %s has no local account: %w", userID, ErrNotFound)
	case access.Registration != identity.RegistrationApproved:
		return &account.DeactivationRefusedError{Reason: account.DeactivationRefusedPending}
	case access.LoginRestricted:
		return &account.DeactivationRefusedError{Reason: account.DeactivationRefusedRestricted}
	}
	if err := s.d.Accounts.DeactivationRefused(ctx, access.AccountID); err != nil {
		return err
	}
	switch last, err := s.IsLastAdministrator(ctx, userID); {
	case err != nil:
		return err
	case last:
		return &account.DeactivationRefusedError{Reason: account.DeactivationRefusedLastAdministrator}
	}
	if s.d.Subscriptions != nil {
		switch live, err := s.d.Subscriptions.HasLiveSubscription(ctx, userID); {
		case err != nil:
			return err
		case live:
			return &account.DeactivationRefusedError{Reason: account.DeactivationRefusedSubscription}
		}
	}
	return nil
}

// DeactivateOwnAccount deactivates the local account that a user logs in as (ADR-0145), once the
// user's current password, a code or recovery code of the second factor, a passkey, or a linked
// login provider confirms it, and returns when the account is deleted. The confirmation email is
// enqueued in the transaction of the deactivation, where the server sends mail. A refusal fails
// with a *account.DeactivationRefusedError before the confirmation is spent, a wrong password with
// ErrWrongPassword, a wrong code with ErrInvalidSecondFactor or ErrTwoFactorNotEnabled, a passkey
// or a provider with the errors of Service.confirm, and too many of them with a
// *kernel.RateLimitedError; they change nothing.
func (s *Service) DeactivateOwnAccount(ctx context.Context, userID string, c Confirmation) (time.Time, error) {
	if s.d.RateLimiter == nil {
		return time.Time{}, errors.New("identity: DeactivateOwnAccount needs a RateLimiter")
	}
	access, err := s.UserAccess(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	if err := s.deactivationRefused(ctx, userID, access); err != nil {
		return time.Time{}, err
	}
	var factor secondFactor
	now := s.d.Clock.Now()
	if c.TwoFactorCode != "" {
		if factor, now, err = s.confirmSecondFactor(ctx, userID, c.TwoFactorCode); err != nil {
			return time.Time{}, err
		}
	} else if err := s.confirm(ctx, OperationAccountDeactivation, userID, c); err != nil {
		return time.Time{}, err
	}
	guard, err := s.guardLastAdministrator(ctx, userID)
	if err != nil {
		return time.Time{}, err
	}
	id := s.d.IDs.NewID()
	var at time.Time
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if guard != nil {
			// Two administrators who deactivate at once take turns, and the second one is refused.
			if err := guard(tx); errors.Is(err, ErrLastAdmin) {
				return &account.DeactivationRefusedError{Reason: account.DeactivationRefusedLastAdministrator}
			} else if err != nil {
				return err
			}
		}
		// Two deactivations at once: the first one locks, and the second finds it done.
		if err := s.d.Repository.LockSignInMethods(ctx, tx, userID); err != nil {
			return err
		}
		if err := s.useRecoveryCode(ctx, tx, factor, now); err != nil {
			return err
		}
		e, err := s.d.Accounts.DeactivateTx(ctx, tx, access.AccountID, id)
		if err != nil {
			return err
		}
		at = e.OccurredAt
		if !s.PasswordResetByEmail() {
			return nil
		}
		return s.enqueueMail(ctx, tx, JobDeactivationEmail, id, deactivationEmail{UserID: userID, DeactivatedAt: e.OccurredAt})
	})
	if err := s.afterSecondFactorTx(ctx, userID, "", err); err != nil {
		return time.Time{}, err
	}
	return at.Add(account.DeactivationPeriod), nil
}

// ReactivateOwnAccount brings back the account of a user whose deactivation has not passed its
// date (account.Service.Reactivate). The sessions and tokens that the deactivation ended stay
// ended, and moderation actions stay in effect.
func (s *Service) ReactivateOwnAccount(ctx context.Context, userID string) error {
	st, err := s.d.Accounts.UserLifecycle(ctx, userID)
	if err != nil {
		return err
	}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		return s.d.Accounts.ReactivateTx(ctx, tx, st.AccountID)
	})
}

// DeleteDeactivatedAccount deletes a deactivated account from the status page, which cannot run the
// ceremony of a passkey or a provider: a user with a password confirms with it, as
// DeleteOwnAccount does, and one without has just signed in, with what signs them in, in a session
// younger than DeleteConfirmationFreshness; ErrSignInAgain otherwise.
func (s *Service) DeleteDeactivatedAccount(ctx context.Context, session identity.Session, password string) error {
	has, err := s.HasPassword(ctx, session.UserID)
	if err != nil {
		return err
	}
	if has {
		return s.DeleteOwnAccount(ctx, session.UserID, Confirmation{Password: password})
	}
	if s.d.Clock.Now().Sub(session.CreatedAt) > DeleteConfirmationFreshness {
		return ErrSignInAgain
	}
	guard, err := s.guardLastAdministrator(ctx, session.UserID)
	if err != nil {
		return err
	}
	st, err := s.d.Accounts.UserLifecycle(ctx, session.UserID)
	if err != nil {
		return err
	}
	return s.d.Accounts.DeleteBySelfGuarded(ctx, st.AccountID, guard)
}

// deactivationEmail is the payload of JobDeactivationEmail.
type deactivationEmail struct {
	UserID string `json:"user_id"`
	// DeactivatedAt is the time of the deactivation, which the date in the email comes from.
	DeactivatedAt time.Time `json:"deactivated_at"`
}

// sendDeactivationEmail runs JobDeactivationEmail: while the deactivation it tells of is still in
// force, it tells the user the date of the deletion, with the link that signs in to undo it. It
// sends nothing without a primary address, or when the suppression of the address stops it.
func (s *Service) sendDeactivationEmail(ctx context.Context, run queue.Job, job deactivationEmail) error {
	st, err := s.d.Accounts.UserLifecycle(ctx, job.UserID)
	switch {
	case errors.Is(err, account.ErrNotFound):
		return nil
	case err != nil:
		return err
	case !st.Deactivated() || !st.Since.Equal(job.DeactivatedAt):
		return nil
	}
	email, _, err := s.d.Repository.FindPrimaryEmail(ctx, job.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if stopped, err := s.mailStopped(ctx, email.ID, identity.MailTransactional); err != nil || stopped {
		return err
	}
	prefs, err := s.d.Repository.FindPreferences(ctx, job.UserID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	m, err := s.mailContext(ctx, prefs.Locale)
	if err != nil {
		return err
	}
	return s.send(ctx, run, JobDeactivationEmail, job.UserID, email.ID, m.composer,
		DeactivationEmailDraft(m.wording(), email.Address, DeletionDateText(st.DeactivationDueAt(), prefs.TimeZone), s.link(PathSignIn, "")))
}
