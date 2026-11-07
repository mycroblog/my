// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// InsertReportReopening implements moderation.ReportNoteRepository.
func (r *Repository) InsertReportReopening(ctx context.Context, tx uow.Tx, ro moderation.ReportReopening) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertReportReopening(ctx, moderationgen.InsertReportReopeningParams{
		ID: ro.ID, ReportID: ro.ReportID, ReopenedByUserID: sqlite.ToNullString(ro.ReopenedByUserID), ReopenedAt: sqlite.ToMillis(ro.ReopenedAt),
	}), "insert report reopening")
}

// InsertReportNote implements moderation.ReportNoteRepository.
func (r *Repository) InsertReportNote(ctx context.Context, tx uow.Tx, n moderation.ReportNote) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertReportNote(ctx, moderationgen.InsertReportNoteParams{
		ID: n.ID, ReportID: n.ReportID, AuthorUserID: sqlite.ToNullString(n.AuthorUserID), Body: n.Body, WrittenAt: sqlite.ToMillis(n.WrittenAt),
	}), "insert report note")
}

// ListReportNotes implements moderation.ReportNoteRepository.
func (r *Repository) ListReportNotes(ctx context.Context, reportID string) ([]moderation.ReportNote, error) {
	rows, err := r.db.reader().ListReportNotes(ctx, reportID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list report notes")
	}
	out := make([]moderation.ReportNote, len(rows))
	for i, row := range rows {
		out[i] = moderation.ReportNote{
			ID: row.ID, ReportID: row.ReportID, AuthorUserID: row.AuthorUserID.String, Body: row.Body, WrittenAt: sqlite.FromMillis(row.WrittenAt),
		}
	}
	return out, nil
}

// CountOtherReportsOf implements moderation.ReportNoteRepository.
func (r *Repository) CountOtherReportsOf(ctx context.Context, targetAccountID, reportID string) (int64, error) {
	n, err := r.db.reader().CountOtherReportsOfTarget(ctx, moderationgen.CountOtherReportsOfTargetParams{
		TargetAccountID: targetAccountID, ReportID: reportID,
	})
	return n, sqlite.Wrap(err, "count reports")
}

// CountActionsOf implements moderation.ReportNoteRepository.
func (r *Repository) CountActionsOf(ctx context.Context, accountID string) (int64, error) {
	n, err := r.db.reader().CountModerationActionsOfTarget(ctx, accountID)
	return n, sqlite.Wrap(err, "count moderation actions")
}
