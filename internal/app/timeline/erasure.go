// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
)

// ErasureRepository is the part of the timeline module's persistence that the erasure of
// deleted accounts uses (data-model §8). The database adapters' timeline repositories implement
// it and pass testkit.AccountErasureContract.
type ErasureRepository interface {
	// EraseFeedsOf deletes up to limit of each of: the entries by the account or of its
	// statuses (with their recipients and hashtags), the direct entries to it, the follows into
	// and out of it, the exclusions by and of it, its domain mutes, its lists (with their names
	// and members), and its membership in the lists of others. It returns the most rows that one
	// kind lost.
	EraseFeedsOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error)
}

// Eraser erases a deleted local account's rows of the timeline projections and its lists
// (data-model §8).
// The restrictions of moderation (feed_author_restrictions) stay with the moderation records.
type Eraser struct {
	uow     uow.UnitOfWork
	erasure ErasureRepository
}

// NewEraser returns the timeline module's eraser.
func NewEraser(u uow.UnitOfWork, r ErasureRepository) *Eraser {
	return &Eraser{uow: u, erasure: r}
}

// EraseAccountData implements account.DataEraser.
func (e *Eraser) EraseAccountData(ctx context.Context, t appaccount.ErasureTarget, _ string, limit int) (appaccount.ErasureStep, error) {
	var n int64
	err := e.uow.Tx(ctx, func(tx uow.Tx) error {
		var err error
		n, err = e.erasure.EraseFeedsOf(ctx, tx, t.AccountID, limit)
		return err
	})
	return appaccount.ErasureStep{Done: n < int64(limit)}, err
}
