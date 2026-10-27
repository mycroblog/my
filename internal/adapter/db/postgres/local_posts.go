// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// addLocalPosts adds delta to the local_post_counters row of an account. A counter that a
// negative delta would bring to zero is deleted instead, so every row stays positive.
func (d *DB) addLocalPosts(ctx context.Context, tx uow.Tx, accountID string, delta int64) error {
	q, err := d.queries(tx)
	if err != nil {
		return err
	}
	switch {
	case delta > 0:
		return wrap(q.AddLocalPosts(ctx, postgresgen.AddLocalPostsParams{AccountID: accountID, Added: delta}), "add local posts")
	case delta < 0:
		removed := -delta
		if err := q.DeleteSpentLocalPostCounter(ctx, postgresgen.DeleteSpentLocalPostCounterParams{AccountID: accountID, Removed: removed}); err != nil {
			return wrap(err, "delete spent local post counter")
		}
		return wrap(q.SubtractLocalPosts(ctx, postgresgen.SubtractLocalPostsParams{Removed: removed, AccountID: accountID}), "subtract local posts")
	}
	return nil
}

// AddLocalPosts implements content.Repository.
func (r *StatusRepository) AddLocalPosts(ctx context.Context, tx uow.Tx, accountID string, delta int64) error {
	return r.db.addLocalPosts(ctx, tx, accountID, delta)
}

// RebuildLocalPostCounters implements content.Repository.
func (r *StatusRepository) RebuildLocalPostCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "local post counters", (*postgresgen.Queries).DeleteAllLocalPostCounters, (*postgresgen.Queries).RebuildLocalPostCounters)
}

// ListLocalPostCounters implements content.Repository.
func (r *StatusRepository) ListLocalPostCounters(ctx context.Context) ([]content.LocalPostCounter, error) {
	rows, err := r.db.reader().ListLocalPostCounters(ctx)
	if err != nil {
		return nil, wrap(err, "list local post counters")
	}
	out := make([]content.LocalPostCounter, len(rows))
	for i, row := range rows {
		out[i] = content.LocalPostCounter(row)
	}
	return out, nil
}

// LocalPostsTotal implements content.Repository.
func (r *StatusRepository) LocalPostsTotal(ctx context.Context) (int64, error) {
	n, err := r.db.reader().SumLocalPosts(ctx)
	return n, wrap(err, "sum local posts")
}

// AddLocalPosts implements content.InteractionRepository.
func (r *InteractionRepository) AddLocalPosts(ctx context.Context, tx uow.Tx, accountID string, delta int64) error {
	return r.db.addLocalPosts(ctx, tx, accountID, delta)
}

// ListLocalStatusBoosters implements content.InteractionRepository.
func (r *InteractionRepository) ListLocalStatusBoosters(ctx context.Context, tx uow.Tx, statusID string) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListLocalStatusBoosters(ctx, statusID)
	return ids, wrap(err, "list local boosters of "+statusID)
}
