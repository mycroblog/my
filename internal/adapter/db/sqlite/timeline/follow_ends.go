// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
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
	pairs, err := sqlite.JSONPairs(followerIDs, followeeIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteFeedFollowEdges(ctx, pairs), "delete feed follow edges")
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
	pairs, err := sqlite.JSONPairs(followerIDs, followeeIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteListMembersOfFollows(ctx, pairs), "delete list members of follows")
}
