// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	apptimeline "github.com/mycroblog/my/internal/app/timeline"
	"github.com/mycroblog/my/internal/app/uow"
)

// UpsertExclusions implements timeline.Repository.
func (r *Repository) UpsertExclusions(ctx context.Context, tx uow.Tx, xs []apptimeline.Exclusion) error {
	if len(xs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	type row struct {
		Viewer    string `json:"viewer"`
		Target    string `json:"target"`
		Reason    string `json:"reason"`
		ExpiresAt *int64 `json:"expires_at"`
	}
	rows := make([]row, len(xs))
	for i, x := range xs {
		rows[i] = row{Viewer: x.ViewerAccountID, Target: x.TargetAccountID, Reason: string(x.Reason)}
		if x.ExpiresAt != nil {
			v := sqlite.ToMillis(*x.ExpiresAt)
			rows[i].ExpiresAt = &v
		}
	}
	list, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertFeedExclusions(ctx, string(list)), "upsert feed exclusions")
}
