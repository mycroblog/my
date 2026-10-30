// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// ErrNotFound reports that no row matches a lookup, or a notification that does not belong to
// the account acting on it.
var ErrNotFound = errors.New("notification: not found")

// Repository is the persistence port of the notification module: notifications, their
// dismissals, read markers, the exclusions projected from the graph module's blocks, mutes, and
// domain mutes,
// and the statuses that the content module deleted (data-model §4.7). The database adapters implement it and pass
// testkit.NotificationContract.
//
// Methods that take a uow.Tx run inside that transaction; the others read committed data outside
// any transaction. Lookups that find nothing return an error wrapping ErrNotFound.
type Repository interface {
	SubscriptionRepository

	// InsertNotification inserts a notification and reports false, writing nothing, when the
	// recipient already has one with the same dedupe key.
	InsertNotification(ctx context.Context, tx uow.Tx, n notification.Notification) (bool, error)
	// AddErrorGroupNotice adds one admin_error_group notice to the hour that starts at hour, in
	// tx, and returns the hour's notices with it; concurrent transactions each add theirs.
	AddErrorGroupNotice(ctx context.Context, tx uow.Tx, hour time.Time) (int64, error)
	// ErrorGroupNotices returns the admin_error_group notices of the hour that starts at hour.
	ErrorGroupNotices(ctx context.Context, hour time.Time) (int64, error)
	// DeleteErrorGroupNoticesBefore deletes the notices of the hours that start before before, in
	// one statement outside any transaction, and returns how many hours it deleted.
	DeleteErrorGroupNoticesBefore(ctx context.Context, before time.Time) (int64, error)
	// Notification returns a notification by ID.
	Notification(ctx context.Context, tx uow.Tx, id string) (notification.Notification, error)
	// InsertDismissal inserts a dismissal and reports false, writing nothing, when the
	// notification is already dismissed.
	InsertDismissal(ctx context.Context, tx uow.Tx, d notification.Dismissal) (bool, error)
	// AdvanceReadMarker sets the account's marker to m.LastReadID unless it is already there or
	// past it, and reports whether it moved.
	AdvanceReadMarker(ctx context.Context, tx uow.Tx, m notification.ReadMarker) (bool, error)
	// SaveNotificationSettings stores the account's choices of settings (FR-210), replacing an
	// earlier choice of the same group and channel, as chosen at at.
	SaveNotificationSettings(ctx context.Context, tx uow.Tx, settings []notification.Setting, at time.Time) error
	// NotificationSettingsTx returns inside tx the settings that the account chose, in order of
	// group and channel.
	NotificationSettingsTx(ctx context.Context, tx uow.Tx, accountID string) ([]notification.Setting, error)
	// NotificationSettings is NotificationSettingsTx outside any transaction.
	NotificationSettings(ctx context.Context, accountID string) ([]notification.Setting, error)

	// Projections, written only by this module's projectors.

	// UpsertExclusion inserts an exclusion, or replaces the expiry of the one with the same
	// viewer, target, and reason.
	UpsertExclusion(ctx context.Context, tx uow.Tx, x notification.Exclusion) error
	// DeleteExclusion deletes the exclusion of the viewer, target, and reason, if any.
	DeleteExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, targetAccountID string, reason notification.ExclusionReason) error
	// InsertDomainExclusion records that the viewer mutes a domain and its subdomains, which
	// hides the notifications whose source domain it covers; recording it again changes nothing.
	InsertDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error
	// DeleteDomainExclusion deletes the viewer's exclusion of a domain, if any.
	DeleteDomainExclusion(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) error
	// DomainExcluded reports whether an exclusion of the viewer covers domain: the domain itself
	// or one of its parent domains.
	DomainExcluded(ctx context.Context, tx uow.Tx, viewerAccountID, domain string) (bool, error)
	// InsertDeletedStatus records that a status is deleted, so that the notifications about it
	// are no longer visible. Recording it again changes nothing.
	InsertDeletedStatus(ctx context.Context, tx uow.Tx, statusID string) error
	// DeleteAllExclusions (with the domain exclusions) and DeleteAllDeletedStatuses empty the
	// projections, for a rebuild.
	DeleteAllExclusions(ctx context.Context, tx uow.Tx) error
	DeleteAllDeletedStatuses(ctx context.Context, tx uow.Tx) error
	// ListNotifiedStatusIDs returns every status that a notification, dismissed or not, is
	// about, in order, for rebuilding notification_deleted_statuses.
	ListNotifiedStatusIDs(ctx context.Context, tx uow.Tx) ([]string, error)

	// Lookups.

	// ListNotifications returns the recipient's visible notifications, newest first, starting
	// after the notification ID before ("" for the newest) and returning at most limit
	// notifications. A non-empty types keeps only notifications of those types, and hidden
	// leaves out the notifications of its types and of the announcements it does not show. A
	// notification is visible when it is not dismissed, no exclusion of the recipient that is in
	// force at now has its source as the target, no domain exclusion of the recipient covers its
	// source domain, and its status, if any, is not recorded as deleted.
	ListNotifications(ctx context.Context, recipientAccountID string, types []notification.Type, hidden Hidden, before string, limit int, now time.Time) ([]notification.Notification, error)
	// ListNotificationsAfter is ListNotifications in the other direction: the recipient's
	// visible notifications whose ID is greater than after, oldest first, at most limit.
	ListNotificationsAfter(ctx context.Context, recipientAccountID string, types []notification.Type, hidden Hidden, after string, limit int, now time.Time) ([]notification.Notification, error)
	// NotificationListed reports whether the notification id is the recipient's and visible
	// as in ListNotifications with hidden.
	NotificationListed(ctx context.Context, recipientAccountID, id string, hidden Hidden, now time.Time) (bool, error)
	// CountNotificationsAfter counts the recipient's visible notifications, as in
	// ListNotifications with hidden, whose ID is greater than after ("" counts them all).
	CountNotificationsAfter(ctx context.Context, recipientAccountID, after string, hidden Hidden, now time.Time) (int64, error)
	// FindReadMarker returns an account's read marker; ErrNotFound means it has read nothing.
	FindReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error)
	// StatusHasNotifications reports whether a notification, dismissed or not, is about the status.
	StatusHasNotifications(ctx context.Context, tx uow.Tx, statusID string) (bool, error)
}

// Hidden is what the reads of the notifications leave out besides the dismissals, the
// exclusions, and the deleted statuses, as of the moment of reading.
type Hidden struct {
	// Types are the moderator types whose permission the recipient's roles no longer grant.
	Types []notification.Type
	// ShownAnnouncements are the announcements that the users see now: the notifications of
	// the other announcements (withdrawn or ended) are left out (ADR-0125 §2).
	ShownAnnouncements []string
}
