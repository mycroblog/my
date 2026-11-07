// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ appaccount.ErasureRepository = (*Repository)(nil)

// ListAccountsToErase implements account.ErasureRepository.
func (r *Repository) ListAccountsToErase(ctx context.Context, deletedBefore time.Time, after string, limit int) ([]appaccount.ErasureTarget, error) {
	rows, err := r.db.reader().ListAccountsToErase(ctx, accountgen.ListAccountsToEraseParams{
		DeletedBefore: sqlite.ToMillis(deletedBefore), After: after, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list accounts to erase")
	}
	out := make([]appaccount.ErasureTarget, len(rows))
	for i, row := range rows {
		out[i] = appaccount.ErasureTarget{AccountID: row.AccountID, UserID: row.UserID.String, DeletedAt: sqlite.FromMillis(row.Since)}
	}
	return out, nil
}

// EraseProfilesAndKeys implements account.ErasureRepository.
func (r *Repository) EraseProfilesAndKeys(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.RunSteps(ctx, accountID, "erase profiles and keys",
		q.EraseProfileFieldSources, q.EraseProfileFields, q.EraseProfileRevisionEmojis, q.EraseProfileRevisionSources, q.EraseRemoteProfileRevisions,
		q.EraseProfileHead, q.EraseProfileRevisions, q.ErasePrivateKeys, q.EraseKeyRevocations, q.EraseKeys, q.EraseAliases,
		q.EraseRemoteAccountSnapshots, q.EraseRemoteAccountHead, q.EraseRemoteAccountHandleVerifications, q.EraseRemoteAccountHandle)
}

// InsertErasure implements account.ErasureRepository.
func (r *Repository) InsertErasure(ctx context.Context, tx uow.Tx, accountID string, at time.Time) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertAccountErasure(ctx, accountgen.InsertAccountErasureParams{AccountID: accountID, ErasedAt: sqlite.ToMillis(at)}), "insert account erasure")
}

// AccountErased implements account.ErasureRepository.
func (r *Repository) AccountErased(ctx context.Context, accountID string) (bool, error) {
	ok, err := r.db.reader().AccountErased(ctx, accountID)
	return ok, sqlite.Wrap(err, "find account erasure")
}
