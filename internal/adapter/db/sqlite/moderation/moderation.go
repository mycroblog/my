// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// Repository implements moderation.Repository on SQLite.
type Repository struct {
	db *moduleDB
}

var _ appmoderation.Repository = (*Repository)(nil)

// NewRepository returns the moderation module's repository on d.
func NewRepository(d *sqlite.DB) *Repository {
	return &Repository{db: &moduleDB{d}}
}

// InsertAction implements moderation.Repository.
func (r *Repository) InsertAction(ctx context.Context, tx uow.Tx, a moderation.Action) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertModerationAction(ctx, moderationgen.InsertModerationActionParams{
		ID: a.ID, TargetAccountID: a.TargetAccountID, Kind: string(a.Kind), ReasonCategory: string(a.ReasonCategory),
		PublicReason: sqlite.ToNullString(a.PublicReason), PrivateNote: sqlite.ToNullString(a.PrivateNote), ReportID: sqlite.ToNullString(a.ReportID),
		Issuer: string(a.Issuer), IssuedByUserID: sqlite.ToNullString(a.IssuedByUserID), NotifyTarget: sqlite.BoolToInt(a.NotifyTarget),
		ExpiresAt: sqlite.ToNullMillis(a.ExpiresAt), IssuedAt: sqlite.ToMillis(a.IssuedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert moderation action")
	}
	for _, id := range a.StatusIDs {
		if err := q.InsertModerationActionStatus(ctx, moderationgen.InsertModerationActionStatusParams{ActionID: a.ID, StatusID: id}); err != nil {
			return sqlite.Wrap(err, "insert moderation action status")
		}
	}
	return nil
}

// actionStatusIDs returns the statuses of an action on statuses, nil for none.
func actionStatusIDs(ctx context.Context, q *moderationgen.Queries, actionID string) ([]string, error) {
	ids, err := q.ListModerationActionStatusIDs(ctx, actionID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list moderation action statuses")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

// Action implements moderation.Repository.
func (r *Repository) Action(ctx context.Context, tx uow.Tx, id string) (moderation.Action, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.Action{}, err
	}
	row, err := q.GetModerationAction(ctx, id)
	if err != nil {
		return moderation.Action{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "moderation action "+id)
	}
	statusIDs, err := actionStatusIDs(ctx, q, id)
	if err != nil {
		return moderation.Action{}, err
	}
	return moderation.Action{
		ID: row.ID, TargetAccountID: row.TargetAccountID, Kind: moderation.ActionKind(row.Kind),
		ReasonCategory: moderation.Category(row.ReasonCategory), PublicReason: row.PublicReason.String,
		PrivateNote: row.PrivateNote.String, ReportID: row.ReportID.String, Issuer: moderation.Issuer(row.Issuer),
		IssuedByUserID: row.IssuedByUserID.String, NotifyTarget: row.NotifyTarget != 0,
		ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt), IssuedAt: sqlite.FromMillis(row.IssuedAt), StatusIDs: statusIDs,
	}, nil
}

// InsertActionLift implements moderation.Repository.
func (r *Repository) InsertActionLift(ctx context.Context, tx uow.Tx, l moderation.Lift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertModerationActionLift(ctx, moderationgen.InsertModerationActionLiftParams{
		ActionID: l.ActionID, LiftedByUserID: sqlite.ToNullString(l.LiftedByUserID), Reason: sqlite.ToNullString(l.Reason),
		LiftedAt: sqlite.ToMillis(l.LiftedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert moderation action lift")
	}
	if n == 0 {
		return fmt.Errorf("lift of moderation action %s: %w", l.ActionID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// InsertDomainAction implements moderation.Repository.
func (r *Repository) InsertDomainAction(ctx context.Context, tx uow.Tx, a moderation.DomainAction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertDomainModerationAction(ctx, moderationgen.InsertDomainModerationActionParams{
		ID: a.ID, Domain: a.Domain, Kind: string(a.Kind), PublicComment: sqlite.ToNullString(a.PublicComment),
		PrivateNote: sqlite.ToNullString(a.PrivateNote), Obfuscate: sqlite.BoolToInt(a.Obfuscate),
		IssuedByUserID: sqlite.ToNullString(a.IssuedByUserID), ExpiresAt: sqlite.ToNullMillis(a.ExpiresAt), IssuedAt: sqlite.ToMillis(a.IssuedAt),
	}), "insert domain moderation action")
}

// DomainAction implements moderation.Repository.
func (r *Repository) DomainAction(ctx context.Context, tx uow.Tx, id string) (moderation.DomainAction, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.DomainAction{}, err
	}
	row, err := q.GetDomainModerationAction(ctx, id)
	if err != nil {
		return moderation.DomainAction{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "domain moderation action "+id)
	}
	return moderation.DomainAction{
		ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: row.PublicComment.String,
		PrivateNote: row.PrivateNote.String, Obfuscate: row.Obfuscate != 0, IssuedByUserID: row.IssuedByUserID.String,
		ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt), IssuedAt: sqlite.FromMillis(row.IssuedAt),
	}, nil
}

// InsertDomainActionLift implements moderation.Repository.
func (r *Repository) InsertDomainActionLift(ctx context.Context, tx uow.Tx, l moderation.DomainLift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertDomainModerationActionLift(ctx, moderationgen.InsertDomainModerationActionLiftParams{
		ActionID: l.ActionID, LiftedByUserID: sqlite.ToNullString(l.LiftedByUserID), LiftedAt: sqlite.ToMillis(l.LiftedAt),
	})
	if err != nil {
		return sqlite.Wrap(err, "insert domain moderation action lift")
	}
	if n == 0 {
		return fmt.Errorf("lift of domain moderation action %s: %w", l.ActionID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// InsertReport implements moderation.Repository.
func (r *Repository) InsertReport(ctx context.Context, tx uow.Tx, rep moderation.Report) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	// The origin goes first: its unique decision is what keeps a job that runs again from filing a
	// second report, and the report has no other key for it.
	if o := rep.HookOrigin; o != nil {
		n, err := q.InsertReportHookOrigin(ctx, moderationgen.InsertReportHookOriginParams{
			ReportID: rep.ID, HookDecisionID: o.DecisionID, Hook: o.Hook, ReasonCode: o.ReasonCode,
		})
		if err != nil || n == 0 {
			return false, sqlite.Wrap(err, "insert report hook origin")
		}
	}
	n, err := q.InsertReport(ctx, moderationgen.InsertReportParams{
		ID: rep.ID, Uri: sqlite.ToNullString(rep.URI), ReporterAccountID: rep.ReporterAccountID, TargetAccountID: rep.TargetAccountID,
		Category: string(rep.Category), Comment: rep.Comment, Forwarded: sqlite.BoolToInt(rep.Forwarded), FiledAt: sqlite.ToMillis(rep.FiledAt),
	})
	if err != nil || n == 0 {
		return false, sqlite.Wrap(err, "insert report")
	}
	for _, id := range rep.StatusIDs {
		if err := q.InsertReportStatus(ctx, moderationgen.InsertReportStatusParams{ReportID: rep.ID, StatusID: id}); err != nil {
			return false, sqlite.Wrap(err, "insert report status")
		}
	}
	for _, id := range rep.RuleIDs {
		if err := q.InsertReportRuleCitation(ctx, moderationgen.InsertReportRuleCitationParams{ReportID: rep.ID, RuleID: id}); err != nil {
			return false, sqlite.Wrap(err, "insert report rule citation")
		}
	}
	if v := rep.VoiceRoom; v != nil {
		err := q.InsertReportVoiceRoom(ctx, moderationgen.InsertReportVoiceRoomParams{ReportID: rep.ID, RoomID: v.RoomID, RevisionID: v.RevisionID})
		if err != nil {
			return false, sqlite.Wrap(err, "insert report voice room")
		}
	}
	return true, nil
}

func restrictionRowsFrom(target string, restriction string, source string, until sql.NullInt64) moderation.RestrictionRow {
	return moderation.RestrictionRow{
		Target: target, Restriction: moderation.Restriction(restriction), SourceActionID: source, EffectiveUntil: sqlite.FromNullMillis(until),
	}
}

// InsertAccountRestriction implements moderation.Repository.
func (r *Repository) InsertAccountRestriction(ctx context.Context, tx uow.Tx, row moderation.RestrictionRow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertAccountRestriction(ctx, moderationgen.InsertAccountRestrictionParams{
		AccountID: row.Target, Restriction: string(row.Restriction), SourceActionID: row.SourceActionID,
		EffectiveUntil: sqlite.ToNullMillis(row.EffectiveUntil),
	}), "insert account restriction")
}

// DeleteAccountRestrictionsOf implements moderation.Repository.
func (r *Repository) DeleteAccountRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteAccountRestrictionsOfAction(ctx, actionID), "delete account restrictions")
}

// LockAccountRestrictions implements moderation.Repository. SQLite runs one write transaction at
// a time, so there is nothing to lock.
func (r *Repository) LockAccountRestrictions(context.Context, uow.Tx, string) error {
	return nil
}

// LockDomainRestrictions implements moderation.Repository. SQLite runs one write transaction at
// a time, so there is nothing to lock.
func (r *Repository) LockDomainRestrictions(context.Context, uow.Tx, string) error {
	return nil
}

// AccountRestrictionRows implements moderation.Repository.
func (r *Repository) AccountRestrictionRows(ctx context.Context, tx uow.Tx, accountID string) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAccountRestrictionRows(ctx, accountID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// DeleteExpiredAccountRestrictions implements moderation.Repository.
func (r *Repository) DeleteExpiredAccountRestrictions(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteExpiredAccountRestrictions(ctx, moderationgen.DeleteExpiredAccountRestrictionsParams{Now: sqlite.ToMillis(now), BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "delete expired account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// DeleteExpiredDomainRestrictions implements moderation.Repository.
func (r *Repository) DeleteExpiredDomainRestrictions(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteExpiredDomainRestrictions(ctx, moderationgen.DeleteExpiredDomainRestrictionsParams{Now: sqlite.ToMillis(now), BatchLimit: int64(limit)})
	if err != nil {
		return nil, sqlite.Wrap(err, "delete expired domain restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.Domain, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// InsertDomainRestriction implements moderation.Repository.
func (r *Repository) InsertDomainRestriction(ctx context.Context, tx uow.Tx, row moderation.RestrictionRow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertDomainRestriction(ctx, moderationgen.InsertDomainRestrictionParams{
		Domain: row.Target, Restriction: string(row.Restriction), SourceActionID: row.SourceActionID,
		EffectiveUntil: sqlite.ToNullMillis(row.EffectiveUntil),
	}), "insert domain restriction")
}

// DeleteDomainRestrictionsOf implements moderation.Repository.
func (r *Repository) DeleteDomainRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.DeleteDomainRestrictionsOfAction(ctx, actionID), "delete domain restrictions")
}

// DomainRestrictionRows implements moderation.Repository.
func (r *Repository) DomainRestrictionRows(ctx context.Context, tx uow.Tx, domain string) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDomainRestrictionRows(ctx, domain)
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.Domain, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// UpsertReportStatus implements moderation.Repository.
func (r *Repository) UpsertReportStatus(ctx context.Context, tx uow.Tx, s moderation.ReportStatus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.UpsertReportState(ctx, moderationgen.UpsertReportStateParams{
		ReportID: s.ReportID, State: string(s.State), Outcome: sqlite.ToNullString(string(s.Outcome)),
		AssigneeUserID: sqlite.ToNullString(s.AssigneeUserID), Since: sqlite.ToMillis(s.Since),
	}), "upsert report state")
}

func restrictionsFrom(rows []string) []moderation.Restriction {
	out := make([]moderation.Restriction, len(rows))
	for i, r := range rows {
		out[i] = moderation.Restriction(r)
	}
	return out
}

// AccountRestrictionsInEffect implements moderation.Repository.
func (r *Repository) AccountRestrictionsInEffect(ctx context.Context, accountID string, now time.Time) ([]moderation.Restriction, error) {
	rows, err := r.db.reader().ListAccountRestrictionsInEffect(ctx, moderationgen.ListAccountRestrictionsInEffectParams{
		AccountID: accountID, Now: sqlite.ToMillis(now),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list account restrictions in effect")
	}
	return restrictionsFrom(rows), nil
}

// DomainRestrictionsInEffect implements moderation.Repository.
func (r *Repository) DomainRestrictionsInEffect(ctx context.Context, domains []string, now time.Time) ([]moderation.Restriction, error) {
	list, err := sqlite.JSONList(domains)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainRestrictionsInEffect(ctx, moderationgen.ListDomainRestrictionsInEffectParams{
		Domains: list, Now: sqlite.ToMillis(now),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain restrictions in effect")
	}
	return restrictionsFrom(rows), nil
}

func (r *Repository) reportFrom(ctx context.Context, row moderationgen.Report) (moderation.Report, error) {
	ids, err := r.db.reader().ListReportStatusIDs(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, sqlite.Wrap(err, "list report statuses")
	}
	rules, err := r.db.reader().ListReportRuleIDs(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, sqlite.Wrap(err, "list report rule citations")
	}
	rooms, err := r.db.reader().GetReportVoiceRoom(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, sqlite.Wrap(err, "read report voice room")
	}
	var room *moderation.ReportVoiceRoom
	if len(rooms) > 0 {
		room = &moderation.ReportVoiceRoom{RoomID: rooms[0].RoomID, RevisionID: rooms[0].RevisionID}
	}
	origins, err := r.db.reader().GetReportHookOrigin(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, sqlite.Wrap(err, "read report hook origin")
	}
	var origin *moderation.ReportHookOrigin
	if len(origins) > 0 {
		origin = &moderation.ReportHookOrigin{DecisionID: origins[0].HookDecisionID, Hook: origins[0].Hook, ReasonCode: origins[0].ReasonCode}
	}
	forward, err := r.reportForward(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, err
	}
	return reportOf(row, ids, rules, room, origin, forward), nil
}

// FindReport implements moderation.Repository.
func (r *Repository) FindReport(ctx context.Context, id string) (moderation.Report, error) {
	row, err := r.db.reader().GetReport(ctx, id)
	if err != nil {
		return moderation.Report{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "report "+id)
	}
	return r.reportFrom(ctx, row)
}

// FindReportByURI implements moderation.Repository.
func (r *Repository) FindReportByURI(ctx context.Context, uri string) (moderation.Report, error) {
	row, err := r.db.reader().GetReportByURI(ctx, sqlite.ToNullString(uri))
	if err != nil {
		return moderation.Report{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "report "+uri)
	}
	return r.reportFrom(ctx, row)
}

// FindReportStatus implements moderation.Repository.
func (r *Repository) FindReportStatus(ctx context.Context, reportID string) (moderation.ReportStatus, error) {
	row, err := r.db.reader().GetReportState(ctx, reportID)
	if err != nil {
		return moderation.ReportStatus{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "report state "+reportID)
	}
	return reportStatusFrom(row), nil
}

func reportStatusFrom(row moderationgen.ReportState) moderation.ReportStatus {
	return moderation.ReportStatus{
		ReportID: row.ReportID, State: moderation.ReportState(row.State), Outcome: moderation.ReportOutcome(row.Outcome.String),
		AssigneeUserID: row.AssigneeUserID.String, Since: sqlite.FromMillis(row.Since),
	}
}

// ListReportsOf implements moderation.Repository.
func (r *Repository) ListReportsOf(ctx context.Context, targetAccountID string) ([]moderation.Report, error) {
	rows, err := r.db.reader().ListReportsOfTarget(ctx, targetAccountID)
	if err != nil {
		return nil, sqlite.Wrap(err, "list reports")
	}
	reports, err := r.reportsWithParts(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]moderation.Report, len(rows))
	for i, row := range rows {
		out[i] = reports[row.ID]
	}
	return out, nil
}

// InsertReportAssignment implements moderation.Repository.
func (r *Repository) InsertReportAssignment(ctx context.Context, tx uow.Tx, a moderation.ReportAssignment) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertReportAssignment(ctx, moderationgen.InsertReportAssignmentParams{
		ID: a.ID, ReportID: a.ReportID, AssigneeUserID: sqlite.ToNullString(a.AssigneeUserID),
		AssignedByUserID: sqlite.ToNullString(a.AssignedByUserID), AssignedAt: sqlite.ToMillis(a.AssignedAt),
	}), "insert report assignment")
}

// InsertReportResolution implements moderation.Repository.
func (r *Repository) InsertReportResolution(ctx context.Context, tx uow.Tx, res moderation.ReportResolution) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.Wrap(q.InsertReportResolution(ctx, moderationgen.InsertReportResolutionParams{
		ID: res.ID, ReportID: res.ReportID, Outcome: string(res.Outcome),
		ResolvedByUserID: sqlite.ToNullString(res.ResolvedByUserID), ResolvedAt: sqlite.ToMillis(res.ResolvedAt),
	}), "insert report resolution")
}

// ReportStatusForUpdate implements moderation.Repository. SQLite serializes write
// transactions, so it needs no lock.
func (r *Repository) ReportStatusForUpdate(ctx context.Context, tx uow.Tx, reportID string) (moderation.ReportStatus, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.ReportStatus{}, err
	}
	row, err := q.GetReportStateForUpdate(ctx, reportID)
	if err != nil {
		return moderation.ReportStatus{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "report state "+reportID)
	}
	return reportStatusFrom(row), nil
}

// ListReports implements moderation.Repository.
func (r *Repository) ListReports(ctx context.Context, q appmoderation.ReportQuery, limit int) ([]moderation.ReportRecord, error) {
	var rows []moderationgen.ListReportsRow
	var err error
	if q.OldestFirst {
		var oldest []moderationgen.ListReportsOldestFirstRow
		oldest, err = r.db.reader().ListReportsOldestFirst(ctx, moderationgen.ListReportsOldestFirstParams{
			State: string(q.State), Category: string(q.Category), Assignee: q.AssigneeUserID, Unassigned: sqlite.BoolToInt(q.Unassigned),
			After: q.After, Lim: int64(limit),
		})
		for _, row := range oldest {
			rows = append(rows, moderationgen.ListReportsRow(row))
		}
	} else {
		rows, err = r.db.reader().ListReports(ctx, moderationgen.ListReportsParams{
			State: string(q.State), Category: string(q.Category), Assignee: q.AssigneeUserID, Unassigned: sqlite.BoolToInt(q.Unassigned),
			Before: q.After, Lim: int64(limit),
		})
	}
	if err != nil {
		return nil, sqlite.Wrap(err, "list reports")
	}
	return r.reportRecordsFrom(ctx, rows)
}

// reportRecordsFrom returns the reports of rows with their parts and their states, reading each
// part of all of them with one query (reportsWithParts).
func (r *Repository) reportRecordsFrom(ctx context.Context, rows []moderationgen.ListReportsRow) ([]moderation.ReportRecord, error) {
	reports := make([]moderationgen.Report, len(rows))
	for i, row := range rows {
		reports[i] = moderationgen.Report{
			ID: row.ID, Uri: row.Uri, ReporterAccountID: row.ReporterAccountID, TargetAccountID: row.TargetAccountID,
			Category: row.Category, Comment: row.Comment, Forwarded: row.Forwarded, FiledAt: row.FiledAt,
		}
	}
	parts, err := r.reportsWithParts(ctx, reports)
	if err != nil {
		return nil, err
	}
	out := make([]moderation.ReportRecord, len(rows))
	for i, row := range rows {
		out[i] = moderation.ReportRecord{Report: parts[row.ID], Status: reportStatusFrom(moderationgen.ReportState{
			ReportID: row.ID, State: row.State, Outcome: row.Outcome, AssigneeUserID: row.AssigneeUserID, Since: row.Since,
		})}
	}
	return out, nil
}

// ListReportsByReporter implements moderation.Repository.
func (r *Repository) ListReportsByReporter(ctx context.Context, reporterAccountID, before string, limit int) ([]moderation.ReportRecord, error) {
	rows, err := r.db.reader().ListReportsByReporter(ctx, moderationgen.ListReportsByReporterParams{
		ReporterAccountID: reporterAccountID, Before: before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list reports by reporter")
	}
	records := make([]moderationgen.ListReportsRow, len(rows))
	for i, row := range rows {
		records[i] = moderationgen.ListReportsRow(row)
	}
	return r.reportRecordsFrom(ctx, records)
}

func domainActionRecordFrom(row moderationgen.ListDomainModerationActionsRow) moderation.DomainActionRecord {
	rec := moderation.DomainActionRecord{Action: moderation.DomainAction{
		ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: row.PublicComment.String,
		PrivateNote: row.PrivateNote.String, Obfuscate: row.Obfuscate != 0, IssuedByUserID: row.IssuedByUserID.String,
		ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt), IssuedAt: sqlite.FromMillis(row.IssuedAt),
	}}
	if row.LiftActionID.Valid {
		rec.Lift = &moderation.DomainLift{ActionID: row.ID, LiftedByUserID: row.LiftedByUserID.String, LiftedAt: sqlite.FromMillis(row.LiftedAt.Int64)}
	}
	return rec
}

// FindDomainActionRecord implements moderation.Repository.
func (r *Repository) FindDomainActionRecord(ctx context.Context, id string) (moderation.DomainActionRecord, error) {
	row, err := r.db.reader().GetDomainModerationActionRecord(ctx, id)
	if err != nil {
		return moderation.DomainActionRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "domain moderation action "+id)
	}
	return domainActionRecordFrom(moderationgen.ListDomainModerationActionsRow(row)), nil
}

// ListDomainActions implements moderation.Repository.
func (r *Repository) ListDomainActions(ctx context.Context, q appmoderation.DomainActionQuery, limit int) ([]moderation.DomainActionRecord, error) {
	var at int64
	if q.InEffectAt != nil {
		at = sqlite.ToMillis(*q.InEffectAt)
	}
	var kinds string // "" keeps every kind
	if len(q.Kinds) > 0 {
		var err error
		if kinds, err = sqlite.JSONList(domainActionKindStrings(q.Kinds)); err != nil {
			return nil, err
		}
	}
	rows, err := r.db.reader().ListDomainModerationActions(ctx, moderationgen.ListDomainModerationActionsParams{
		Domain: q.Domain, Kinds: kinds, InEffectAt: at, Before: q.Before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list domain moderation actions")
	}
	out := make([]moderation.DomainActionRecord, len(rows))
	for i, row := range rows {
		out[i] = domainActionRecordFrom(row)
	}
	return out, nil
}

func actionRecordFrom(row moderationgen.ListModerationActionsRow) moderation.ActionRecord {
	rec := moderation.ActionRecord{Action: moderation.Action{
		ID: row.ID, TargetAccountID: row.TargetAccountID, Kind: moderation.ActionKind(row.Kind),
		ReasonCategory: moderation.Category(row.ReasonCategory), PublicReason: row.PublicReason.String,
		PrivateNote: row.PrivateNote.String, ReportID: row.ReportID.String, Issuer: moderation.Issuer(row.Issuer),
		IssuedByUserID: row.IssuedByUserID.String, NotifyTarget: row.NotifyTarget != 0,
		ExpiresAt: sqlite.FromNullMillis(row.ExpiresAt), IssuedAt: sqlite.FromMillis(row.IssuedAt),
	}}
	if row.LiftActionID.Valid {
		rec.Lift = &moderation.Lift{
			ActionID: row.ID, LiftedByUserID: row.LiftedByUserID.String, Reason: row.LiftReason.String,
			LiftedAt: sqlite.FromMillis(row.LiftedAt.Int64),
		}
	}
	return rec
}

// FindActionRecord implements moderation.Repository.
func (r *Repository) FindActionRecord(ctx context.Context, id string) (moderation.ActionRecord, error) {
	row, err := r.db.reader().GetModerationActionRecord(ctx, id)
	if err != nil {
		return moderation.ActionRecord{}, sqlite.NotFound(err, appmoderation.ErrNotFound, "moderation action "+id)
	}
	rec := actionRecordFrom(moderationgen.ListModerationActionsRow(row))
	if rec.Action.StatusIDs, err = actionStatusIDs(ctx, r.db.reader(), id); err != nil {
		return moderation.ActionRecord{}, err
	}
	return rec, nil
}

// ListActions implements moderation.Repository.
func (r *Repository) ListActions(ctx context.Context, q appmoderation.ActionQuery, limit int) ([]moderation.ActionRecord, error) {
	var at int64
	if q.InEffectAt != nil {
		at = sqlite.ToMillis(*q.InEffectAt)
	}
	rows, err := r.db.reader().ListModerationActions(ctx, moderationgen.ListModerationActionsParams{
		AccountID: q.AccountID, InEffectAt: at, Before: q.Before, Lim: int64(limit),
	})
	if err != nil {
		return nil, sqlite.Wrap(err, "list moderation actions")
	}
	out := make([]moderation.ActionRecord, len(rows))
	for i, row := range rows {
		out[i] = actionRecordFrom(row)
		if out[i].Action.StatusIDs, err = actionStatusIDs(ctx, r.db.reader(), row.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// domainActionKindStrings returns the kinds as strings, never nil.
func domainActionKindStrings(kinds []moderation.DomainActionKind) []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// reportOf returns the report of row with its parts.
func reportOf(row moderationgen.Report, ids, rules []string, room *moderation.ReportVoiceRoom, origin *moderation.ReportHookOrigin, forward *moderation.ReportForward) moderation.Report {
	return moderation.Report{
		ID: row.ID, URI: row.Uri.String, ReporterAccountID: row.ReporterAccountID, TargetAccountID: row.TargetAccountID,
		Category: moderation.Category(row.Category), Comment: row.Comment, Forwarded: row.Forwarded != 0,
		FiledAt: sqlite.FromMillis(row.FiledAt), StatusIDs: ids, RuleIDs: rules, VoiceRoom: room, HookOrigin: origin, Forward: forward,
	}
}
