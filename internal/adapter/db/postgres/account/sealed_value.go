// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

// SealedColumns returns the sealed columns of the module's tables that `my kek rewrap` re-wraps.
func SealedColumns(d *postgres.DB) []postgres.SealedColumn {
	db := &moduleDB{d}
	return []postgres.SealedColumn{
		{
			Name: "local_account_private_keys.private_key_enc",
			List: func(ctx context.Context, after string, limit int) ([]rewrap.Value, error) {
				pageLimit, err := postgres.ToInt32(limit, "page limit")
				if err != nil {
					return nil, err
				}
				rows, err := db.reader().ListSealedAccountPrivateKeys(ctx, accountgen.ListSealedAccountPrivateKeysParams{After: postgres.ToNullString(after), PageLimit: pageLimit})
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
