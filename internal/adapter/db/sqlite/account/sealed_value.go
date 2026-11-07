// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

// SealedColumns returns the sealed columns of the module's tables that `my kek rewrap` re-wraps.
func SealedColumns(d *sqlite.DB) []sqlite.SealedColumn {
	db := &moduleDB{d}
	return []sqlite.SealedColumn{
		{
			Name: "local_account_private_keys.private_key_enc",
			List: func(ctx context.Context, after string, limit int) ([]rewrap.Value, error) {
				rows, err := db.reader().ListSealedAccountPrivateKeys(ctx, accountgen.ListSealedAccountPrivateKeysParams{After: after, PageLimit: int64(limit)})
				var out []rewrap.Value
				for _, r := range rows {
					out = append(out, rewrap.Value{RowID: r.RowID, Sealed: r.Sealed})
				}
				return out, err
			},
			Replace: func(ctx context.Context, tx uow.Tx, rowID string, old, sealed []byte) (int64, error) {
				q, err := db.queries(tx)
				if err != nil {
					return 0, err
				}
				return q.ReplaceSealedAccountPrivateKeys(ctx, accountgen.ReplaceSealedAccountPrivateKeysParams{Sealed: sealed, RowID: rowID, OldSealed: old})
			},
		},
	}
}
