// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/identity"
)

// What the notifications read of the identity module (FR-712, ADR-0109 §4): who holds a
// permission, where a user's notification emails go, and how many registrations await a
// decision.

// EventRegistrationPending follows a sign-up whose registration awaits a moderator's decision.
const EventRegistrationPending = "identity.registration_pending"

// RegistrationPending carries a new user whose registration awaits a moderator's approval: the
// sign-up of approval mode, and of open mode when no email verification stands in for it
// (FR-002). It is emitted in the sign-up's transaction.
type RegistrationPending struct {
	UserID string
	// AccountID is the user's local account, and Username its username.
	AccountID   string
	Username    string
	SubmittedAt time.Time
}

// EventType implements uow.Event.
func (RegistrationPending) EventType() string { return EventRegistrationPending }

// EventRegistrationApproved follows the approval of a registration: at the sign-up, when the
// registration needs no moderator, or by a moderator's decision.
const EventRegistrationApproved = "identity.registration_approved"

// RegistrationApproved carries a user whose registration was approved, which makes the local
// account usable. It is emitted in the transaction of the approval.
type RegistrationApproved struct {
	UserID string
	// AccountID is the user's local account, and Username its username.
	AccountID  string
	Username   string
	ApprovedAt time.Time
}

// EventType implements uow.Event.
func (RegistrationApproved) EventType() string { return EventRegistrationApproved }

// PermissionHolder is a user whose current roles grant a permission.
type PermissionHolder struct {
	UserID string
	// AccountID is the user's local account.
	AccountID string
}

// PermissionHolders returns inside tx the users whose current roles grant p, with their local
// accounts. A user without a local account is left out.
func (s *Service) PermissionHolders(ctx context.Context, tx uow.Tx, p identity.Permission) ([]PermissionHolder, error) {
	ids, err := s.d.Repository.ListUserIDsWithPermission(ctx, tx, p)
	if err != nil {
		return nil, err
	}
	out := make([]PermissionHolder, 0, len(ids))
	for _, id := range ids {
		la, err := s.d.Accounts.LocalAccountOfUser(ctx, id)
		switch {
		case errors.Is(err, account.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		}
		out = append(out, PermissionHolder{UserID: id, AccountID: la.AccountID})
	}
	return out, nil
}

// Mailbox is where a user's notification emails go: the primary address and its ID, and the
// user's locale.
type Mailbox struct {
	EmailAddressID string
	Address        string
	Locale         string
}

// NotificationMailbox returns where a user's notification emails are sent (ADR-0109 §4): the
// primary address once verified, in the user's locale. ok is false when the user has no
// preferences or no primary address, has not verified it, or when the suppression of the
// address stops notifications (ADR-0086 §5). Whether the user wants a notification by email is
// the notification settings' to say.
func (s *Service) NotificationMailbox(ctx context.Context, userID string) (box Mailbox, ok bool, err error) {
	prefs, err := s.d.Repository.FindPreferences(ctx, userID)
	switch {
	case errors.Is(err, ErrNotFound):
		return Mailbox{}, false, nil
	case err != nil:
		return Mailbox{}, false, err
	}
	email, verified, err := s.d.Repository.FindPrimaryEmail(ctx, userID)
	switch {
	case errors.Is(err, ErrNotFound):
		return Mailbox{}, false, nil
	case err != nil:
		return Mailbox{}, false, err
	case !verified:
		return Mailbox{}, false, nil
	}
	if stopped, err := s.mailStopped(ctx, email.ID, identity.MailNotification); err != nil || stopped {
		return Mailbox{}, false, err
	}
	return Mailbox{EmailAddressID: email.ID, Address: email.Address, Locale: prefs.Locale}, true, nil
}

// CountPendingRegistrations counts the registrations that await a moderator's decision
// (GET /api/v1/admin/counts).
func (s *Service) CountPendingRegistrations(ctx context.Context) (int64, error) {
	return s.d.Repository.CountRegistrations(ctx, identity.RegistrationPending)
}
