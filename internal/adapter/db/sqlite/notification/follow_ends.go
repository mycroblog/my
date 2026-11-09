// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
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
	pairs, err := sqlite.JSONPairs(accountIDs, targetIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeletePostSubscriptionRequests(ctx, pairs), "delete post subscription requests")
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
	pairs, err := sqlite.JSONPairs(accountIDs, targetIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeletePostSubscriptions(ctx, pairs), "delete post subscriptions")
}
