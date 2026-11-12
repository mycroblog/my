// SPDX-License-Identifier: AGPL-3.0-only

package gathering

import (
	"context"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
)

// Eraser empties the titles and descriptions, and deletes the places, of the revisions that a
// deleted local account wrote (ADR-0161 §14), and forgets the reminders it chose;
// the events, the rest of their revisions, their dates, and their answers stay.
type Eraser struct {
	uow  uow.UnitOfWork
	repo Repository
}

// NewEraser returns the gathering module's eraser.
func NewEraser(u uow.UnitOfWork, r Repository) *Eraser { return &Eraser{uow: u, repo: r} }

var _ account.DataEraser = (*Eraser)(nil)

// EraseAccountData implements account.DataEraser: one batch of the revisions that the account
// wrote, in one transaction.
func (e *Eraser) EraseAccountData(ctx context.Context, t account.ErasureTarget, _ string, limit int) (account.ErasureStep, error) {
	var n int
	err := e.uow.Tx(ctx, func(tx uow.Tx) error {
		var err error
		if n, err = e.repo.EraseRevisions(ctx, tx, t.AccountID, limit); err != nil || n >= limit {
			return err
		}
		return e.repo.DeleteReminders(ctx, tx, t.AccountID)
	})
	return account.ErasureStep{Done: err == nil && n < limit}, err
}
