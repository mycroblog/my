// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/notificationgen"
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
	var p notificationgen.UpsertNotificationExclusionsParams
	for _, x := range xs {
		exp := ""
		if x.ExpiresAt != nil {
			exp = x.ExpiresAt.UTC().Format(time.RFC3339Nano)
		}
		p.ViewerAccountIds, p.TargetAccountIds = append(p.ViewerAccountIds, x.ViewerAccountID), append(p.TargetAccountIds, x.TargetAccountID)
		p.Reasons, p.ExpiresAts = append(p.Reasons, string(x.Reason)), append(p.ExpiresAts, exp)
	}
	return postgres.Wrap(q.UpsertNotificationExclusions(ctx, p), "upsert notification exclusions")
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
	return postgres.Wrap(q.DeleteNotificationExclusionsOf(ctx, notificationgen.DeleteNotificationExclusionsOfParams{
		Reason: string(reason), ViewerAccountIds: viewerIDs, TargetAccountIds: targetIDs,
	}), "delete notification exclusions")
}
