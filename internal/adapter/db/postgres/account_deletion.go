// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// ListDueDeletions implements account.Repository.
func (r *AccountRepository) ListDueDeletions(ctx context.Context, now time.Time, limit int) ([]account.LifecycleState, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDueDeletions(ctx, postgresgen.ListDueDeletionsParams{Now: now, Lim: lim})
	if err != nil {
		return nil, wrap(err, "list due deletions")
	}
	out := make([]account.LifecycleState, len(rows))
	for i, row := range rows {
		out[i] = lifecycleState(row)
	}
	return out, nil
}

// ListDueDeactivations implements account.Repository.
func (r *AccountRepository) ListDueDeactivations(ctx context.Context, cutoff time.Time, limit int) ([]account.LifecycleState, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDueDeactivations(ctx, postgresgen.ListDueDeactivationsParams{Cutoff: cutoff, Lim: lim})
	if err != nil {
		return nil, wrap(err, "list due deactivations")
	}
	out := make([]account.LifecycleState, len(rows))
	for i, row := range rows {
		out[i] = lifecycleState(row)
	}
	return out, nil
}

// FindLatestReactivation implements account.Repository.
func (r *AccountRepository) FindLatestReactivation(ctx context.Context, accountID string) (time.Time, error) {
	at, err := r.db.reader().FindLatestReactivation(ctx, accountID)
	if err != nil {
		return time.Time{}, notFound(err, appaccount.ErrNotFound, "reactivation of "+accountID)
	}
	return utc(at), nil
}

// FindLatestReactivationTx implements account.Repository.
func (r *AccountRepository) FindLatestReactivationTx(ctx context.Context, tx uow.Tx, accountID string) (time.Time, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return time.Time{}, err
	}
	at, err := q.FindLatestReactivation(ctx, accountID)
	if err != nil {
		return time.Time{}, notFound(err, appaccount.ErrNotFound, "reactivation of "+accountID)
	}
	return utc(at), nil
}
