// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/contentgen"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ appcontent.ErasureRepository = (*StatusRepository)(nil)

// ListAuthorStatusesToErase implements content.ErasureRepository.
func (r *StatusRepository) ListAuthorStatusesToErase(ctx context.Context, tx uow.Tx, authorAccountID, after string, limit int) ([]appcontent.ErasableStatus, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAuthorStatusesToErase(ctx, contentgen.ListAuthorStatusesToEraseParams{
		AuthorAccountID: authorAccountID, After: after, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list statuses to erase")
	}
	out := make([]appcontent.ErasableStatus, len(rows))
	for i, row := range rows {
		out[i] = appcontent.ErasableStatus{
			Status: statusFrom(contentgen.Status{
				ID: row.ID, Uri: row.Uri, Url: row.Url, AuthorAccountID: row.AuthorAccountID, IsLocal: row.IsLocal,
				Visibility: row.Visibility, InReplyToUri: row.InReplyToUri,
				PublishedAt: row.PublishedAt,
			}),
			Deleted: row.Deleted != 0,
		}
	}
	return out, nil
}

// EraseStatuses implements content.ErasureRepository.
func (r *StatusRepository) EraseStatuses(ctx context.Context, tx uow.Tx, statusIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(statusIDs)
	if err != nil {
		return err
	}
	if err := sqlite.RunSteps(ctx, ids, "erase statuses",
		q.EraseLocalStatusesOfStatuses, q.EraseCountersOfStatuses, q.EraseLinkCardsOfStatuses, q.EraseBoostEdgesOfStatuses,
		q.EraseBoostRetractionsOfStatuses, q.EraseBoostsOfStatuses, q.EraseFavouritesOfStatuses,
		q.EraseReactionsOfStatuses, q.EraseReactionCountsOfStatuses, q.EraseBookmarksOfStatuses, q.ErasePinsOfStatuses); err != nil {
		return err
	}
	return r.PurgeStatuses(ctx, tx, statusIDs)
}

// EraseInteractionsBy implements content.ErasureRepository.
func (r *StatusRepository) EraseInteractionsBy(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	lim := int64(limit)
	var most int64
	keep := func(n int64, err error) error {
		most = max(most, n)
		return err
	}
	steps := []func() error{
		func() error {
			return q.UncountFavouritesBy(ctx, contentgen.UncountFavouritesByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return keep(q.EraseFavouritesBy(ctx, contentgen.EraseFavouritesByParams{AccountID: accountID, Lim: lim}))
		},
		func() error {
			return q.DeleteSingleReactionCountsBy(ctx, contentgen.DeleteSingleReactionCountsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return q.UncountReactionsBy(ctx, contentgen.UncountReactionsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return keep(q.EraseReactionsBy(ctx, contentgen.EraseReactionsByParams{AccountID: accountID, Lim: lim}))
		},
		func() error {
			return keep(q.EraseBookmarksBy(ctx, contentgen.EraseBookmarksByParams{AccountID: accountID, Lim: lim}))
		},
		func() error {
			return keep(q.ErasePollVotesBy(ctx, contentgen.ErasePollVotesByParams{AccountID: accountID, Lim: lim}))
		},
		func() error { return keep(q.ErasePinsBy(ctx, accountID)) },
		func() error {
			ids, err := q.ListRemoteMediaToErase(ctx, contentgen.ListRemoteMediaToEraseParams{AccountID: accountID, Lim: lim})
			if err != nil {
				return err
			}
			for _, id := range ids {
				if _, err := q.DeleteUnreferencedRemoteMedia(ctx, id); err != nil {
					return err
				}
				if err := q.DeleteRemoteMediaFile(ctx, id); err != nil {
					return err
				}
			}
			most = max(most, int64(len(ids)))
			return nil
		},
		// The drafts (ADR-0156) go whole, with their schedules, before the media eraser counts
		// their uploads as attached.
		func() error { return appcontent.EraseDrafts(ctx, tx, r, accountID) },
		// The automatic deletion policy (FR-118) is a setting of the account; its notices of the
		// server maximum (ADR-0151 §5) go with it.
		func() error { return q.EraseAutoDeleteKeptVisibilitiesBy(ctx, accountID) },
		func() error { return q.EraseAutoDeletePolicyBy(ctx, accountID) },
		func() error { return q.EraseAutoDeleteLimitBy(ctx, accountID) },
		func() error { return q.EraseAutoDeleteLimitNoticesBy(ctx, accountID) },
		func() error {
			return q.UncountBoostsBy(ctx, contentgen.UncountBoostsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			// The boosts in force count as the account's local posts too (local_post_counters).
			n, err := q.EraseBoostEdgesBy(ctx, contentgen.EraseBoostEdgesByParams{AccountID: accountID, Lim: lim})
			if err := keep(n, err); err != nil {
				return err
			}
			return r.db.addLocalPosts(ctx, tx, accountID, -n)
		},
		func() error {
			return q.EraseBoostRetractionsBy(ctx, contentgen.EraseBoostRetractionsByParams{AccountID: accountID, Lim: lim})
		},
		func() error {
			return keep(q.EraseBoostsBy(ctx, contentgen.EraseBoostsByParams{AccountID: accountID, Lim: lim}))
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return 0, sqlite.Wrap(err, "erase interactions")
		}
	}
	return most, nil
}
