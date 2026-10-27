// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"fmt"
	"time"

	appdiagnostics "github.com/mycroblog/my/internal/app/diagnostics"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// The admin_error_group notifications (ADR-0103 §8, data-model §4.7): a new error group, or a
// resolved one that regressed, tells the holders of manage_errors, at most
// notification.ErrorGroupHourlyCap times an hour for the whole instance; the rest of the hour
// fold into one notification of notification.ErrorGroupOthers, whose count the list reads from
// the hour's notices. A group whose occurrences so far came only from web reports sent without
// a session notifies when the first one with a session confirms it, and a regression from such
// a report notifies nobody, so that anonymous reports never reach the staff. A muted group
// notifies nobody.

// registerErrorGroupProjectors registers the projectors of the diagnostics events on r.
func (p *moderatorProjector) registerErrorGroupProjectors(r *uow.Projectors) {
	r.Register(appdiagnostics.EventGroupCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		g := e.(appdiagnostics.GroupCreated).Group
		if g.Unauthenticated {
			return nil
		}
		return p.notifyErrorGroup(ctx, tx, notification.ErrorGroupCreated, g.ID, g.ID, g.FirstSeenAt)
	})
	r.Register(appdiagnostics.EventGroupConfirmed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appdiagnostics.GroupConfirmed)
		if ev.Head.Muted() {
			return nil
		}
		return p.notifyErrorGroup(ctx, tx, notification.ErrorGroupCreated, ev.Group.ID, ev.Group.ID, ev.ConfirmedAt)
	})
	r.Register(appdiagnostics.EventGroupRegressed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(appdiagnostics.GroupRegressed)
		if ev.Unauthenticated {
			return nil
		}
		return p.notifyErrorGroup(ctx, tx, notification.ErrorGroupRegressed, ev.Group.ID, ev.Event.ID, ev.Event.OccurredAt)
	})
}

// notifyErrorGroup counts a notice of the group in the hour of at and notifies of it while the
// hour is under the cap, or makes the notification of the others at the first notice beyond
// it. keyID is the ID that the dedupe key of the notification of the group names.
func (p *moderatorProjector) notifyErrorGroup(ctx context.Context, tx uow.Tx, change notification.ErrorGroupChange, groupID, keyID string, at time.Time) error {
	if p.d.Staff == nil {
		return nil
	}
	hour := notification.ErrorGroupHour(at)
	n, err := p.d.Repository.AddErrorGroupNotice(ctx, tx, hour)
	if err != nil {
		return fmt.Errorf("notification: count the error group notice of %s: %w", groupID, err)
	}
	switch {
	case n <= notification.ErrorGroupHourlyCap:
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminErrorGroup, ErrorGroupChange: change, ErrorGroupID: groupID,
			DedupeKey: notification.AdminErrorGroupKey(change, keyID), CreatedAt: at,
		})
	case n == notification.ErrorGroupHourlyCap+1:
		return p.notify(ctx, tx, notification.Notification{
			Type: notification.TypeAdminErrorGroup, ErrorGroupChange: notification.ErrorGroupOthers, ErrorGroupHour: hour,
			DedupeKey: notification.AdminErrorGroupOthersKey(hour), CreatedAt: at,
		})
	}
	return nil
}

// ErrorGroupOthers returns how many new and regressed groups the admin_error_group of n folds:
// the notices of its hour beyond the cap, 0 for any other notification.
func (s *Service) ErrorGroupOthers(ctx context.Context, n notification.Notification) (int64, error) {
	if n.Type != notification.TypeAdminErrorGroup || n.ErrorGroupChange != notification.ErrorGroupOthers {
		return 0, nil
	}
	notices, err := s.d.Repository.ErrorGroupNotices(ctx, n.ErrorGroupHour)
	if err != nil {
		return 0, fmt.Errorf("notification: the error group notices of %s: %w", n.ErrorGroupHour, err)
	}
	return max(notices-notification.ErrorGroupHourlyCap, 0), nil
}
