// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

var _ appnotification.SubscriptionRepository = (*NotificationRepository)(nil)

// InsertSubscription implements notification.SubscriptionRepository.
func (r *NotificationRepository) InsertSubscription(ctx context.Context, tx uow.Tx, s notification.Subscription) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertPostSubscription(ctx, sqlitegen.InsertPostSubscriptionParams{
		AccountID: s.AccountID, TargetAccountID: s.TargetAccountID, CreatedAt: toMillis(s.CreatedAt),
	})
	return n > 0, wrap(err, "insert post subscription")
}

// DeleteSubscription implements notification.SubscriptionRepository.
func (r *NotificationRepository) DeleteSubscription(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeletePostSubscription(ctx, sqlitegen.DeletePostSubscriptionParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return n > 0, wrap(err, "delete post subscription")
}

// CountSubscriptions implements notification.SubscriptionRepository.
func (r *NotificationRepository) CountSubscriptions(ctx context.Context, tx uow.Tx, accountID string) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountPostSubscriptions(ctx, accountID)
	return n, wrap(err, "count post subscriptions")
}

// Subscribed implements notification.SubscriptionRepository.
func (r *NotificationRepository) Subscribed(ctx context.Context, accountID, targetAccountID string) (bool, error) {
	ok, err := r.db.reader().PostSubscriptionExists(ctx, sqlitegen.PostSubscriptionExistsParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return ok, wrap(err, "check post subscription")
}

// HasSubscribers implements notification.SubscriptionRepository.
func (r *NotificationRepository) HasSubscribers(ctx context.Context, tx uow.Tx, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.HasPostSubscribers(ctx, targetAccountID)
	return ok, wrap(err, "check post subscribers")
}

// ListSubscriptions implements notification.SubscriptionRepository.
func (r *NotificationRepository) ListSubscriptions(ctx context.Context, accountID string, before appnotification.SubscriptionCursor, limit int) ([]notification.Subscription, error) {
	rows, err := r.db.reader().ListPostSubscriptionsPage(ctx, sqlitegen.ListPostSubscriptionsPageParams{
		AccountID: accountID, BeforeTargetAccountID: before.TargetAccountID, BeforeCreatedAt: toMillis(before.CreatedAt),
		PageLimit: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list post subscriptions")
	}
	out := make([]notification.Subscription, len(rows))
	for i, row := range rows {
		out[i] = notification.Subscription{AccountID: row.AccountID, TargetAccountID: row.TargetAccountID, CreatedAt: fromMillis(row.CreatedAt)}
	}
	return out, nil
}

// ListSubscribers implements notification.SubscriptionRepository.
func (r *NotificationRepository) ListSubscribers(ctx context.Context, targetAccountID, after string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListPostSubscribersPage(ctx, sqlitegen.ListPostSubscribersPageParams{
		TargetAccountID: targetAccountID, After: after, PageLimit: int64(limit),
	})
	return ids, wrap(err, "list post subscribers")
}

// InsertSubscriptionRequest implements notification.SubscriptionRepository.
func (r *NotificationRepository) InsertSubscriptionRequest(ctx context.Context, tx uow.Tx, s notification.SubscriptionRequest) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertPostSubscriptionRequest(ctx, sqlitegen.UpsertPostSubscriptionRequestParams{
		AccountID: s.AccountID, TargetAccountID: s.TargetAccountID, CreatedAt: toMillis(s.CreatedAt),
	}), "insert post subscription request")
}

// DeleteSubscriptionRequest implements notification.SubscriptionRepository.
func (r *NotificationRepository) DeleteSubscriptionRequest(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeletePostSubscriptionRequest(ctx, sqlitegen.DeletePostSubscriptionRequestParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return n > 0, wrap(err, "delete post subscription request")
}

// SubscriptionRequested implements notification.SubscriptionRepository.
func (r *NotificationRepository) SubscriptionRequested(ctx context.Context, accountID, targetAccountID string) (bool, error) {
	ok, err := r.db.reader().PostSubscriptionRequestExists(ctx, sqlitegen.PostSubscriptionRequestExistsParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return ok, wrap(err, "check post subscription request")
}
