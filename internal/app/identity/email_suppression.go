// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The suppression of the email addresses that do not receive email (ADR-0086 §5). A failure of
// delivery (a hard bounce or a complaint) suppresses its address until a lift; the handlers of
// the emails check the suppression before they send (identity.EmailSuppression.Stops). The
// identity module records the failures, the lifts, and the projection email_suppressions, which
// it updates in the transaction of each event (ADR-0020 rule 7) by the rule that a rebuild
// applies (RebuildEmailSuppressions).

// ErrNotSuppressed reports a lift of an address that is not suppressed.
var ErrNotSuppressed = errors.New("identity: the email address is not suppressed")

// EmailSuppressionRepository is the part of Repository that records the suppressions.
type EmailSuppressionRepository interface {
	// InsertEmailDeliveryFailure records f and reports whether it did: a failure with the
	// FeedbackKey of a recorded one is not recorded again.
	InsertEmailDeliveryFailure(ctx context.Context, tx uow.Tx, f identity.EmailDeliveryFailure) (bool, error)
	InsertEmailSuppressionLift(ctx context.Context, tx uow.Tx, l identity.EmailSuppressionLift) error
	// RefreshEmailSuppression derives the row of email_suppressions of one address again from
	// its failures and lifts.
	RefreshEmailSuppression(ctx context.Context, tx uow.Tx, emailAddressID string) error
	// FindEmailSuppression returns the suppression of an address; ErrNotFound when it is not
	// suppressed.
	FindEmailSuppression(ctx context.Context, emailAddressID string) (identity.EmailSuppression, error)
}

// DeliveryFailure is a failure of delivery to record (RecordDeliveryFailure).
type DeliveryFailure struct {
	EmailAddressID string
	Kind           identity.EmailFailureKind
	Source         identity.EmailFailureSource
	// FeedbackKey identifies the fact: the same key recorded again is one record.
	FeedbackKey string
	// EnhancedStatus is the RFC 3463 code that the source named; anything else is dropped.
	EnhancedStatus string
	OccurredAt     time.Time
}

// RecordDeliveryFailure records a hard bounce or a complaint of an address and suppresses the
// address, in one transaction. A failure whose FeedbackKey is recorded already changes nothing.
func (s *Service) RecordDeliveryFailure(ctx context.Context, in DeliveryFailure) error {
	if in.EmailAddressID == "" || in.FeedbackKey == "" {
		return errors.New("identity: a delivery failure needs an address and a feedback key")
	}
	f := identity.EmailDeliveryFailure{
		ID: s.d.IDs.NewID(), EmailAddressID: in.EmailAddressID, Kind: in.Kind, Source: in.Source,
		FeedbackKey: in.FeedbackKey, EnhancedStatus: identity.EnhancedStatusCode(in.EnhancedStatus), OccurredAt: in.OccurredAt,
	}
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		inserted, err := s.d.Repository.InsertEmailDeliveryFailure(ctx, tx, f)
		if err != nil || !inserted {
			return err
		}
		return s.d.Repository.RefreshEmailSuppression(ctx, tx, f.EmailAddressID)
	})
	if err != nil {
		return fmt.Errorf("identity: record the delivery failure of address %s: %w", in.EmailAddressID, err)
	}
	return nil
}

// RecordRecipientRejection records the rejection of the recipient of the email that the run of
// a job sent to an address (kernel.ErrRecipientRejected) as a hard bounce, keyed by the job's
// ID and attempt. It records nothing for any other error.
func (s *Service) RecordRecipientRejection(ctx context.Context, run queue.Job, emailAddressID string, rejection error) error {
	if !errors.Is(rejection, kernel.ErrRecipientRejected) || emailAddressID == "" {
		return nil
	}
	return s.RecordDeliveryFailure(ctx, DeliveryFailure{
		EmailAddressID: emailAddressID, Kind: identity.EmailHardBounce, Source: identity.EmailFailureRejection,
		FeedbackKey: "job:" + run.ID + ":" + strconv.Itoa(run.Attempt), EnhancedStatus: kernel.EnhancedStatus(rejection),
		OccurredAt: s.d.Clock.Now(),
	})
}

// EmailSuppression returns the suppression of an address and whether it is suppressed.
func (s *Service) EmailSuppression(ctx context.Context, emailAddressID string) (identity.EmailSuppression, bool, error) {
	return findSuppression(ctx, s.d.Repository, emailAddressID)
}

// findSuppression is EmailSuppression on repo.
func findSuppression(ctx context.Context, repo Repository, emailAddressID string) (identity.EmailSuppression, bool, error) {
	sup, err := repo.FindEmailSuppression(ctx, emailAddressID)
	switch {
	case errors.Is(err, ErrNotFound):
		return identity.EmailSuppression{}, false, nil
	case err != nil:
		return identity.EmailSuppression{}, false, err
	}
	return sup, true, nil
}

