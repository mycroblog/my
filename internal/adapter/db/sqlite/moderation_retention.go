// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

var _ appmoderation.RetentionRepository = (*ModerationRepository)(nil)

// InsertRetentionHold implements moderation.RetentionRepository.
func (r *ModerationRepository) InsertRetentionHold(ctx context.Context, tx uow.Tx, h moderation.RetentionHold) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertRetentionHold(ctx, sqlitegen.InsertRetentionHoldParams{
		ID: h.ID, AccountID: h.AccountID, Reason: h.Reason, CreatedByUserID: toNullString(h.CreatedByUserID),
		CreatedAt: toMillis(h.CreatedAt),
	}), "insert retention hold")
}

// InsertRetentionHoldRelease implements moderation.RetentionRepository.
func (r *ModerationRepository) InsertRetentionHoldRelease(ctx context.Context, tx uow.Tx, rel moderation.RetentionHoldRelease) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertRetentionHoldRelease(ctx, sqlitegen.InsertRetentionHoldReleaseParams{
		HoldID: rel.HoldID, ReleasedByUserID: toNullString(rel.ReleasedByUserID), ReleasedAt: toMillis(rel.ReleasedAt),
	})
	if err != nil {
		return wrap(err, "insert retention hold release")
	}
	if n == 0 {
		return fmt.Errorf("release of retention hold %s: %w", rel.HoldID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// RetentionHold implements moderation.RetentionRepository.
func (r *ModerationRepository) RetentionHold(ctx context.Context, tx uow.Tx, id string) (moderation.RetentionHoldRecord, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.RetentionHoldRecord{}, err
	}
	row, err := q.GetRetentionHold(ctx, id)
	if err != nil {
		return moderation.RetentionHoldRecord{}, notFound(err, appmoderation.ErrNotFound, "retention hold "+id)
	}
	return holdRecord(row), nil
}

// ListRetentionHolds implements moderation.RetentionRepository.
func (r *ModerationRepository) ListRetentionHolds(ctx context.Context, accountID string) ([]moderation.RetentionHoldRecord, error) {
	rows, err := r.db.reader().ListRetentionHolds(ctx, accountID)
	if err != nil {
		return nil, wrap(err, "list retention holds")
	}
	out := make([]moderation.RetentionHoldRecord, len(rows))
	for i, row := range rows {
		out[i] = holdRecord(sqlitegen.GetRetentionHoldRow(row))
	}
	return out, nil
}

func holdRecord(row sqlitegen.GetRetentionHoldRow) moderation.RetentionHoldRecord {
	rec := moderation.RetentionHoldRecord{Hold: moderation.RetentionHold{
		ID: row.ID, AccountID: row.AccountID, Reason: row.Reason, CreatedByUserID: row.CreatedByUserID.String,
		CreatedAt: fromMillis(row.CreatedAt),
	}}
	if row.ReleaseHoldID.Valid {
		rec.Release = &moderation.RetentionHoldRelease{
			HoldID: row.ReleaseHoldID.String, ReleasedByUserID: row.ReleasedByUserID.String, ReleasedAt: fromMillis(row.ReleasedAt.Int64),
		}
	}
	return rec
}

// HeldAccountIDs implements moderation.RetentionRepository.
func (r *ModerationRepository) HeldAccountIDs(ctx context.Context) ([]string, error) {
	ids, err := r.db.reader().ListHeldAccountIDs(ctx)
	return ids, wrap(err, "list held accounts")
}

// StatusRetained implements moderation.RetentionRepository.
func (r *ModerationRepository) StatusRetained(ctx context.Context, tx uow.Tx, statusID, authorAccountID string) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.StatusRetained(ctx, sqlitegen.StatusRetainedParams{StatusID: statusID, AuthorAccountID: authorAccountID})
	return n != 0, wrap(err, "tell whether a status is retained")
}

// idBelow is the UUIDv7 floor just past t: a record stamped no later than t has a smaller ID,
// which bounds the scan of the primary key.
func idBelow(t time.Time) string { return kernel.UUIDv7Floor(t.Add(time.Millisecond)) }

// ListActionsToPurge implements moderation.RetentionRepository.
func (r *ModerationRepository) ListActionsToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListModerationActionsToPurge(ctx, sqlitegen.ListModerationActionsToPurgeParams{
		IDBefore: idBelow(endedBefore), EndedBefore: toMillis(endedBefore), BatchLimit: int64(limit),
	})
	return ids, wrap(err, "list moderation actions to purge")
}

// PurgeActions implements moderation.RetentionRepository.
func (r *ModerationRepository) PurgeActions(ctx context.Context, tx uow.Tx, actionIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := jsonList(actionIDs)
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
			return wrap(err, "purge moderation actions")
		}
	}
	return nil
}

