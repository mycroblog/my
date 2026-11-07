// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ appnotification.ErasureRepository = (*Repository)(nil)

// EraseNotificationsOf implements notification.ErasureRepository.
func (r *Repository) EraseNotificationsOf(ctx context.Context, tx uow.Tx, accountID, userID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	lim := int64(limit)
	list, err := q.ListNotificationIDsToErase(ctx, notificationgen.ListNotificationIDsToEraseParams{
		AccountID: accountID, UserID: sqlite.ToNullString(userID), Lim: lim,
	})
	if err != nil {
		return 0, sqlite.Wrap(err, "list notifications to erase")
	}
	most := int64(len(list))
	if len(list) > 0 {
		ids, err := sqlite.JSONList(list)
		if err != nil {
			return 0, err
		}
		if err := sqlite.RunSteps(ctx, ids, "erase notifications", q.EraseNotificationDismissals, q.EraseNotifications); err != nil {
			return 0, err
		}
	}
	if err := q.EraseNotificationReadMarker(ctx, accountID); err != nil {
		return 0, sqlite.Wrap(err, "erase notification read marker")
	}
	if err := q.EraseNotificationSettingsOf(ctx, accountID); err != nil {
		return 0, sqlite.Wrap(err, "erase notification settings")
	}
	if err := sqlite.RunSteps(ctx, accountID, "erase push subscriptions", q.ErasePushSubscriptionAlertsOf, q.ErasePushSubscriptionsOf); err != nil {
		return 0, err
	}
	viewer, err := q.EraseNotificationExclusionsAsViewer(ctx, notificationgen.EraseNotificationExclusionsAsViewerParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, sqlite.Wrap(err, "erase notification exclusions")
	}
	target, err := q.EraseNotificationExclusionsAsTarget(ctx, notificationgen.EraseNotificationExclusionsAsTargetParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, sqlite.Wrap(err, "erase notification exclusions")
	}
	domains, err := q.EraseNotificationDomainExclusionsOf(ctx, notificationgen.EraseNotificationDomainExclusionsOfParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, sqlite.Wrap(err, "erase notification domain exclusions")
	}
	if err := sqlite.RunSteps(ctx, accountID, "erase post subscriptions", q.ErasePostSubscriptionsOf, q.ErasePostSubscriptionRequestsOf); err != nil {
		return 0, err
	}
	subscribers, err := q.ErasePostSubscriptionsAsTarget(ctx, notificationgen.ErasePostSubscriptionsAsTargetParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, sqlite.Wrap(err, "erase post subscriptions")
	}
	requesters, err := q.ErasePostSubscriptionRequestsAsTarget(ctx, notificationgen.ErasePostSubscriptionRequestsAsTargetParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, sqlite.Wrap(err, "erase post subscription requests")
	}
	return max(most, viewer, target, domains, subscribers, requesters), nil
}

// EraseStatusNotifications implements notification.ErasureRepository.
func (r *Repository) EraseStatusNotifications(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	return sqlite.RunSteps(ctx, ids, "erase notifications of statuses",
		q.EraseNotificationDismissalsOfStatuses, q.EraseNotificationsOfStatuses, q.EraseDeletedStatusesOfStatuses)
}
