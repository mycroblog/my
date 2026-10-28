// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/kernel"
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
		return wrap(q.AddLocalPosts(ctx, sqlitegen.AddLocalPostsParams{AccountID: accountID, Added: delta}), "add local posts")
	case delta < 0:
		removed := -delta
		if err := q.DeleteSpentLocalPostCounter(ctx, sqlitegen.DeleteSpentLocalPostCounterParams{AccountID: accountID, Removed: removed}); err != nil {
			return wrap(err, "delete spent local post counter")
		}
		return wrap(q.SubtractLocalPosts(ctx, sqlitegen.SubtractLocalPostsParams{Removed: removed, AccountID: accountID}), "subtract local posts")
	}
	return nil
}

// AddLocalPosts implements content.Repository.
func (r *StatusRepository) AddLocalPosts(ctx context.Context, tx uow.Tx, accountID string, delta int64) error {
	return r.db.addLocalPosts(ctx, tx, accountID, delta)
}

// RebuildLocalPostCounters implements content.Repository.
func (r *StatusRepository) RebuildLocalPostCounters(ctx context.Context, tx uow.Tx) error {
	return r.rebuild(ctx, tx, "local post counters", (*sqlitegen.Queries).DeleteAllLocalPostCounters, (*sqlitegen.Queries).RebuildLocalPostCounters)
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

// DailyLocalPublicPosts implements content.Repository.
func (r *StatusRepository) DailyLocalPublicPosts(ctx context.Context, to time.Time, days int) ([]int, error) {
	out := make([]int, days)
	from := to.Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := r.db.reader().CountLocalPublicPostsByDay(ctx, sqlitegen.CountLocalPublicPostsByDayParams{FromMs: toMillis(from), FromID: kernel.UUIDv7Floor(from), ToMs: toMillis(to)})
	if err != nil {
		return nil, wrap(err, "count local public posts by day")
	}
	for _, row := range rows {
		if row.Day >= 0 && row.Day < int64(days) {
			out[row.Day] = int(row.Posts)
		}
	}
	return out, nil
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

// ListRecentLocalPosters implements content.Repository.
func (r *StatusRepository) ListRecentLocalPosters(ctx context.Context, since time.Time, limit int) ([]string, error) {
	rows, err := r.db.reader().ListRecentLocalPosters(ctx, sqlitegen.ListRecentLocalPostersParams{
		FromID: kernel.UUIDv7Floor(since), SinceMs: toMillis(since), Lim: int64(limit),
	})
	if err != nil {
		return nil, wrap(err, "list recent local posters")
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.AccountID
	}
	return out, nil
}
