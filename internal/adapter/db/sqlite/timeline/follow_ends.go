// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
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
	type row struct {
		FollowerAccountID string `json:"follower_account_id"`
		FolloweeAccountID string `json:"followee_account_id"`
		ShowBoosts        bool   `json:"show_boosts"`
	}
	rows := make([]row, len(es))
	for i, e := range es {
		rows[i] = row{FollowerAccountID: e.FollowerAccountID, FolloweeAccountID: e.FolloweeAccountID, ShowBoosts: e.ShowBoosts}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertFeedFollowEdges(ctx, string(list)), "upsert feed follow edges")
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
