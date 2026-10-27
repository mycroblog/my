// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertReportReopening implements moderation.ReportNoteRepository.
func (r *ModerationRepository) InsertReportReopening(ctx context.Context, tx uow.Tx, ro moderation.ReportReopening) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertReportReopening(ctx, postgresgen.InsertReportReopeningParams{
		ID: ro.ID, ReportID: ro.ReportID, ReopenedByUserID: toNullString(ro.ReopenedByUserID), ReopenedAt: ro.ReopenedAt,
	}), "insert report reopening")
}

// InsertReportNote implements moderation.ReportNoteRepository.
func (r *ModerationRepository) InsertReportNote(ctx context.Context, tx uow.Tx, n moderation.ReportNote) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertReportNote(ctx, postgresgen.InsertReportNoteParams{
		ID: n.ID, ReportID: n.ReportID, AuthorUserID: toNullString(n.AuthorUserID), Body: n.Body, WrittenAt: n.WrittenAt,
	}), "insert report note")
}

// ListReportNotes implements moderation.ReportNoteRepository.
func (r *ModerationRepository) ListReportNotes(ctx context.Context, reportID string) ([]moderation.ReportNote, error) {
	rows, err := r.db.reader().ListReportNotes(ctx, reportID)
	if err != nil {
		return nil, wrap(err, "list report notes")
	}
	out := make([]moderation.ReportNote, len(rows))
	for i, row := range rows {
		out[i] = moderation.ReportNote{
			ID: row.ID, ReportID: row.ReportID, AuthorUserID: fromNullString(row.AuthorUserID), Body: row.Body, WrittenAt: utc(row.WrittenAt),
		}
	}
	return out, nil
}

// CountOtherReportsOf implements moderation.ReportNoteRepository.
func (r *ModerationRepository) CountOtherReportsOf(ctx context.Context, targetAccountID, reportID string) (int64, error) {
	n, err := r.db.reader().CountOtherReportsOfTarget(ctx, postgresgen.CountOtherReportsOfTargetParams{
		TargetAccountID: targetAccountID, ReportID: reportID,
	})
	return n, wrap(err, "count reports")
}

// CountActionsOf implements moderation.ReportNoteRepository.
func (r *ModerationRepository) CountActionsOf(ctx context.Context, accountID string) (int64, error) {
	n, err := r.db.reader().CountModerationActionsOfTarget(ctx, accountID)
	return n, wrap(err, "count moderation actions")
}
