// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// Deps are the ports a Service uses.
type Deps struct {
	UnitOfWork uow.UnitOfWork
	Repository Repository
	Clock      kernel.Clock
	// Staff reads the permissions of the recipients of the moderator notifications, which the
	// list leaves out once the recipient no longer holds them, and which offer the moderator rows
	// of the notification settings; nil holds none.
	Staff Staff
	// SupportersEnabled reports whether the supporter program is on, which offers the supporter
	// row of the notification settings; nil means it is off.
	SupportersEnabled func(ctx context.Context) (bool, error)
	// Announcements tells which announcements the users see now: the list leaves out the
	// notifications of the others (ADR-0125 §2). nil shows none of them.
	Announcements ShownAnnouncements
}

// ShownAnnouncements tells which server-wide announcements the users see now: published and
// within their window. *appinstance.Service implements it.
type ShownAnnouncements interface {
	ShownAnnouncementIDs(ctx context.Context) ([]string, error)
}

// Service runs the notification module's use cases of a local account's own notifications
// (FR-205): listing them, the read marker and unread count, and dismissing them. The projectors
// that create notifications are registered with RegisterProjectors.
type Service struct {
	d Deps
}

// NewService returns a Service on d.
func NewService(d Deps) *Service { return &Service{d: d} }

// Page sizes of the notification list.
const (
	DefaultPageLimit = 40
	MaxPageLimit     = 80
)

// Query selects a page of an account's notifications, newest first, or oldest first after a
// cursor (After).
type Query struct {
	// Types keeps only notifications of these types; empty keeps every type.
	Types []notification.Type
	// Before is the ID of the last notification of the previous page; "" starts at the newest.
	Before string
	// After, when set, pages forward instead: the page holds the notifications whose ID is
	// greater than After, oldest first, and Before is ignored. A reader that follows new
	// notifications passes the ID of the last one it has, so that a burst larger than a page
	// takes several pages and none is skipped.
	After string
	// Limit is the page size; zero or less means DefaultPageLimit, and it is capped at MaxPageLimit.
	Limit int
}

func (q Query) limit() int {
	switch {
	case q.Limit <= 0:
		return DefaultPageLimit
	case q.Limit > MaxPageLimit:
		return MaxPageLimit
	}
	return q.Limit
}

