// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/accountgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
)

var _ appaccount.SearchRepository = (*Repository)(nil)

// SearchAccountIDs implements account.SearchRepository.
func (r *Repository) SearchAccountIDs(ctx context.Context, q appaccount.SearchQuery, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().SearchAccountIDs(ctx, accountgen.SearchAccountIDsParams{
		Domain: q.Domain, Origin: q.Origin, UsernamePrefix: q.UsernamePrefix, Before: postgres.ToNullString(q.Before), Lim: lim,
	})
	return ids, postgres.Wrap(err, "search accounts")
}

// ListAccountIDsInLifecycleState implements account.SearchRepository.
func (r *Repository) ListAccountIDsInLifecycleState(ctx context.Context, state account.LifecycleStateName, before string, limit int) ([]string, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	ids, err := r.db.reader().ListAccountIDsInLifecycleState(ctx, accountgen.ListAccountIDsInLifecycleStateParams{
		State: string(state), Before: postgres.ToNullString(before), Lim: lim,
	})
	return ids, postgres.Wrap(err, "list accounts in a lifecycle state")
}
