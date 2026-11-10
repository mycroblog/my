// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"slices"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ apptimeline.TagFollowRepository = (*Repository)(nil)

// InsertTagFollow implements timeline.TagFollowRepository.
func (r *Repository) InsertTagFollow(ctx context.Context, tx uow.Tx, f apptimeline.TagFollow) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertFeedTagFollow(ctx, timelinegen.InsertFeedTagFollowParams{
		AccountID: f.AccountID, TagID: f.TagID, FollowedAt: f.FollowedAt,
	})
	return n == 1, postgres.Wrap(err, "insert feed tag follow")
}

// DeleteTagFollow implements timeline.TagFollowRepository.
func (r *Repository) DeleteTagFollow(ctx context.Context, tx uow.Tx, accountID, tagID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteFeedTagFollow(ctx, timelinegen.DeleteFeedTagFollowParams{AccountID: accountID, TagID: tagID}), "delete feed tag follow")
}

// CountTagFollows implements timeline.TagFollowRepository.
func (r *Repository) CountTagFollows(ctx context.Context, tx uow.Tx, accountID string) (int, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.CountFeedTagFollows(ctx, accountID)
	return int(n), postgres.Wrap(err, "count feed tag follows")
}

// TagFollowsOf implements timeline.TagFollowRepository.
func (r *Repository) TagFollowsOf(ctx context.Context, accountID string) ([]apptimeline.TagFollow, error) {
	rows, err := r.db.reader().ListFeedTagFollows(ctx, accountID)
	return tagFollowsFrom(rows, err, "list feed tag follows")
}

// FollowsTag implements timeline.TagFollowRepository.
func (r *Repository) FollowsTag(ctx context.Context, accountID, tagID string) (bool, error) {
	ok, err := r.db.reader().FeedTagFollowExists(ctx, timelinegen.FeedTagFollowExistsParams{AccountID: accountID, TagID: tagID})
	return ok, postgres.Wrap(err, "look up feed tag follow")
}

// AllTagFollows implements timeline.TagFollowRepository.
func (r *Repository) AllTagFollows(ctx context.Context) ([]apptimeline.TagFollow, error) {
	rows, err := r.db.reader().ListAllFeedTagFollows(ctx)
	return tagFollowsFrom(rows, err, "list all feed tag follows")
}

func tagFollowsFrom(rows []timelinegen.FeedTagFollow, err error, what string) ([]apptimeline.TagFollow, error) {
	if err != nil {
		return nil, postgres.Wrap(err, what)
	}
	out := make([]apptimeline.TagFollow, len(rows))
	for i, row := range rows {
		out[i] = apptimeline.TagFollow{AccountID: row.AccountID, TagID: row.TagID, FollowedAt: row.FollowedAt}
	}
	return out, nil
}

// ListHomeTags implements timeline.TagFollowRepository.
func (r *Repository) ListHomeTags(ctx context.Context, hq apptimeline.HomeQuery) ([]apptimeline.Entry, error) {
	all, langs := languages(hq.Languages)
	limit, err := postgres.ToInt32(hq.Limit, "page limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListHomeTagEntries(ctx, timelinegen.ListHomeTagEntriesParams{
		Viewer: hq.ViewerAccountID, Before: postgres.ToNullString(hq.Before), WindowFloor: postgres.ToNullString(hq.Floor), Now: hq.Now,
		AllLanguages: all, Languages: langs, PageLimit: limit,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list home tag entries")
	}
	out := make([]apptimeline.Entry, len(rows))
	for i, row := range rows {
		e := entryFrom(timelinegen.FeedEntry{
			ID: row.ID, Kind: row.Kind, StatusID: row.StatusID, AuthorAccountID: row.AuthorAccountID, AuthorDomain: row.AuthorDomain,
			OriginalAuthorAccountID: row.OriginalAuthorAccountID, OriginalAuthorDomain: row.OriginalAuthorDomain,
			Visibility: row.Visibility, IsLocal: row.IsLocal, Language: row.Language, RevisionSeq: row.RevisionSeq, IsReply: row.IsReply,
		})
		e.FollowedTagIDs = slices.Clone(row.TagIds)
		slices.Sort(e.FollowedTagIDs)
		out[i] = e
	}
	return out, nil
}

// FeedTagFollowersAmong implements timeline.TagFollowRepository.
func (r *Repository) FeedTagFollowersAmong(ctx context.Context, entryID string, accountIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(accountIDs) == 0 {
		return out, nil
	}
	ids, err := r.db.reader().ListFeedTagFollowersAmong(ctx, timelinegen.ListFeedTagFollowersAmongParams{EntryID: entryID, AccountIds: accountIDs})
	if err != nil {
		return nil, postgres.Wrap(err, "list feed tag followers")
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
