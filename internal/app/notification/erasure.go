// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
)

// ErasureRepository is the part of the notification module's persistence that the erasure of
// deleted accounts uses (data-model §8). The database adapters' notification repositories
// implement it and pass testkit.AccountErasureContract.
type ErasureRepository interface {
	// EraseNotificationsOf deletes up to limit notifications to or from the account or about its
	// user, with their dismissals, its read marker, notification settings, and Web Push
	// subscriptions, and up to limit of each of the exclusions by and of it. It returns the most rows that one kind lost.
	EraseNotificationsOf(ctx context.Context, tx uow.Tx, accountID, userID string, limit int) (int64, error)
	// EraseStatusNotifications deletes the notifications about the statuses, with their
	// dismissals, and the statuses' rows of notification_deleted_statuses.
	EraseStatusNotifications(ctx context.Context, tx uow.Tx, statusIDs []string) error
}

// Eraser erases a deleted local account's notifications (data-model §8): those it received,
// those of its actions, those about its user, and those about its statuses as the content
// module's eraser purges them.
type Eraser struct {
	uow     uow.UnitOfWork
	erasure ErasureRepository
}

// NewEraser returns the notification module's eraser.
func NewEraser(u uow.UnitOfWork, r ErasureRepository) *Eraser {
	return &Eraser{uow: u, erasure: r}
}

// EraseAccountData implements account.DataEraser.
func (e *Eraser) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, _ string, limit int) (appaccount.ErasureStep, error) {
	var n int64
	err := e.uow.Tx(ctx, func(tx uow.Tx) error {
		var err error
		n, err = e.erasure.EraseNotificationsOf(ctx, tx, t.AccountID, t.UserID, limit)
		return err
	})
	return appaccount.ErasureStep{Done: n < int64(limit)}, err
}

// EraseStatusNotifications implements content.StatusNotificationEraser inside tx.
func (e *Eraser) EraseStatusNotifications(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	return e.erasure.EraseStatusNotifications(ctx, tx, statusIDs)
}