// ListReportsToPurge implements moderation.RetentionRepository.
func (r *ModerationRepository) ListReportsToPurge(ctx context.Context, tx uow.Tx, resolvedBefore time.Time, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListReportsToPurge(ctx, sqlitegen.ListReportsToPurgeParams{
		IDBefore: idBelow(resolvedBefore), ResolvedBefore: toMillis(resolvedBefore), BatchLimit: int64(limit),
	})
	return ids, wrap(err, "list reports to purge")
}

// PurgeReports implements moderation.RetentionRepository.
func (r *ModerationRepository) PurgeReports(ctx context.Context, tx uow.Tx, reportIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := jsonList(reportIDs)
	if err != nil {
		return err
	}
	steps := []func(context.Context, string) error{
		q.PurgeReportStatuses, q.PurgeReportRuleCitations, q.PurgeReportAssignments, q.PurgeReportResolutions,
		q.PurgeReportReopenings, q.PurgeReportNotes, q.PurgeReportVoiceRooms, q.PurgeReportStates, q.PurgeReports,
	}
	for _, step := range steps {
		if err := step(ctx, ids); err != nil {
			return wrap(err, "purge reports")
		}
	}
	return nil
}

// ReportedVoiceRooms implements moderation.RetentionRepository.
func (r *ModerationRepository) ReportedVoiceRooms(ctx context.Context, roomIDs []string) (map[string]bool, error) {
	ids, err := jsonList(roomIDs)
	if err != nil {
		return nil, err
	}
	rooms, err := r.db.reader().ListReportedVoiceRooms(ctx, ids)
	if err != nil {
		return nil, wrap(err, "list reported voice rooms")
	}
	out := make(map[string]bool, len(rooms))
	for _, id := range rooms {
		out[id] = true
	}
	return out, nil
}

// ListSignupBlocksToPurge implements moderation.RetentionRepository.
func (r *ModerationRepository) ListSignupBlocksToPurge(ctx context.Context, tx uow.Tx, endedBefore time.Time, limit int) ([]string, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids, err := q.ListSignupBlocksToPurge(ctx, sqlitegen.ListSignupBlocksToPurgeParams{
		IDBefore: idBelow(endedBefore), EndedBefore: toMillis(endedBefore), BatchLimit: int64(limit),
	})
	return ids, wrap(err, "list sign-up blocks to purge")
}

// PurgeSignupBlocks implements moderation.RetentionRepository.
func (r *ModerationRepository) PurgeSignupBlocks(ctx context.Context, tx uow.Tx, blockIDs []string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	ids, err := jsonList(blockIDs)
	if err != nil {
		return err
	}
	if err := q.PurgeSignupBlockLifts(ctx, ids); err != nil {
		return wrap(err, "purge sign-up blocks")
	}
	return wrap(q.PurgeSignupBlocks(ctx, ids), "purge sign-up blocks")
}

// DeleteAuditEntriesBefore implements moderation.RetentionRepository.
func (r *ModerationRepository) DeleteAuditEntriesBefore(ctx context.Context, tx uow.Tx, t time.Time, limit int) (int64, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteAuditLogEntriesBefore(ctx, sqlitegen.DeleteAuditLogEntriesBeforeParams{
		IDBefore: idBelow(t), OccurredBefore: toMillis(t), BatchLimit: int64(limit),
	})
	return n, wrap(err, "delete audit log entries")
}

// ReportHistory implements moderation.RetentionRepository.
func (r *ModerationRepository) ReportHistory(ctx context.Context, reportID string) (appmoderation.ReportHistory, error) {
	q := r.db.reader()
	assignments, err := q.ListReportAssignments(ctx, reportID)
	if err != nil {
		return appmoderation.ReportHistory{}, wrap(err, "list report assignments")
	}
	resolutions, err := q.ListReportResolutions(ctx, reportID)
	if err != nil {
		return appmoderation.ReportHistory{}, wrap(err, "list report resolutions")
	}
	reopenings, err := q.ListReportReopenings(ctx, reportID)
	if err != nil {
		return appmoderation.ReportHistory{}, wrap(err, "list report reopenings")
	}
	var h appmoderation.ReportHistory
	for _, a := range assignments {
		h.Assignments = append(h.Assignments, moderation.ReportAssignment{
			ID: a.ID, ReportID: a.ReportID, AssigneeUserID: a.AssigneeUserID.String, AssignedByUserID: a.AssignedByUserID.String,
			AssignedAt: fromMillis(a.AssignedAt),
		})
	}
	for _, res := range resolutions {
		h.Resolutions = append(h.Resolutions, moderation.ReportResolution{
			ID: res.ID, ReportID: res.ReportID, Outcome: moderation.ReportOutcome(res.Outcome), ResolvedByUserID: res.ResolvedByUserID.String,
			ResolvedAt: fromMillis(res.ResolvedAt),
		})
	}
	for _, ro := range reopenings {
		h.Reopenings = append(h.Reopenings, moderation.ReportReopening{
			ID: ro.ID, ReportID: ro.ReportID, ReopenedByUserID: ro.ReopenedByUserID.String, ReopenedAt: fromMillis(ro.ReopenedAt),
		})
	}
	return h, nil
}
