// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/app/rewrap"
	"github.com/mycroblog/my/internal/app/uow"
)

// SealedColumn is a sealed column that `my kek rewrap` re-wraps (rewrap.Store), as the module that
// owns its table reads and writes it: the sub-packages of the modules return theirs, and package
// rewrap of this adapter gathers them into one store.
type SealedColumn struct {
	// Name is the column, as "table.column".
	Name string
	// List returns at most limit of the column's values whose row ID is greater than after, in
	// the order of the row IDs.
	List func(ctx context.Context, after string, limit int) ([]rewrap.Value, error)
	// Replace writes sealed in place of old in the row, in tx, and returns the number of rows it
	// changed: 0 when the row is gone or holds another value.
	Replace func(ctx context.Context, tx uow.Tx, rowID string, old, sealed []byte) (int64, error)
}
