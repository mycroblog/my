// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/contentgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// The projections of many deleted statuses at once (appcontent.StatusesDeleted), each write as
// the write of one status does for every status it names.

// ReplyLinksOf implements content.Repository.
func (r *StatusRepository) ReplyLinksOf(ctx context.Context, tx uow.Tx, statusIDs []string) (map[string]content.ReplyLink, error) {
	out := make(map[string]content.ReplyLink, len(statusIDs))
	if len(statusIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListStatusReplyLinksOf(ctx, statusIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list reply links")
	}
	for _, row := range rows {
		out[row.StatusID] = content.ReplyLink(row)
	}
	return out, nil
}

// AddManyReplies implements content.Repository.
func (r *StatusRepository) AddManyReplies(ctx context.Context, tx uow.Tx, deltas map[string]int64) error {
	if len(deltas) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var p contentgen.AddManyStatusRepliesParams
	for _, id := range slices.Sorted(maps.Keys(deltas)) {
		p.StatusIds, p.Deltas = append(p.StatusIds, id), append(p.Deltas, deltas[id])
	}
	return postgres.Wrap(q.AddManyStatusReplies(ctx, p), "add replies")
}

// DeleteStatusProjectionsOf implements content.Repository.
func (r *StatusRepository) DeleteStatusProjectionsOf(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	if len(statusIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	for _, step := range []struct {
		what string
		fn   func(context.Context, []string) error
	}{
		{"delete status heads", q.DeleteStatusHeadsOf},
		{"delete status mentions", q.DeleteStatusMentionsOf},
		{"delete status counters", q.DeleteStatusCountersOf},
		{"delete reply links", q.DeleteStatusReplyLinksOf},
	} {
		if err := step.fn(ctx, statusIDs); err != nil {
			return postgres.Wrap(err, step.what)
		}
	}
	return nil
}

// RemovePosts implements content.Repository.
func (r *StatusRepository) RemovePosts(ctx context.Context, tx uow.Tx, accountID string, n int64) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	latest, err := q.LatestLivePublishedAt(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return postgres.Wrap(q.DeleteAccountPostCounter(ctx, accountID), "delete account post counter")
	}
	if err != nil {
		return postgres.Wrap(err, "latest status of "+accountID)
	}
	if err := q.RemoveAccountPosts(ctx, contentgen.RemoveAccountPostsParams{Removed: n, LastStatusAt: latest, AccountID: accountID}); err != nil {
		return postgres.Wrap(err, "remove account posts")
	}
	return postgres.Wrap(q.DeleteEmptyAccountPostCounter(ctx, accountID), "delete empty account post counter")
}

// DeleteStatusLinkCardsOf implements content.Repository.
func (r *StatusRepository) DeleteStatusLinkCardsOf(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	if len(statusIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteManyStatusLinkCards(ctx, statusIDs), "delete status link cards")
}

// SubtractLocalPostsOf implements content.Repository.
func (r *StatusRepository) SubtractLocalPostsOf(ctx context.Context, tx uow.Tx, removed map[string]int64) error {
	return r.db.subtractLocalPostsOf(ctx, tx, removed)
}

// SubtractLocalPostsOf implements content.InteractionRepository.
func (r *InteractionRepository) SubtractLocalPostsOf(ctx context.Context, tx uow.Tx, removed map[string]int64) error {
	return r.db.subtractLocalPostsOf(ctx, tx, removed)
}

// subtractLocalPostsOf is addLocalPosts of a negative delta of each account of removed.
func (d *moduleDB) subtractLocalPostsOf(ctx context.Context, tx uow.Tx, removed map[string]int64) error {
	if len(removed) == 0 {
		return nil
	}
	q, err := d.queries(tx)
	if err != nil {
		return err
	}
	var ids []string
	var ns []int64
	for _, id := range slices.Sorted(maps.Keys(removed)) {
		ids, ns = append(ids, id), append(ns, removed[id])
	}
	if err := q.DeleteManySpentLocalPostCounters(ctx, contentgen.DeleteManySpentLocalPostCountersParams{AccountIds: ids, Removed: ns}); err != nil {
		return postgres.Wrap(err, "delete spent local post counters")
	}
	return postgres.Wrap(q.SubtractManyLocalPosts(ctx, contentgen.SubtractManyLocalPostsParams{AccountIds: ids, Removed: ns}), "subtract local posts")
}

// DeleteStatusReactionCountsOf implements content.InteractionRepository.
func (r *InteractionRepository) DeleteStatusReactionCountsOf(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	if len(statusIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteManyStatusReactionCounts(ctx, statusIDs), "delete status reaction counts")
}

// ListLocalStatusBoostersOf implements content.InteractionRepository.
func (r *InteractionRepository) ListLocalStatusBoostersOf(ctx context.Context, tx uow.Tx, statusIDs []string) ([]string, error) {
	if len(statusIDs) == 0 {
		return nil, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListLocalStatusBoostersOf(ctx, statusIDs)
	return ids, postgres.Wrap(err, "list local boosters")
}

// DeleteStatusBoostEdgesOf implements content.InteractionRepository.
func (r *InteractionRepository) DeleteStatusBoostEdgesOf(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	if len(statusIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteManyStatusBoostEdges(ctx, statusIDs), "delete status boost edges")
}
