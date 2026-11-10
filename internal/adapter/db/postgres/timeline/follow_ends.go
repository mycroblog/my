// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// UpsertFollowEdges implements timeline.Repository.
func (r *Repository) UpsertFollowEdges(ctx context.Context, tx uow.Tx, es []apptimeline.FollowEdge) error {
	if len(es) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	var p timelinegen.UpsertFeedFollowEdgesParams
	for _, e := range es {
		p.FollowerIds, p.FolloweeIds, p.ShowBoosts = append(p.FollowerIds, e.FollowerAccountID), append(p.FolloweeIds, e.FolloweeAccountID), append(p.ShowBoosts, e.ShowBoosts)
	}
	return postgres.Wrap(q.UpsertFeedFollowEdges(ctx, p), "upsert feed follow edges")
}

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
