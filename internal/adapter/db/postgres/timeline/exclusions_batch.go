// SPDX-License-Identifier: AGPL-3.0-only

package timeline

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
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
	var p timelinegen.UpsertFeedExclusionsParams
	for _, x := range xs {
		exp := ""
		if x.ExpiresAt != nil {
			exp = x.ExpiresAt.UTC().Format(time.RFC3339Nano)
		}
		p.ViewerAccountIds, p.TargetAccountIds = append(p.ViewerAccountIds, x.ViewerAccountID), append(p.TargetAccountIds, x.TargetAccountID)
		p.Reasons, p.ExpiresAts = append(p.Reasons, string(x.Reason)), append(p.ExpiresAts, exp)
	}
	return postgres.Wrap(q.UpsertFeedExclusions(ctx, p), "upsert feed exclusions")
}
