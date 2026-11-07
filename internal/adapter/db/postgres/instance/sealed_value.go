// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/instancegen"
	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

// SealedColumns returns the sealed columns of the module's tables that `my kek rewrap` re-wraps.
func SealedColumns(d *postgres.DB) []postgres.SealedColumn {
	db := &moduleDB{d}
	return []postgres.SealedColumn{
		{
			Name: "instance_secrets.sealed_secret",
			List: func(ctx context.Context, after string, limit int) ([]rewrap.Value, error) {
				pageLimit, err := postgres.ToInt32(limit, "page limit")
				if err != nil {
					return nil, err
				}
				rows, err := db.reader().ListSealedInstanceSecrets(ctx, instancegen.ListSealedInstanceSecretsParams{After: postgres.ToNullString(after), PageLimit: pageLimit})
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
				return q.ReplaceSealedInstanceSecrets(ctx, instancegen.ReplaceSealedInstanceSecretsParams{Sealed: sealed, RowID: rowID, OldSealed: old})
			},
		},
	}
}
