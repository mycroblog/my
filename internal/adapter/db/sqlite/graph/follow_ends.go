// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// LockPairs implements graph.Repository: SQLite's write transactions already serialize.
func (r *Repository) LockPairs(context.Context, uow.Tx, string, []string) error { return nil }

// FollowsByIDs implements graph.Repository.
func (r *Repository) FollowsByIDs(ctx context.Context, tx uow.Tx, ids []string) (map[string]graph.Follow, error) {
	out := make(map[string]graph.Follow, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	list, err := sqlite.JSONList(ids)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowsByIDs(ctx, list)
	if err != nil {
		return nil, sqlite.Wrap(err, "list follows by IDs")
	}
	for _, row := range rows {
		out[row.ID] = followFrom(row)
	}
	return out, nil
}

// InsertTerminations implements graph.Repository.
func (r *Repository) InsertTerminations(ctx context.Context, tx uow.Tx, followIDs []string, reason graph.TerminationReason, at time.Time) error {
	if len(followIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	list, err := sqlite.JSONList(followIDs)
	if err != nil {
		return err
	}
	err = q.InsertFollowTerminations(ctx, graphgen.InsertFollowTerminationsParams{FollowIds: list, Reason: string(reason), TerminatedAt: sqlite.ToMillis(at)})
	return alreadyRecorded(err, "follow_terminations", followIDs[0])
}

// DeleteEdgesOfFollows implements graph.Repository.
func (r *Repository) DeleteEdgesOfFollows(ctx context.Context, tx uow.Tx, edges []graph.Edge) (int, error) {
	if len(edges) == 0 {
		return 0, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	type row struct {
		FollowID string `json:"follow_id"`
		State    string `json:"state"`
	}
	rows := make([]row, len(edges))
	for i, e := range edges {
		rows[i] = row{FollowID: e.FollowID, State: string(e.State)}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteFollowEdgesOfFollows(ctx, string(list))
	return int(n), sqlite.Wrap(err, "delete follow edges of follows")
}

// AddManyCounters implements graph.Repository.
func (r *Repository) AddManyCounters(ctx context.Context, tx uow.Tx, deltas map[string]appgraph.CounterDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	type row struct {
		AccountID string `json:"account_id"`
		Followers int64  `json:"followers"`
		Following int64  `json:"following"`
	}
	rows := make([]row, 0, len(deltas))
	ids := make([]string, 0, len(deltas))
	for id, d := range deltas {
		rows = append(rows, row{AccountID: id, Followers: d.Followers, Following: d.Following})
		ids = append(ids, id)
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	had, err := q.AddToManyFollowCounters(ctx, string(list))
	if err != nil {
		return sqlite.Wrap(err, "add to many follow counters")
	}
	for _, rw := range rows {
		if slices.Contains(had, rw.AccountID) {
			continue
		}
		if err := q.AddFollowCounters(ctx, graphgen.AddFollowCountersParams{AccountID: rw.AccountID, FollowersCount: rw.Followers, FollowingCount: rw.Following}); err != nil {
			return sqlite.Wrap(err, "add follow counters")
		}
	}
	idList, err := sqlite.JSONList(ids)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteZeroFollowCountersOf(ctx, idList), "delete zero follow counters")
}
