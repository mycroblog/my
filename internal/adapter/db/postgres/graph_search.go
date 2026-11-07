// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/graph"
)

// RelationsAmong implements graph.Repository: one query per relation over ids.
func (r *GraphRepository) RelationsAmong(ctx context.Context, accountID string, ids []string, now time.Time) (appgraph.RelationsAmong, error) {
	var out appgraph.RelationsAmong
	if len(ids) == 0 {
		return out, nil
	}
	q := r.db.reader()
	var err error
	if out.Blocked, err = q.ListBlockedAmong(ctx, graphgen.ListBlockedAmongParams{Viewer: accountID, Ids: ids}); err != nil {
		return out, wrap(err, "list blocked accounts among candidates")
	}
	if out.BlockedBy, err = q.ListBlockersAmong(ctx, graphgen.ListBlockersAmongParams{Viewer: accountID, Ids: ids}); err != nil {
		return out, wrap(err, "list blocking accounts among candidates")
	}
	if out.Muted, err = q.ListMutedAmong(ctx, graphgen.ListMutedAmongParams{Viewer: accountID, Ids: ids, Now: now}); err != nil {
		return out, wrap(err, "list muted accounts among candidates")
	}
	if out.Following, err = q.ListFollowedAmong(ctx, graphgen.ListFollowedAmongParams{Viewer: accountID, Ids: ids}); err != nil {
		return out, wrap(err, "list followed accounts among candidates")
	}
	rows, err := q.ListFollowCountersAmong(ctx, ids)
	if err != nil {
		return out, wrap(err, "list follow counters among candidates")
	}
	out.Counters = make([]graph.Counters, len(rows))
	for i, row := range rows {
		out.Counters[i] = graph.Counters(row)
	}
	return out, nil
}
