// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/graph"
)

// This file holds the source listings that the content and graph modules replay to rebuild
// other modules' projections (content.ReplayEvents, graph.ReplayEvents).

// ListLiveBoosts implements content.InteractionRepository.
func (r *InteractionRepository) ListLiveBoosts(ctx context.Context, tx uow.Tx) ([]content.Boost, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListLiveBoosts(ctx)
	if err != nil {
		return nil, wrap(err, "list live boosts")
	}
	out := make([]content.Boost, len(rows))
	for i, row := range rows {
		out[i] = boostFrom(row)
	}
	return out, nil
}

// ListAllMutes implements graph.Repository.
func (r *GraphRepository) ListAllMutes(ctx context.Context, tx uow.Tx) ([]graph.Mute, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAllMutes(ctx)
	if err != nil {
		return nil, wrap(err, "list all mutes")
	}
	out := make([]graph.Mute, len(rows))
	for i, row := range rows {
		out[i] = muteFrom(row)
	}
	return out, nil
}
