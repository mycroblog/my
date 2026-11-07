// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

var _ appgraph.ErasureRepository = (*Repository)(nil)

// ListEdgesToErase implements graph.ErasureRepository.
func (r *Repository) ListEdgesToErase(ctx context.Context, tx uow.Tx, accountID string, limit int) ([]graph.Edge, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListFollowEdgesToErase(ctx, graphgen.ListFollowEdgesToEraseParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "list follow edges to erase")
	}
	return edgesFrom(rows), nil
}

// EraseFollowsOf implements graph.ErasureRepository.
func (r *Repository) EraseFollowsOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	list, err := q.ListFollowIDsToErase(ctx, graphgen.ListFollowIDsToEraseParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil || len(list) == 0 {
		return 0, sqlite.Wrap(err, "list follows to erase")
	}
	ids, err := sqlite.JSONList(list)
	if err != nil {
		return 0, err
	}
	err = sqlite.RunSteps(ctx, ids, "erase follows",
		q.EraseFollowAcceptances, q.EraseFollowRejections, q.EraseFollowTerminations, q.EraseFollowResends,
		q.EraseFollowPreferences, q.EraseFollowEdgesOfFollows, q.EraseFollows)
	return int64(len(list)), err
}

// EraseBlocksOf implements graph.ErasureRepository.
func (r *Repository) EraseBlocksOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	list, err := q.ListBlockIDsToErase(ctx, graphgen.ListBlockIDsToEraseParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil || len(list) == 0 {
		return 0, sqlite.Wrap(err, "list blocks to erase")
	}
	ids, err := sqlite.JSONList(list)
	if err != nil {
		return 0, err
	}
	err = sqlite.RunSteps(ctx, ids, "erase blocks", q.EraseBlockRetractions, q.EraseBlockEdgesOfBlocks, q.EraseBlocks)
	return int64(len(list)), err
}

// EraseMutesOf implements graph.ErasureRepository.
func (r *Repository) EraseMutesOf(ctx context.Context, tx uow.Tx, accountID string, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	by, err := q.EraseMutesBy(ctx, graphgen.EraseMutesByParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil {
		return 0, sqlite.Wrap(err, "erase mutes")
	}
	of, err := q.EraseMutesOf(ctx, graphgen.EraseMutesOfParams{AccountID: accountID, Lim: int64(limit)})
	if err != nil {
		return 0, sqlite.Wrap(err, "erase mutes")
	}
	domains, err := q.EraseDomainMutesBy(ctx, graphgen.EraseDomainMutesByParams{AccountID: accountID, Lim: int64(limit)})
	return max(by, of, domains), sqlite.Wrap(err, "erase domain mutes")
}

// EraseCountersOf implements graph.ErasureRepository.
func (r *Repository) EraseCountersOf(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.EraseFollowCounters(ctx, accountID), "erase follow counters")
}
