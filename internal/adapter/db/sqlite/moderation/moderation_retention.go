// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.RetentionRepository = (*Repository)(nil)

// InsertRetentionHold implements moderation.RetentionRepository.
func (r *Repository) InsertRetentionHold(ctx context.Context, tx uow.Tx, h moderation.RetentionHold) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertRetentionHold(ctx, moderationgen.InsertRetentionHoldParams{
		ID: h.ID, AccountID: h.AccountID, Reason: h.Reason, CreatedByUserID: sqlite.ToNullString(h.CreatedByUserID),
		CreatedAt: sqlite.ToMillis(h.CreatedAt),
	}), "insert retention hold")
}

// InsertRetentionHoldRelease implements moderation.RetentionRepository.
func (r *Repository) InsertRetentionHoldRelease(ctx context.Context, tx uow.Tx, rel moderation.RetentionHoldRelease) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertRetentionHoldRelease(ctx, moderationgen.InsertRetentionHoldReleaseParams{
		HoldID: rel.HoldID, ReleasedByUserID: sqlite.ToNullString(rel.ReleasedByUserID), ReleasedAt: sqlite.ToMillis(rel.ReleasedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert retention hold release")
	}
	if n == 0 {
		return fmt.Errorf("release of retention hold %s: %w", rel.HoldID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// RetentionHold implements moderation.RetentionRepository.
func (r *Repository) RetentionHold(ctx context.Context, tx uow.Tx, id string) (moderation.RetentionHoldRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.RetentionHoldRecord{}, err
	}
	row, err := q.GetRetentionHold(ctx, id)
	if err != nil {
		return moderation.RetentionHoldRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "retention hold "+id)
	}
	return holdRecord(row), nil
}

// ListRetentionHolds implements moderation.RetentionRepository.
func (r *Repository) ListRetentionHolds(ctx context.Context, accountID string) ([]moderation.RetentionHoldRecord, error) {
	rows, err := r.db.reader().ListRetentionHolds(ctx, accountID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list retention holds")
	}
	out := make([]moderation.RetentionHoldRecord, len(rows))
	for i, row := range rows {
		out[i] = holdRecord(moderationgen.GetRetentionHoldRow(row))
	}
	return out, nil
}

func holdRecord(row moderationgen.GetRetentionHoldRow) moderation.RetentionHoldRecord {
	rec := moderation.RetentionHoldRecord{Hold: moderation.RetentionHold{
		ID: row.ID, AccountID: row.AccountID, Reason: row.Reason, CreatedByUserID: row.CreatedByUserID.String,
		CreatedAt: sqlite.FromMillis(row.CreatedAt),
	}}
	if row.ReleaseHoldID.Valid {
		rec.Release = &moderation.RetentionHoldRelease{
			HoldID: row.ReleaseHoldID.String, ReleasedByUserID: row.ReleasedByUserID.String, ReleasedAt: sqlite.FromMillis(row.ReleasedAt.Int64),
		}
	}
	return rec
}

// HeldAccountIDs implements moderation.RetentionRepository.
func (r *Repository) HeldAccountIDs(ctx context.Context) ([]string, error) {
	ids, err := r.db.reader().ListHeldAccountIDs(ctx)
	return ids, sqlite.Wrap(err, "list held accounts")
}

// StatusRetained implements moderation.RetentionRepository.
func (r *Repository) StatusRetained(ctx context.Context, tx uow.Tx, statusID, authorAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.StatusRetained(ctx, moderationgen.StatusRetainedParams{StatusID: statusID, AuthorAccountID: authorAccountID})
	return n != 0, sqlite.Wrap(err, "tell whether a status is retained")
}

// idBelow is the UUIDv7 floor just past t: a record stamped no later than t has a smaller ID,
// which bounds the scan of the primary key.
func idBelow(t time.Time) string { return kernel.UUIDv7Floor(t.Add(time.Millisecond)) }

// ListActionsToPurge implements moderation.RetentionRepository.
func (r *Repository) ListActionsToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListModerationActionsToPurge(ctx, moderationgen.ListModerationActionsToPurgeParams{
		IDBefore: idBelow(endedBefore), EndedBefore: sqlite.ToMillis(endedBefore), BatchLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list moderation actions to purge")
}

// PurgeActions implements moderation.RetentionRepository.
func (r *Repository) PurgeActions(ctx context.Context, tx uow.Tx, actionIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(actionIDs)
	if err != nil {
		return err
	}
	// Children before their parents: the appeals' rows, then the action's.
	steps := []func(context.Context, string) error{
		q.PurgeModerationAppealStates, q.PurgeModerationAppealDecisions, q.PurgeModerationAppeals,
		q.PurgeModerationActionStatuses, q.PurgeModerationActionLifts, q.PurgeModerationActionRestrictions,
		q.PurgeModerationActions,
	}
	for _, step := range steps {
		if err := step(ctx, ids); err != nil {
			return sqlite.Wrap(err, "purge moderation actions")
		}
	}
	return nil
}

// ListReportsToPurge implements moderation.RetentionRepository.
func (r *Repository) ListReportsToPurge(ctx context.Context, tx uow.Tx, resolvedBefore time.Time, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListReportsToPurge(ctx, moderationgen.ListReportsToPurgeParams{
		IDBefore: idBelow(resolvedBefore), ResolvedBefore: sqlite.ToMillis(resolvedBefore), BatchLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list reports to purge")
}

// PurgeReports implements moderation.RetentionRepository.
func (r *Repository) PurgeReports(ctx context.Context, tx uow.Tx, reportIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(reportIDs)
	if err != nil {
		return err
	}
	steps := []func(context.Context, string) error{
		q.PurgeReportStatuses, q.PurgeReportRuleCitations, q.PurgeReportAssignments, q.PurgeReportResolutions,
		q.PurgeReportReopenings, q.PurgeReportNotes, q.PurgeReportVoiceRooms, q.PurgeReportHookOrigins, q.PurgeReportForwards, q.PurgeReportStates, q.PurgeReports,
	}
	for _, step := range steps {
		if err := step(ctx, ids); err != nil {
			return sqlite.Wrap(err, "purge reports")
		}
	}
	return nil
}

// ReportedVoiceRooms implements moderation.RetentionRepository.
func (r *Repository) ReportedVoiceRooms(ctx context.Context, roomIDs []string) (map[string]bool, error) {
	ids, err := sqlite.JSONList(roomIDs)
	if err != nil {
		return nil, err
	}
	rooms, err := r.db.reader().ListReportedVoiceRooms(ctx, ids)
	if err != nil {
		return nil, sqlite.Wrap(err, "list reported voice rooms")
	}
	out := make(map[string]bool, len(rooms))
	for _, id := range rooms {
		out[id] = true
	}
	return out, nil
}

// ListSignupBlocksToPurge implements moderation.RetentionRepository.
func (r *Repository) ListSignupBlocksToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListSignupBlocksToPurge(ctx, moderationgen.ListSignupBlocksToPurgeParams{
		IDBefore: idBelow(endedBefore), EndedBefore: sqlite.ToMillis(endedBefore), BatchLimit: int64(limit),
	})
	return ids, sqlite.Wrap(err, "list sign-up blocks to purge")
}

// PurgeSignupBlocks implements moderation.RetentionRepository.
func (r *Repository) PurgeSignupBlocks(ctx context.Context, tx uow.Tx, blockIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(blockIDs)
	if err != nil {
		return err
	}
	if err := q.PurgeSignupBlockLifts(ctx, ids); err != nil {
		return sqlite.Wrap(err, "purge sign-up blocks")
	}
	return sqlite.Wrap(q.PurgeSignupBlocks(ctx, ids), "purge sign-up blocks")
}

// DeleteAuditEntriesBefore implements moderation.RetentionRepository.
func (r *Repository) DeleteAuditEntriesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteAuditLogEntriesBefore(ctx, moderationgen.DeleteAuditLogEntriesBeforeParams{
		IDBefore: idBelow(t), OccurredBefore: sqlite.ToMillis(t), BatchLimit: int64(limit),
	})
	return n, sqlite.Wrap(err, "delete audit log entries")
}

// ReportHistory implements moderation.RetentionRepository.
func (r *Repository) ReportHistory(ctx context.Context, reportID string) (appmoderation.ReportHistory, error) {
	q := r.db.reader()
	assignments, err := q.ListReportAssignments(ctx, reportID)
	if err != nil {
		return appmoderation.ReportHistory{}, sqlite.Wrap(err, "list report assignments")
	}
	resolutions, err := q.ListReportResolutions(ctx, reportID)
	if err != nil {
		return appmoderation.ReportHistory{}, sqlite.Wrap(err, "list report resolutions")
	}
	reopenings, err := q.ListReportReopenings(ctx, reportID)
	if err != nil {
		return appmoderation.ReportHistory{}, sqlite.Wrap(err, "list report reopenings")
	}
	var h appmoderation.ReportHistory
	for _, a := range assignments {
		h.Assignments = append(h.Assignments, moderation.ReportAssignment{
			ID: a.ID, ReportID: a.ReportID, AssigneeUserID: a.AssigneeUserID.String, AssignedByUserID: a.AssignedByUserID.String,
			AssignedAt: sqlite.FromMillis(a.AssignedAt),
		})
	}
	for _, res := range resolutions {
		h.Resolutions = append(h.Resolutions, moderation.ReportResolution{
			ID: res.ID, ReportID: res.ReportID, Outcome: moderation.ReportOutcome(res.Outcome), ResolvedByUserID: res.ResolvedByUserID.String,
			ResolvedAt: sqlite.FromMillis(res.ResolvedAt),
		})
	}
	for _, ro := range reopenings {
		h.Reopenings = append(h.Reopenings, moderation.ReportReopening{
			ID: ro.ID, ReportID: ro.ReportID, ReopenedByUserID: ro.ReopenedByUserID.String, ReopenedAt: sqlite.FromMillis(ro.ReopenedAt),
		})
	}
	return h, nil
}

// ListRetentionHoldsToPurge implements moderation.RetentionRepository.
func (r *Repository) ListRetentionHoldsToPurge(ctx context.Context, tx uow.Tx, releasedBefore time.Time, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListRetentionHoldsToPurge(ctx, moderationgen.ListRetentionHoldsToPurgeParams{ReleasedBefore: sqlite.ToMillis(releasedBefore), BatchLimit: int64(limit)})
	return ids, sqlite.Wrap(err, "list retention holds to purge")
}

// PurgeRetentionHolds implements moderation.RetentionRepository.
func (r *Repository) PurgeRetentionHolds(ctx context.Context, tx uow.Tx, holdIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := sqlite.JSONList(holdIDs)
	if err != nil {
		return err
	}
	if err := q.PurgeRetentionHoldReleases(ctx, ids); err != nil {
		return sqlite.Wrap(err, "purge retention hold releases")
	}
	return sqlite.Wrap(q.PurgeRetentionHolds(ctx, ids), "purge retention holds")
}

// EraseSeverancesOf implements moderation.Repository.
func (r *Repository) EraseSeverancesOf(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.EraseRelationshipSeverances(ctx, accountID), "erase relationship severances")
}
