// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"

	"github.com/mycroblog/my/internal/app/uow"
)

// localityBatch is an Accounts that tells the local accounts among many at once, as
// *appaccount.Service does.
type localityBatch interface {
	LocalAmong(ctx context.Context, tx uow.Tx, ids []string) (map[string]bool, error)
}

// localAmong reports, inside tx, which of ids are local accounts: with one read when accounts
// can, one by one otherwise.
func localAmong(ctx context.Context, tx uow.Tx, accounts Accounts, ids []string) (map[string]bool, error) {
	if b, ok := accounts.(localityBatch); ok {
		return b.LocalAmong(ctx, tx, ids)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		local, err := accounts.IsLocal(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out[id] = local
	}
	return out, nil
}
