// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertReportReopening implements moderation.ReportNoteRepository.
func (r *Repository) InsertReportReopening(ctx context.Context, tx uow.Tx, ro moderation.ReportReopening) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertReportReopening(ctx, moderationgen.InsertReportReopeningParams{
		ID: ro.ID, ReportID: ro.ReportID, ReopenedByUserID: postgres.ToNullString(ro.ReopenedByUserID), ReopenedAt: ro.ReopenedAt,
	}), "insert report reopening")
}

// InsertReportNote implements moderation.ReportNoteRepository.
func (r *Repository) InsertReportNote(ctx context.Context, tx uow.Tx, n moderation.ReportNote) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertReportNote(ctx, moderationgen.InsertReportNoteParams{
		ID: n.ID, ReportID: n.ReportID, AuthorUserID: postgres.ToNullString(n.AuthorUserID), Body: n.Body, WrittenAt: n.WrittenAt,
	}), "insert report note")
}

// ListReportNotes implements moderation.ReportNoteRepository.
func (r *Repository) ListReportNotes(ctx context.Context, reportID string) ([]moderation.ReportNote, error) {
	rows, err := r.db.reader().ListReportNotes(ctx, reportID)
	if err != nil {
		return nil, postgres.Wrap(err, "list report notes")
	}
	out := make([]moderation.ReportNote, len(rows))
	for i, row := range rows {
		out[i] = moderation.ReportNote{
			ID: row.ID, ReportID: row.ReportID, AuthorUserID: postgres.FromNullString(row.AuthorUserID), Body: row.Body, WrittenAt: postgres.UTC(row.WrittenAt),
		}
	}
	return out, nil
}

// CountOtherReportsOf implements moderation.ReportNoteRepository.
func (r *Repository) CountOtherReportsOf(ctx context.Context, targetAccountID, reportID string) (int64, error) {
	n, err := r.db.reader().CountOtherReportsOfTarget(ctx, moderationgen.CountOtherReportsOfTargetParams{
		TargetAccountID: targetAccountID, ReportID: reportID,
	})
	return n, postgres.Wrap(err, "count reports")
}

// CountActionsOf implements moderation.ReportNoteRepository.
func (r *Repository) CountActionsOf(ctx context.Context, accountID string) (int64, error) {
	n, err := r.db.reader().CountModerationActionsOfTarget(ctx, accountID)
	return n, postgres.Wrap(err, "count moderation actions")
}