// List returns accountID's notifications that are not dismissed, newest first (oldest first
// with q.After). It leaves out
// the notifications from accounts that accountID blocks or is blocked by, or mutes with
// hide_notifications, at the time of reading, also those notified before the block or mute,
// and the notifications about deleted statuses. It also leaves out the moderator notifications
// whose permission (notification.ModeratorPermission) the roles of accountID's user no longer
// grant, and the notifications of the announcements that are withdrawn or ended.
// An unknown type in q.Types fails with notification.ErrInvalidType.
func (s *Service) List(ctx context.Context, accountID string, q Query) ([]notification.Notification, error) {
	for _, t := range q.Types {
		if !t.Valid() {
			return nil, fmt.Errorf("type %q: %w", t, notification.ErrInvalidType)
		}
	}
	hidden, err := s.hidden(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if q.After != "" {
		return s.d.Repository.ListNotificationsAfter(ctx, accountID, q.Types, hidden, q.After, q.limit(), s.d.Clock.Now())
	}
	return s.d.Repository.ListNotifications(ctx, accountID, q.Types, hidden, q.Before, q.limit(), s.d.Clock.Now())
}

// Listed reports whether List would return accountID's notification id now: it exists for
// accountID, is not dismissed, and no block, mute, domain mute, deleted status, lost
// permission, or withdrawn or ended announcement hides it. The Web Push job checks it before sending.
func (s *Service) Listed(ctx context.Context, accountID, id string) (bool, error) {
	hidden, err := s.hidden(ctx, accountID)
	if err != nil {
		return false, err
	}
	return s.d.Repository.NotificationListed(ctx, accountID, id, hidden, s.d.Clock.Now())
}

// ReadMarker returns accountID's read marker; an account that has read nothing has a marker
// with an empty LastReadID.
func (s *Service) ReadMarker(ctx context.Context, accountID string) (notification.ReadMarker, error) {
	m, err := s.d.Repository.FindReadMarker(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return notification.ReadMarker{AccountID: accountID}, nil
	}
	return m, err
}

// UnreadCount returns the number of accountID's notifications that List would return and that
// are newer than its read marker.
func (s *Service) UnreadCount(ctx context.Context, accountID string) (int64, error) {
	m, err := s.ReadMarker(ctx, accountID)
	if err != nil {
		return 0, err
	}
	hidden, err := s.hidden(ctx, accountID)
	if err != nil {
		return 0, err
	}
	return s.d.Repository.CountNotificationsAfter(ctx, accountID, m.LastReadID, hidden, s.d.Clock.Now())
}

// hidden returns what the reads of accountID's notifications leave out now: the types of the
// moderator notifications that accountID's user no longer has the permission for, and every
// announcement but those that the users see now.
func (s *Service) hidden(ctx context.Context, accountID string) (Hidden, error) {
	var granted []notification.Permission
	if s.d.Staff != nil {
		var err error
		if granted, err = s.d.Staff.GrantedPermissions(ctx, accountID); err != nil {
			return Hidden{}, fmt.Errorf("notification: the permissions of %s: %w", accountID, err)
		}
	}
	h := Hidden{Types: notification.HiddenTypes(granted)}
	if s.d.Announcements != nil {
		var err error
		if h.ShownAnnouncements, err = s.d.Announcements.ShownAnnouncementIDs(ctx); err != nil {
			return Hidden{}, fmt.Errorf("notification: the announcements shown: %w", err)
		}
	}
	return h, nil
}

// MarkRead marks accountID's notifications up to and including lastReadID as read, and
// returns the marker. The marker only moves forward: marking an older notification changes
// nothing, so clients that report their positions concurrently never move it back.
// lastReadID must be one of the account's notifications (ErrNotFound otherwise).
func (s *Service) MarkRead(ctx context.Context, accountID, lastReadID string) (notification.ReadMarker, error) {
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.own(ctx, tx, accountID, lastReadID); err != nil {
			return err
		}
		_, err := s.d.Repository.AdvanceReadMarker(ctx, tx, notification.ReadMarker{AccountID: accountID, LastReadID: lastReadID})
		return err
	})
	if err != nil {
		return notification.ReadMarker{}, err
	}
	return s.ReadMarker(ctx, accountID)
}

// Dismiss dismisses one of accountID's notifications: it leaves the list and the unread count.
// Dismissing it again changes nothing; a notification of another account fails with ErrNotFound.
func (s *Service) Dismiss(ctx context.Context, accountID, notificationID string) error {
	d := notification.Dismissal{NotificationID: notificationID, DismissedAt: s.d.Clock.Now()}
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if _, err := s.own(ctx, tx, accountID, notificationID); err != nil {
			return err
		}
		_, err := s.d.Repository.InsertDismissal(ctx, tx, d)
		return err
	})
}

// own returns the notification id when it belongs to accountID.
func (s *Service) own(ctx context.Context, tx uow.Tx, accountID, id string) (notification.Notification, error) {
	n, err := s.d.Repository.Notification(ctx, tx, id)
	if err != nil {
		return notification.Notification{}, err
	}
	if n.RecipientAccountID != accountID {
		return notification.Notification{}, fmt.Errorf("notification %s of %s: %w", id, accountID, ErrNotFound)
	}
	return n, nil
}

// StatusNotified reports inside tx whether a notification is about a status, which keeps a
// remote status from the retention purge (ADR-0034).
func (s *Service) StatusNotified(ctx context.Context, tx uow.Tx, statusID string) (bool, error) {
	return s.d.Repository.StatusHasNotifications(ctx, tx, statusID)
}
