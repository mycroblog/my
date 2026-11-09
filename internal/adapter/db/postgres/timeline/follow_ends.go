// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	"github.com/mycroblog/my/internal/app/uow"
)

// DeleteFollowEdges implements timeline.Repository.
func (r *Repository) DeleteFollowEdges(ctx context.Context, tx uow.Tx, followerIDs, followeeIDs []string) error {
	if len(followerIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteFeedFollowEdges(ctx, timelinegen.DeleteFeedFollowEdgesParams{FollowerIds: followerIDs, FolloweeIds: followeeIDs}), "delete feed follow edges")
}

// DeleteListMembersOfFollows implements timeline.Repository.
func (r *Repository) DeleteListMembersOfFollows(ctx context.Context, tx uow.Tx, followerIDs, followeeIDs []string) error {
	if len(followerIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteListMembersOfFollows(ctx, timelinegen.DeleteListMembersOfFollowsParams{FollowerIds: followerIDs, FolloweeIds: followeeIDs}), "delete list members of follows")
}
