// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertReportReopening implements moderation.ReportNoteRepository.
func (r *ModerationRepository) InsertReportReopening(ctx context.Context, tx uow.Tx, ro moderation.ReportReopening) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertReportReopening(ctx, sqlitegen.InsertReportReopeningParams{
		ID: ro.ID, ReportID: ro.ReportID, ReopenedByUserID: toNullString(ro.ReopenedByUserID), ReopenedAt: toMillis(ro.ReopenedAt),
	}), "insert report reopening")
}

// InsertReportNote implements moderation.ReportNoteRepository.
func (r *ModerationRepository) InsertReportNote(ctx context.Context, tx uow.Tx, n moderation.ReportNote) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertReportNote(ctx, sqlitegen.InsertReportNoteParams{
		ID: n.ID, ReportID: n.ReportID, AuthorUserID: toNullString(n.AuthorUserID), Body: n.Body, WrittenAt: toMillis(n.WrittenAt),
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
			ID: row.ID, ReportID: row.ReportID, AuthorUserID: row.AuthorUserID.String, Body: row.Body, WrittenAt: fromMillis(row.WrittenAt),
		}
	}
	return out, nil
}

// CountOtherReportsOf implements moderation.ReportNoteRepository.
func (r *ModerationRepository) CountOtherReportsOf(ctx context.Context, targetAccountID, reportID string) (int64, error) {
	n, err := r.db.reader().CountOtherReportsOfTarget(ctx, sqlitegen.CountOtherReportsOfTargetParams{
		TargetAccountID: targetAccountID, ReportID: reportID,
	})
	return n, wrap(err, "count reports")
}

// CountActionsOf implements moderation.ReportNoteRepository.
func (r *ModerationRepository) CountActionsOf(ctx context.Context, accountID string) (int64, error) {
	n, err := r.db.reader().CountModerationActionsOfTarget(ctx, accountID)
	return n, wrap(err, "count moderation actions")
}
