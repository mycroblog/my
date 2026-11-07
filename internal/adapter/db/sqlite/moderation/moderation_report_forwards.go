// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertReportForward implements moderation.ReportForwardRepository.
func (r *Repository) InsertReportForward(ctx context.Context, tx uow.Tx, f moderation.ReportForward) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReportForward(ctx, moderationgen.InsertReportForwardParams{
		ID: f.ID, ReportID: f.ReportID, Comment: f.Comment, ForwardedByUserID: sqlite.ToNullString(f.ForwardedByUserID), ForwardedAt: sqlite.ToMillis(f.ForwardedAt),
	})
	return n > 0, sqlite.Wrap(err, "insert report forward")
}

// reportForward reads the forward of a report, nil for none.
func (r *Repository) reportForward(ctx context.Context, reportID string) (*moderation.ReportForward, error) {
	rows, err := r.db.reader().GetReportForward(ctx, reportID)
	if err != nil || len(rows) == 0 {
		return nil, sqlite.Wrap(err, "read report forward")
	}
	row := rows[0]
	return &moderation.ReportForward{
		ID: row.ID, ReportID: row.ReportID, Comment: row.Comment, ForwardedByUserID: row.ForwardedByUserID.String, ForwardedAt: sqlite.FromMillis(row.ForwardedAt),
	}, nil
}
