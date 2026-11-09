// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// LockPairs implements graph.Repository: the locks of LockPair, in the order of their keys.
func (r *Repository) LockPairs(ctx context.Context, tx uow.Tx, accountID string, others []string) error {
	if len(others) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	keys := make([]string, len(others))
	for i, other := range others {
		a, b := accountID, other
		if b < a {
			a, b = b, a
		}
		keys[i] = "graph:" + a + ":" + b
	}
	return postgres.Wrap(q.LockAccountPairs(ctx, keys), "lock account pairs")
}

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
	rows, err := q.ListFollowsByIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list follows by IDs")
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
	err = q.InsertFollowTerminations(ctx, graphgen.InsertFollowTerminationsParams{FollowIds: followIDs, Reason: string(reason), TerminatedAt: at})
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
	ids, states := make([]string, len(edges)), make([]string, len(edges))
	for i, e := range edges {
		ids[i], states[i] = e.FollowID, string(e.State)
	}
	n, err := q.DeleteFollowEdgesOfFollows(ctx, graphgen.DeleteFollowEdgesOfFollowsParams{FollowIds: ids, States: states})
	return int(n), postgres.Wrap(err, "delete follow edges of follows")
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
	var p graphgen.AddToManyFollowCountersParams
	for id, d := range deltas {
		p.AccountIds = append(p.AccountIds, id)
		p.FollowersDeltas = append(p.FollowersDeltas, d.Followers)
		p.FollowingDeltas = append(p.FollowingDeltas, d.Following)
	}
	had, err := q.AddToManyFollowCounters(ctx, p)
	if err != nil {
		return postgres.Wrap(err, "add to many follow counters")
	}
	for id, d := range deltas {
		if slices.Contains(had, id) {
			continue
		}
		if err := q.AddFollowCounters(ctx, graphgen.AddFollowCountersParams{AccountID: id, FollowersCount: d.Followers, FollowingCount: d.Following}); err != nil {
			return postgres.Wrap(err, "add follow counters")
		}
	}
	return postgres.Wrap(q.DeleteZeroFollowCountersOf(ctx, p.AccountIds), "delete zero follow counters")
}
