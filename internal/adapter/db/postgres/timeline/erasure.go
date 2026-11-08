// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

var _ apptimeline.ErasureRepository = (*Repository)(nil)

// EraseFeedsOf implements timeline.ErasureRepository.
func (r *Repository) EraseFeedsOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	lim := postgres.BatchLimit(limit)
	entries, err := q.ListFeedEntryIDsToErase(ctx, timelinegen.ListFeedEntryIDsToEraseParams{AccountID: accountID, Lim: lim})
	if err != nil {
		return 0, postgres.Wrap(err, "list feed entries to erase")
	}
	most := int64(len(entries))
	if len(entries) > 0 {
		ids := entries
		used, err := q.CountUsedFeedTagsOfEntries(ctx, ids)
		if err != nil {
			return 0, postgres.Wrap(err, "count the counted tags of feed entries")
		}
		if err := postgres.RunSteps(ctx, ids, "erase feed entries",
			q.EraseFeedDirectRecipientsOfEntries, q.EraseFeedTagEntriesOfEntries, q.EraseFeedEntries); err != nil {
			return 0, err
		}
		deltas := make(map[string]int64, len(used))
		for _, u := range used {
			deltas[u.TagID] = -u.N
		}
		if err := applyTagUsage(ctx, q, deltas); err != nil {
			return 0, err
		}
	}
	for _, step := range []func(context.Context, string, int32) (int64, error){
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseFeedDirectRecipientsOf(ctx, timelinegen.EraseFeedDirectRecipientsOfParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseFeedFollowEdgesAsFollower(ctx, timelinegen.EraseFeedFollowEdgesAsFollowerParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseFeedFollowEdgesAsFollowee(ctx, timelinegen.EraseFeedFollowEdgesAsFolloweeParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseFeedExclusionsAsViewer(ctx, timelinegen.EraseFeedExclusionsAsViewerParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseFeedExclusionsAsTarget(ctx, timelinegen.EraseFeedExclusionsAsTargetParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseFeedDomainMutesOf(ctx, timelinegen.EraseFeedDomainMutesOfParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseListMembersAsOwner(ctx, timelinegen.EraseListMembersAsOwnerParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseListMembersAsMember(ctx, timelinegen.EraseListMembersAsMemberParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseListNamesOf(ctx, timelinegen.EraseListNamesOfParams{AccountID: a, Lim: n})
		},
		func(ctx context.Context, a string, n int32) (int64, error) {
			return q.EraseListsOf(ctx, timelinegen.EraseListsOfParams{AccountID: a, Lim: n})
		},
	} {
		n, err := step(ctx, accountID, lim)
		if err != nil {
			return 0, postgres.Wrap(err, "erase feeds")
		}
		most = max(most, n)
	}
	return most, nil
}
