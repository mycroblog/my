// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

var (
	// errEdgeExists reports that a concurrent transaction created the pair's edge first. It is
	// retryable: the next attempt sees that edge and treats the follow as already made.
	errEdgeExists = errors.New("graph: the pair already has a live follow")
	// errBlockEdgeExists is errEdgeExists for block_edges.
	errBlockEdgeExists = errors.New("graph: the pair already has a block in force")
	// errEdgeChanged reports that a concurrent transaction changed the edge that a command read
	// (accepted, rejected, or ended the follow). It is retryable: the next attempt reads the edge
	// again, so that only one outcome is recorded and the counters move once.
	errEdgeChanged = errors.New("graph: the follow changed concurrently")
)

// RegisterProjectors registers the graph module's synchronous projectors (data-model §9):
// follow_edges and follow_counters from follows and their events, and block_edges from blocks
// and their retractions. Each projector applies the same rule as RebuildProjections, so
// updating incrementally and rebuilding agree.
func RegisterProjectors(r *uow.Projectors, repo Repository) {
	r.Register(EventFollowed, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(Followed)
		if err := insertEdge(ctx, tx, repo, graph.ActiveEdge(ev.Follow, ev.Acceptance)); err != nil {
			return err
		}
		return countFollow(ctx, tx, repo, ev.Follow, 1)
	})
	r.Register(EventFollowRequested, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return insertEdge(ctx, tx, repo, graph.PendingEdge(e.(FollowRequested).Follow))
	})
	r.Register(EventFollowsCreated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(FollowsCreated)
		edges := make([]graph.Edge, 0, len(ev.Followed)+len(ev.Requested))
		deltas := map[string]CounterDelta{}
		for _, f := range ev.Followed {
			edges = append(edges, graph.ActiveEdge(f.Follow, f.Acceptance))
			d := deltas[f.Follow.FollowerAccountID]
			d.Following++
			deltas[f.Follow.FollowerAccountID] = d
			d = deltas[f.Follow.FolloweeAccountID]
			d.Followers++
			deltas[f.Follow.FolloweeAccountID] = d
		}
		for _, f := range ev.Requested {
			edges = append(edges, graph.PendingEdge(f.Follow))
		}
		inserted, err := repo.InsertEdges(ctx, tx, edges)
		if err != nil {
			return err
		}
		if inserted != len(edges) {
			return uow.MarkRetryable(errEdgeExists)
		}
		return repo.AddManyCounters(ctx, tx, deltas)
	})
	r.Register(EventFollowAccepted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(FollowAccepted)
		updated, err := repo.UpdateEdge(ctx, tx, graph.ActiveEdge(ev.Follow, ev.Acceptance), graph.StatePending)
		if err != nil {
			return err
		}
		if !updated {
			return uow.MarkRetryable(errEdgeChanged)
		}
		return countFollow(ctx, tx, repo, ev.Follow, 1)
	})
	r.Register(EventFollowRejected, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(FollowRejected)
		f := ev.Follow
		deleted, err := repo.DeleteEdge(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID, f.ID, graph.StatePending)
		if err != nil {
			return err
		}
		// A follow that the system rejects at once never had an edge.
		if !deleted && ev.Rejection.RejectedBy != graph.RejectedBySystem {
			return uow.MarkRetryable(errEdgeChanged)
		}
		return nil
	})
	r.Register(EventFollowTerminated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ev := e.(FollowTerminated)
		f := ev.Follow
		deleted, err := repo.DeleteEdge(ctx, tx, f.FollowerAccountID, f.FolloweeAccountID, f.ID, ev.PreviousState)
		if err != nil {
			return err
		}
		if !deleted {
			return uow.MarkRetryable(errEdgeChanged)
		}
		if ev.PreviousState != graph.StateActive {
			return nil
		}
		return countFollow(ctx, tx, repo, f, -1)
	})
	r.Register(EventFollowsTerminated, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		ts := e.(FollowsTerminated).Terminations
		edges := make([]graph.Edge, len(ts))
		deltas := map[string]CounterDelta{}
		for i, t := range ts {
			f := t.Follow
			edges[i] = graph.Edge{FollowerAccountID: f.FollowerAccountID, FolloweeAccountID: f.FolloweeAccountID, FollowID: f.ID, State: t.PreviousState}
			if t.PreviousState == graph.StateActive {
				d := deltas[f.FollowerAccountID]
				d.Following--
				deltas[f.FollowerAccountID] = d
				d = deltas[f.FolloweeAccountID]
				d.Followers--
				deltas[f.FolloweeAccountID] = d
			}
		}
		deleted, err := repo.DeleteEdgesOfFollows(ctx, tx, edges)
		if err != nil {
			return err
		}
		if deleted != len(edges) {
			return uow.MarkRetryable(errEdgeChanged)
		}
		return repo.AddManyCounters(ctx, tx, deltas)
	})
	r.Register(EventBlocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		inserted, err := repo.InsertBlockEdge(ctx, tx, graph.EdgeOf(e.(Blocked).Block))
		if err != nil {
			return err
		}
		if !inserted {
			return uow.MarkRetryable(errBlockEdgeExists)
		}
		return nil
	})
	r.Register(EventUnblocked, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		b := e.(Unblocked).Block
		return repo.DeleteBlockEdge(ctx, tx, b.BlockerAccountID, b.BlockedAccountID, b.ID)
	})
}

