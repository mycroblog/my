// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/notificationgen"
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
	lim := postgres.BatchLimit(limit)
	list, err := q.ListNotificationIDsToErase(ctx, notificationgen.ListNotificationIDsToEraseParams{
		AccountID: accountID, UserID: postgres.ToNullString(userID), Lim: lim,
	})
	if err != nil {
		return 0, postgres.Wrap(err, "list notifications to erase")
	}
	most := int64(len(list))
	if len(list) > 0 {
		ids := list
		if err := postgres.RunSteps(ctx, ids, "erase notifications", q.EraseNotificationDismissals, q.EraseNotificationFilterings, q.EraseNotifications); err != nil {
			return 0, err
		}
	}
	if err := q.EraseNotificationReadMarker(ctx, accountID); err != nil {
		return 0, postgres.Wrap(err, "erase notification read marker")
	}
	if err := q.EraseNotificationSettingsOf(ctx, accountID); err != nil {
		return 0, postgres.Wrap(err, "erase notification settings")
	}
	if err := postgres.RunSteps(ctx, accountID, "erase push subscriptions", q.ErasePushSubscriptionAlertsOf, q.ErasePushSubscriptionsOf); err != nil {
		return 0, err
	}
	viewer, err := q.EraseNotificationExclusionsAsViewer(ctx, notificationgen.EraseNotificationExclusionsAsViewerParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase notification exclusions")
	}
	target, err := q.EraseNotificationExclusionsAsTarget(ctx, notificationgen.EraseNotificationExclusionsAsTargetParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase notification exclusions")
	}
	domains, err := q.EraseNotificationDomainExclusionsOf(ctx, notificationgen.EraseNotificationDomainExclusionsOfParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase notification domain exclusions")
	}
	if err := postgres.RunSteps(ctx, accountID, "erase post subscriptions", q.ErasePostSubscriptionsOf, q.ErasePostSubscriptionRequestsOf); err != nil {
		return 0, err
	}
	if err := postgres.RunSteps(ctx, accountID, "erase conversation mutes", q.EraseConversationMutesOf); err != nil {
		return 0, err
	}
	subscribers, err := q.ErasePostSubscriptionsAsTarget(ctx, notificationgen.ErasePostSubscriptionsAsTargetParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase post subscriptions")
	}
	requesters, err := q.ErasePostSubscriptionRequestsAsTarget(ctx, notificationgen.ErasePostSubscriptionRequestsAsTargetParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "erase post subscription requests")
	}
	policy, err := erasePolicyOf(ctx, q, accountID, lim)
	if err != nil {
		return 0, err
	}
	return max(most, viewer, target, domains, subscribers, requesters, policy), nil
}

// EraseStatusNotifications implements notification.ErasureRepository.
func (r *Repository) EraseStatusNotifications(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids := statusIDs
	pairs, err := filteredPairsOfStatuses(ctx, q, ids)
	if err != nil {
		return err
	}
	err = postgres.RunSteps(ctx, ids, "erase notifications of statuses",
		q.EraseNotificationDismissalsOfStatuses, q.EraseNotificationFilteringsOfStatuses, q.EraseNotificationsOfStatuses, q.EraseDeletedStatusesOfStatuses)
	if err != nil || len(pairs) == 0 {
		return err
	}
	// The requests that held the erased notifications count the rest.
	return refreshRequests(ctx, q, pairs)
}
