// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/notificationgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// DeleteSubscriptionRequests implements notification.Repository.
func (r *Repository) DeleteSubscriptionRequests(ctx context.Context, tx uow.Tx, accountIDs, targetIDs []string) error {
	if len(accountIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeletePostSubscriptionRequests(ctx, notificationgen.DeletePostSubscriptionRequestsParams{AccountIds: accountIDs, TargetIds: targetIDs}), "delete post subscription requests")
}

// DeleteSubscriptions implements notification.Repository.
func (r *Repository) DeleteSubscriptions(ctx context.Context, tx uow.Tx, accountIDs, targetIDs []string) error {
	if len(accountIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeletePostSubscriptions(ctx, notificationgen.DeletePostSubscriptionsParams{AccountIds: accountIDs, TargetIds: targetIDs}), "delete post subscriptions")
}