func insertEdge(ctx context.Context, tx uow.Tx, repo Repository, e graph.Edge) error {
	inserted, err := repo.InsertEdge(ctx, tx, e)
	if err != nil {
		return err
	}
	if !inserted {
		return uow.MarkRetryable(errEdgeExists)
	}
	return nil
}

// countFollow adds delta to the follower's following count and the followee's followers count.
func countFollow(ctx context.Context, tx uow.Tx, repo Repository, f graph.Follow, delta int64) error {
	if err := repo.AddCounters(ctx, tx, f.FollowerAccountID, 0, delta); err != nil {
		return err
	}
	return repo.AddCounters(ctx, tx, f.FolloweeAccountID, delta, 0)
}

// RebuildProjections empties follow_edges, follow_counters, and block_edges and derives them
// again from the follows, the blocks, and their events (data-model §9), in one transaction.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo Repository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error {
		if err := repo.DeleteAllEdges(ctx, tx); err != nil {
			return err
		}
		if err := repo.DeleteAllCounters(ctx, tx); err != nil {
			return err
		}
		histories, err := repo.ListHistories(ctx, tx)
		if err != nil {
			return err
		}
		var edges []graph.Edge
		for _, h := range histories {
			e, ok := h.Edge()
			if !ok {
				continue
			}
			inserted, err := repo.InsertEdge(ctx, tx, e)
			if err != nil {
				return err
			}
			if !inserted {
				return fmt.Errorf("graph: follows %s and another follow are both live", e.FollowID)
			}
			edges = append(edges, e)
		}
		counters := graph.CountEdges(edges)
		for _, id := range slices.Sorted(maps.Keys(counters)) {
			if err := repo.UpsertCounters(ctx, tx, counters[id]); err != nil {
				return err
			}
		}
		return rebuildBlockEdges(ctx, tx, repo)
	})
	if err != nil {
		return fmt.Errorf("rebuild follow_edges, follow_counters, and block_edges: %w", err)
	}
	return nil
}

func rebuildBlockEdges(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllBlockEdges(ctx, tx); err != nil {
		return err
	}
	histories, err := repo.ListBlockHistories(ctx, tx)
	if err != nil {
		return err
	}
	for _, h := range histories {
		e, ok := h.Edge()
		if !ok {
			continue
		}
		inserted, err := repo.InsertBlockEdge(ctx, tx, e)
		if err != nil {
			return err
		}
		if !inserted {
			return fmt.Errorf("graph: blocks %s and another block are both in force", e.BlockID)
		}
	}
	return nil
}
