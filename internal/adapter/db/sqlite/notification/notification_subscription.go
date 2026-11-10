// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/notificationgen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

var _ appnotification.SubscriptionRepository = (*Repository)(nil)

// InsertSubscription implements notification.SubscriptionRepository.
func (r *Repository) InsertSubscription(ctx context.Context, tx uow.Tx, s notification.Subscription) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertPostSubscription(ctx, notificationgen.InsertPostSubscriptionParams{
		AccountID: s.AccountID, TargetAccountID: s.TargetAccountID, CreatedAt: sqlite.ToMillis(s.CreatedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert post subscription")
}

// DeleteSubscription implements notification.SubscriptionRepository.
func (r *Repository) DeleteSubscription(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeletePostSubscription(ctx, notificationgen.DeletePostSubscriptionParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return n > 0, sqlite.Wrap(err, "delete post subscription")
}

// CountSubscriptions implements notification.SubscriptionRepository.
func (r *Repository) CountSubscriptions(ctx context.Context, tx uow.Tx, accountID string) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountPostSubscriptions(ctx, accountID)
	return n, sqlite.Wrap(err, "count post subscriptions")
}

// Subscribed implements notification.SubscriptionRepository.
func (r *Repository) Subscribed(ctx context.Context, accountID, targetAccountID string) (bool, error) {
	ok, err := r.db.reader().PostSubscriptionExists(ctx, notificationgen.PostSubscriptionExistsParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return ok, sqlite.Wrap(err, "check post subscription")
}

// SubscribedAmong implements notification.SubscriptionRepository.
func (r *Repository) SubscribedAmong(ctx context.Context, accountID string, targetIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(targetIDs) == 0 {
		return out, nil
	}
	list, err := sqlite.JSONList(targetIDs)
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListSubscribedTargets(ctx, notificationgen.ListSubscribedTargetsParams{AccountID: accountID, TargetIds: list})
	if err != nil {
		return nil, sqlite.Wrap(err, "list subscribed targets")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// HasSubscribers implements notification.SubscriptionRepository.
func (r *Repository) HasSubscribers(ctx context.Context, tx uow.Tx, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.HasPostSubscribers(ctx, targetAccountID)
	return ok, sqlite.Wrap(err, "check post subscribers")
}

// ListSubscriptions implements notification.SubscriptionRepository.
func (r *Repository) ListSubscriptions(ctx context.Context, accountID string, before appnotification.SubscriptionCursor, limit int) ([]notification.Subscription, error) {
	rows, err := r.db.reader().ListPostSubscriptionsPage(ctx, notificationgen.ListPostSubscriptionsPageParams{
		AccountID: accountID, BeforeTargetAccountID: before.TargetAccountID, BeforeCreatedAt: sqlite.ToMillis(before.CreatedAt),
		PageLimit: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list post subscriptions")
	}
	out := make([]notification.Subscription, len(rows))
	for i, row := range rows {
		out[i] = notification.Subscription{AccountID: row.AccountID, TargetAccountID: row.TargetAccountID, CreatedAt: sqlite.FromMillis(row.CreatedAt)}
	}
	return out, nil
}

// CountSubscribers implements notification.SubscriptionRepository.
func (r *Repository) CountSubscribers(ctx context.Context, targetAccountID string) (int64, error) {
	n, err := r.db.reader().CountPostSubscribers(ctx, targetAccountID)
	return n, sqlite.Wrap(err, "count post subscribers")
}

// ListSubscribers implements notification.SubscriptionRepository.
func (r *Repository) ListSubscribers(ctx context.Context, targetAccountID, after string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListPostSubscribersPage(ctx, notificationgen.ListPostSubscribersPageParams{
		TargetAccountID: targetAccountID, After: after, PageLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list post subscribers")
}

// InsertSubscriptionRequest implements notification.SubscriptionRepository.
func (r *Repository) InsertSubscriptionRequest(ctx context.Context, tx uow.Tx, s notification.SubscriptionRequest) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertPostSubscriptionRequest(ctx, notificationgen.UpsertPostSubscriptionRequestParams{
		AccountID: s.AccountID, TargetAccountID: s.TargetAccountID, CreatedAt: sqlite.ToMillis(s.CreatedAt),
	}), "insert post subscription request")
}

// DeleteSubscriptionRequest implements notification.SubscriptionRepository.
func (r *Repository) DeleteSubscriptionRequest(ctx context.Context, tx uow.Tx, accountID, targetAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.DeletePostSubscriptionRequest(ctx, notificationgen.DeletePostSubscriptionRequestParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return n > 0, sqlite.Wrap(err, "delete post subscription request")
}

// SubscriptionRequested implements notification.SubscriptionRepository.
func (r *Repository) SubscriptionRequested(ctx context.Context, accountID, targetAccountID string) (bool, error) {
	ok, err := r.db.reader().PostSubscriptionRequestExists(ctx, notificationgen.PostSubscriptionRequestExistsParams{AccountID: accountID, TargetAccountID: targetAccountID})
	return ok, sqlite.Wrap(err, "check post subscription request")
}
