// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/graph"
)

// ListAllMutes implements graph.Repository.
func (r *Repository) ListAllMutes(ctx context.Context, tx uow.Tx) ([]graph.Mute, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAllMutes(ctx)
	if err != nil {
		return nil, postgres.Wrap(err, "list all mutes")
	}
	out := make([]graph.Mute, len(rows))
	for i, row := range rows {
		out[i] = muteFrom(row)
	}
	return out, nil
}
