// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
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
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListStatusReplyLinksOf(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list reply links")
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
	list, err := idDeltas(deltas)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.AddManyStatusReplies(ctx, list), "add replies")
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
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	for _, step := range []struct {
		what string
		fn   func(context.Context, string) error
	}{
		{"delete status heads", q.DeleteStatusHeadsOf},
		{"delete status mentions", q.DeleteStatusMentionsOf},
		{"delete status counters", q.DeleteStatusCountersOf},
		{"delete reply links", q.DeleteStatusReplyLinksOf},
	} {
		if err := step.fn(ctx, list); err != nil {
			return sqlite.Wrap(err, step.what)
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
	if errors.Is(err, sql.ErrNoRows) {
		return sqlite.Wrap(q.DeleteAccountPostCounter(ctx, accountID), "delete account post counter")
	}
	if err != nil {
		return sqlite.Wrap(err, "latest status of "+accountID)
	}
	if err := q.RemoveAccountPosts(ctx, contentgen.RemoveAccountPostsParams{Removed: n, LastStatusAt: latest, AccountID: accountID}); err != nil {
		return sqlite.Wrap(err, "remove account posts")
	}
	return sqlite.Wrap(q.DeleteEmptyAccountPostCounter(ctx, accountID), "delete empty account post counter")
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
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteManyStatusLinkCards(ctx, list), "delete status link cards")
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
	list, err := idDeltas(removed)
	if err != nil {
		return err
	}
	if err := q.DeleteManySpentLocalPostCounters(ctx, list); err != nil {
		return sqlite.Wrap(err, "delete spent local post counters")
	}
	return sqlite.Wrap(q.SubtractManyLocalPosts(ctx, list), "subtract local posts")
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
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteManyStatusReactionCounts(ctx, list), "delete status reaction counts")
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
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListLocalStatusBoostersOf(ctx, list)
	return ids, sqlite.Wrap(err, "list local boosters")
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
	list, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteManyStatusBoostEdges(ctx, list), "delete status boost edges")
}

// idDeltas encodes deltas as the JSON array of {"id": ..., "n": ...} that the queries of many
// counters read, in ID order.
func idDeltas(deltas map[string]int64) (string, error) {
	type row struct {
		ID string `json:"id"`
		N  int64  `json:"n"`
	}
	rows := make([]row, 0, len(deltas))
	for _, id := range slices.Sorted(maps.Keys(deltas)) {
		rows = append(rows, row{ID: id, N: deltas[id]})
	}
	b, err := json.Marshal(rows)
	return string(b), err
}