// mailStopped reports whether the suppression of an address stops an email of class c.
func (s *Service) mailStopped(ctx context.Context, emailAddressID string, c identity.MailClass) (bool, error) {
	sup, ok, err := s.EmailSuppression(ctx, emailAddressID)
	return ok && sup.Stops(c), err
}

// NotificationAddress returns the ID of a user's primary address and whether notification and
// announcement emails may go to it: false without a primary address or when it is suppressed.
func (s *Service) NotificationAddress(ctx context.Context, userID string) (emailAddressID string, ok bool, err error) {
	email, _, err := s.d.Repository.FindPrimaryEmail(ctx, userID)
	switch {
	case errors.Is(err, ErrNotFound):
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	stopped, err := s.mailStopped(ctx, email.ID, identity.MailNotification)
	if err != nil || stopped {
		return "", false, err
	}
	return email.ID, true, nil
}

// PrimaryEmailSuppression returns the suppression of a user's primary address and whether it is
// suppressed, for the user's own settings.
func (s *Service) PrimaryEmailSuppression(ctx context.Context, userID string) (identity.EmailSuppression, bool, error) {
	email, _, err := s.d.Repository.FindPrimaryEmail(ctx, userID)
	switch {
	case errors.Is(err, ErrNotFound):
		return identity.EmailSuppression{}, false, nil
	case err != nil:
		return identity.EmailSuppression{}, false, err
	}
	return s.EmailSuppression(ctx, email.ID)
}

// liftAfterVerification lifts the suppression of an address that the user verified with a token
// issued at issuedAt, when the token came after the failure that started the suppression: the
// email of an earlier token may be the one that bounced.
func (s *Service) liftAfterVerification(ctx context.Context, emailAddressID string, issuedAt, now time.Time) error {
	sup, ok, err := s.EmailSuppression(ctx, emailAddressID)
	if err != nil || !ok || issuedAt.Before(sup.Since) {
		return err
	}
	l := identity.EmailSuppressionLift{ID: s.d.IDs.NewID(), EmailAddressID: emailAddressID, Reason: identity.EmailLiftReverified, LiftedAt: now}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return recordLift(ctx, tx, s.d.Repository, l) })
}

// recordLift records l in tx and derives the suppression of its address again.
func recordLift(ctx context.Context, tx uow.Tx, repo Repository, l identity.EmailSuppressionLift) error {
	if err := repo.InsertEmailSuppressionLift(ctx, tx, l); err != nil {
		return err
	}
	return repo.RefreshEmailSuppression(ctx, tx, l.EmailAddressID)
}

// LiftEmailSuppression lifts the suppression of the primary address of a local account's user
// for byUserID ("" for the CLI), so that emails go to it again, and records it in the audit log
// (email_suppression_lifted) in the same transaction. It fails with account.ErrNotFound for an
// unknown account, ErrNotLocalAccount for an account without a user, and ErrNotSuppressed when
// the user has no primary address or it is not suppressed.
func (s *AccountAdmin) LiftEmailSuppression(ctx context.Context, accountID, byUserID string) error {
	actor, err := s.d.Accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if actor.Local == nil || actor.Local.UserID == "" {
		return ErrNotLocalAccount
	}
	email, _, err := s.d.Repository.FindPrimaryEmail(ctx, actor.Local.UserID)
	if errors.Is(err, ErrNotFound) {
		return ErrNotSuppressed
	}
	if err != nil {
		return err
	}
	if _, ok, err := findSuppression(ctx, s.d.Repository, email.ID); err != nil {
		return err
	} else if !ok {
		return ErrNotSuppressed
	}
	now, liftID := s.d.Clock.Now(), s.d.IDs.NewID()
	actorKind, actorUser := appmoderation.AuditActor(byUserID)
	audit := appmoderation.AuditRecorded{Entry: appmoderation.AuditEntry{
		ID: s.d.IDs.NewID(), ActorKind: actorKind, ActorUserID: actorUser, Action: appmoderation.AuditEmailSuppressionLifted,
		TargetKind: appmoderation.AuditTargetAccount, TargetID: accountID, TargetLabel: "@" + actor.Local.Username,
		SourceEventID: liftID, OccurredAt: now,
	}}
	l := identity.EmailSuppressionLift{ID: liftID, EmailAddressID: email.ID, Reason: identity.EmailLiftModerator, LiftedAt: now}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := recordLift(ctx, tx, s.d.Repository, l); err != nil {
			return err
		}
		return tx.Emit(ctx, audit)
	})
}
