// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"encoding/json"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/notificationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/notification"
)

// UpsertExclusions implements notification.Repository.
func (r *Repository) UpsertExclusions(ctx context.Context, tx uow.Tx, xs []notification.Exclusion) error {
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
	return sqlite.Wrap(q.UpsertNotificationExclusions(ctx, string(list)), "upsert notification exclusions")
}

// DeleteExclusionsOf implements notification.Repository.
func (r *Repository) DeleteExclusionsOf(ctx context.Context, tx uow.Tx, viewerIDs, targetIDs []string, reason notification.ExclusionReason) error {
	if len(viewerIDs) == 0 {
		return nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	pairs, err := sqlite.JSONPairs(viewerIDs, targetIDs)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteNotificationExclusionsOf(ctx, notificationgen.DeleteNotificationExclusionsOfParams{Reason: string(reason), Pairs: pairs}), "delete notification exclusions")
}
