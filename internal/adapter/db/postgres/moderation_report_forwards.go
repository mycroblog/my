// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertReportForward implements moderation.ReportForwardRepository.
func (r *ModerationRepository) InsertReportForward(ctx context.Context, tx uow.Tx, f moderation.ReportForward) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReportForward(ctx, postgresgen.InsertReportForwardParams{
		ID: f.ID, ReportID: f.ReportID, ForwardedByUserID: toNullString(f.ForwardedByUserID), ForwardedAt: f.ForwardedAt,
	})
	return n > 0, wrap(err, "insert report forward")
}

// reportForward reads the forward of a report, nil for none.
func (r *ModerationRepository) reportForward(ctx context.Context, reportID string) (*moderation.ReportForward, error) {
	rows, err := r.db.reader().GetReportForward(ctx, reportID)
	if err != nil || len(rows) == 0 {
		return nil, wrap(err, "read report forward")
	}
	row := rows[0]
	return &moderation.ReportForward{
		ID: row.ID, ReportID: row.ReportID, ForwardedByUserID: fromNullString(row.ForwardedByUserID), ForwardedAt: utc(row.ForwardedAt),
	}, nil
}
