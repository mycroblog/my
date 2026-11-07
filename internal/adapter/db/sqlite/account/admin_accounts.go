// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
)

var _ appaccount.SearchRepository = (*Repository)(nil)

// SearchAccountIDs implements account.SearchRepository.
func (r *Repository) SearchAccountIDs(ctx context.Context, q appaccount.SearchQuery, limit int) ([]string, error) {
	ids, err := r.db.reader().SearchAccountIDs(ctx, accountgen.SearchAccountIDsParams{
		Domain: q.Domain, Origin: q.Origin, UsernamePrefix: q.UsernamePrefix, Before: q.Before, Lim: int64(limit),
	})
	return ids, sqlite.Wrap(err, "search accounts")
}

// ListAccountIDsInLifecycleState implements account.SearchRepository.
func (r *Repository) ListAccountIDsInLifecycleState(ctx context.Context, state account.LifecycleStateName, before string, limit int) ([]string, error) {
	ids, err := r.db.reader().ListAccountIDsInLifecycleState(ctx, accountgen.ListAccountIDsInLifecycleStateParams{
		State: string(state), Before: before, Lim: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list accounts in a lifecycle state")
}
