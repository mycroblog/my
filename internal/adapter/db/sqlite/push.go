// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

var _ appnotification.PushRepository = (*NotificationRepository)(nil)

// SavePushSubscription implements notification.PushRepository.
func (r *NotificationRepository) SavePushSubscription(ctx context.Context, tx uow.Tx, s notification.PushSubscription) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	replaced := sqlitegen.DeletePushSubscriptionAlertsReplacedByParams{OauthTokenFamilyID: s.TokenFamilyID, Endpoint: s.Endpoint}
	if err := q.DeletePushSubscriptionAlertsReplacedBy(ctx, replaced); err != nil {
		return wrap(err, "delete replaced push subscription alerts")
	}
	if err := q.DeletePushSubscriptionsReplacedBy(ctx, sqlitegen.DeletePushSubscriptionsReplacedByParams(replaced)); err != nil {
		return wrap(err, "delete replaced push subscriptions")
	}
	err = q.InsertPushSubscription(ctx, sqlitegen.InsertPushSubscriptionParams{
		ID: s.ID, AccountID: s.AccountID, OauthTokenFamilyID: s.TokenFamilyID, Endpoint: s.Endpoint,
		KeyP256dh: s.KeyP256DH, KeyAuth: s.KeyAuth, CreatedAt: toMillis(s.CreatedAt), UpdatedAt: toMillis(s.UpdatedAt),
	})
	if err != nil {
		return wrap(err, "insert push subscription")
	}
	for _, t := range s.Alerts {
		err := q.InsertPushSubscriptionAlert(ctx, sqlitegen.InsertPushSubscriptionAlertParams{SubscriptionID: s.ID, NotificationType: string(t)})
		if err != nil {
			return wrap(err, "insert push subscription alert")
		}
	}
	return nil
}

// DeletePushSubscription implements notification.PushRepository.
func (r *NotificationRepository) DeletePushSubscription(ctx context.Context, tx uow.Tx, id string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	if err := q.DeletePushSubscriptionAlerts(ctx, id); err != nil {
		return false, wrap(err, "delete push subscription alerts")
	}
	rows, err := q.DeletePushSubscription(ctx, id)
	if err != nil {
		return false, wrap(err, "delete push subscription")
	}
	return rows == 1, nil
}

// PushSubscriptionIDs implements notification.PushRepository.
func (r *NotificationRepository) PushSubscriptionIDs(ctx context.Context, tx uow.Tx, accountID string, t notification.Type) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListPushSubscriptionIDsForAlert(ctx, sqlitegen.ListPushSubscriptionIDsForAlertParams{AccountID: accountID, NotificationType: string(t)})
	return ids, wrap(err, "list push subscriptions")
}

// FindPushSubscription implements notification.PushRepository.
func (r *NotificationRepository) FindPushSubscription(ctx context.Context, id string) (notification.PushSubscription, error) {
	row, err := r.db.reader().GetPushSubscription(ctx, id)
	if err != nil {
		return notification.PushSubscription{}, notFound(err, appnotification.ErrNotFound, "push subscription "+id)
	}
	return r.pushSubscription(ctx, row)
}

// FindPushSubscriptionOfFamily implements notification.PushRepository.
func (r *NotificationRepository) FindPushSubscriptionOfFamily(ctx context.Context, tokenFamilyID string) (notification.PushSubscription, error) {
	row, err := r.db.reader().GetPushSubscriptionOfTokenFamily(ctx, tokenFamilyID)
	if err != nil {
		return notification.PushSubscription{}, notFound(err, appnotification.ErrNotFound, "push subscription of "+tokenFamilyID)
	}
	return r.pushSubscription(ctx, row)
}

// pushSubscription reads the alerts of row and puts them in the order of notification.Types.
func (r *NotificationRepository) pushSubscription(ctx context.Context, row sqlitegen.PushSubscription) (notification.PushSubscription, error) {
	types, err := r.db.reader().ListPushSubscriptionAlerts(ctx, row.ID)
	if err != nil {
		return notification.PushSubscription{}, wrap(err, "list push subscription alerts")
	}
	alerts := make([]notification.Type, len(types))
	for i, t := range types {
		alerts[i] = notification.Type(t)
	}
	alerts, _ = notification.NormalizeAlerts(alerts)
	return notification.PushSubscription{
		ID: row.ID, AccountID: row.AccountID, TokenFamilyID: row.OauthTokenFamilyID, Endpoint: row.Endpoint,
		KeyP256DH: row.KeyP256dh, KeyAuth: row.KeyAuth, Alerts: alerts, CreatedAt: fromMillis(row.CreatedAt), UpdatedAt: fromMillis(row.UpdatedAt),
	}, nil
}
