// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The reads behind the moderator notifications and the counts of what awaits the moderators
// (FR-712).

// HasOtherOpenReportOf implements moderation.Repository.
func (r *Repository) HasOtherOpenReportOf(ctx context.Context, tx uow.Tx, targetAccountID, reportID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	ok, err := q.HasOtherOpenReportOf(ctx, moderationgen.HasOtherOpenReportOfParams{TargetAccountID: targetAccountID, ReportID: reportID})
	return ok, sqlite.Wrap(err, "find other open reports")
}

// CountReports implements moderation.Repository.
func (r *Repository) CountReports(ctx context.Context, state moderation.ReportState) (int64, error) {
	n, err := r.db.reader().CountReportsInState(ctx, string(state))
	return n, sqlite.Wrap(err, "count reports")
}

// OldestReportFiledAt implements moderation.Repository.
func (r *Repository) OldestReportFiledAt(ctx context.Context, state moderation.ReportState) (time.Time, error) {
	t, err := r.db.reader().OldestReportFiledAt(ctx, string(state))
	if err != nil {
		return time.Time{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "the oldest report")
	}
	return sqlite.FromMillis(t), nil
}
