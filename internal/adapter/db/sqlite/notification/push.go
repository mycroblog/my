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

var _ appnotification.PushRepository = (*Repository)(nil)

// SavePushSubscription implements notification.PushRepository.
func (r *Repository) SavePushSubscription(ctx context.Context, tx uow.Tx, s notification.PushSubscription) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	replaced := notificationgen.DeletePushSubscriptionAlertsReplacedByParams{OauthTokenFamilyID: s.TokenFamilyID, Endpoint: s.Endpoint}
	if err := q.DeletePushSubscriptionAlertsReplacedBy(ctx, replaced); err != nil {
		return sqlite.Wrap(err, "delete replaced push subscription alerts")
	}
	if err := q.DeletePushSubscriptionsReplacedBy(ctx, notificationgen.DeletePushSubscriptionsReplacedByParams(replaced)); err != nil {
		return sqlite.Wrap(err, "delete replaced push subscriptions")
	}
	err = q.InsertPushSubscription(ctx, notificationgen.InsertPushSubscriptionParams{
		ID: s.ID, AccountID: s.AccountID, OauthTokenFamilyID: s.TokenFamilyID, Endpoint: s.Endpoint,
		KeyP256dh: s.KeyP256DH, KeyAuth: s.KeyAuth, CreatedAt: sqlite.ToMillis(s.CreatedAt), UpdatedAt: sqlite.ToMillis(s.UpdatedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert push subscription")
	}
	for _, t := range s.Alerts {
		err := q.InsertPushSubscriptionAlert(ctx, notificationgen.InsertPushSubscriptionAlertParams{SubscriptionID: s.ID, NotificationType: string(t)})
		if err != nil {
			return sqlite.Wrap(err, "insert push subscription alert")
		}
	}
	return nil
}

// DeletePushSubscription implements notification.PushRepository.
func (r *Repository) DeletePushSubscription(ctx context.Context, tx uow.Tx, id string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	if err := q.DeletePushSubscriptionAlerts(ctx, id); err != nil {
		return false, sqlite.Wrap(err, "delete push subscription alerts")
	}
	rows, err := q.DeletePushSubscription(ctx, id)
	if err != nil {
		return false, sqlite.Wrap(err, "delete push subscription")
	}
	return rows == 1, nil
}

// PushSubscriptionIDs implements notification.PushRepository.
func (r *Repository) PushSubscriptionIDs(ctx context.Context, tx uow.Tx, accountID string, t notification.Type) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListPushSubscriptionIDsForAlert(ctx, notificationgen.ListPushSubscriptionIDsForAlertParams{AccountID: accountID, NotificationType: string(t)})
	return ids, sqlite.Wrap(err, "list push subscriptions")
}

// FindPushSubscription implements notification.PushRepository.
func (r *Repository) FindPushSubscription(ctx context.Context, id string) (notification.PushSubscription, error) {
	row, err := r.db.reader().GetPushSubscription(ctx, id)
	if err != nil {
		return notification.PushSubscription{}, sqlite.NotFound(err, appnotification.ErrNotFound, "push subscription "+id)
	}
	return r.pushSubscription(ctx, row)
}

// FindPushSubscriptionOfFamily implements notification.PushRepository.
func (r *Repository) FindPushSubscriptionOfFamily(ctx context.Context, tokenFamilyID string) (notification.PushSubscription, error) {
	row, err := r.db.reader().GetPushSubscriptionOfTokenFamily(ctx, tokenFamilyID)
	if err != nil {
		return notification.PushSubscription{}, sqlite.NotFound(err, appnotification.ErrNotFound, "push subscription of "+tokenFamilyID)
	}
	return r.pushSubscription(ctx, row)
}

// pushSubscription reads the alerts of row and puts them in the order of notification.Types.
func (r *Repository) pushSubscription(ctx context.Context, row notificationgen.PushSubscription) (notification.PushSubscription, error) {
	types, err := r.db.reader().ListPushSubscriptionAlerts(ctx, row.ID)
	if err != nil {
		return notification.PushSubscription{}, sqlite.Wrap(err, "list push subscription alerts")
	}
	alerts := make([]notification.Type, len(types))
	for i, t := range types {
		alerts[i] = notification.Type(t)
	}
	alerts, _ = notification.NormalizeAlerts(alerts)
	return notification.PushSubscription{
		ID: row.ID, AccountID: row.AccountID, TokenFamilyID: row.OauthTokenFamilyID, Endpoint: row.Endpoint,
		KeyP256DH: row.KeyP256dh, KeyAuth: row.KeyAuth, Alerts: alerts, CreatedAt: sqlite.FromMillis(row.CreatedAt), UpdatedAt: sqlite.FromMillis(row.UpdatedAt),
	}, nil
}

// PushSubscriptionIDsOf implements notification.PushRepository.
func (r *Repository) PushSubscriptionIDsOf(ctx context.Context, tx uow.Tx, accountIDs []string, t notification.Type) (map[string][]string, error) {
	out := make(map[string][]string, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(accountIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListPushSubscriptionIDsForAlertOf(ctx, notificationgen.ListPushSubscriptionIDsForAlertOfParams{AccountIds: list, NotificationType: string(t)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list push subscriptions")
	}
	for _, row := range rows {
		out[row.AccountID] = append(out[row.AccountID], row.ID)
	}
	return out, nil
}
