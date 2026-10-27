// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// ModerationRepository implements moderation.Repository on PostgreSQL.
type ModerationRepository struct {
	db *DB
}

var _ appmoderation.Repository = (*ModerationRepository)(nil)

// ModerationRepository returns the moderation module's repository on d.
func (d *DB) ModerationRepository() *ModerationRepository { return &ModerationRepository{db: d} }

// InsertAction implements moderation.Repository.
func (r *ModerationRepository) InsertAction(ctx context.Context, tx uow.Tx, a moderation.Action) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	err = q.InsertModerationAction(ctx, postgresgen.InsertModerationActionParams{
		ID: a.ID, TargetAccountID: a.TargetAccountID, Kind: string(a.Kind), ReasonCategory: string(a.ReasonCategory),
		PublicReason: toNullString(a.PublicReason), PrivateNote: toNullString(a.PrivateNote), ReportID: toNullString(a.ReportID),
		Issuer: string(a.Issuer), IssuedByUserID: toNullString(a.IssuedByUserID), NotifyTarget: a.NotifyTarget,
		ExpiresAt: a.ExpiresAt, IssuedAt: a.IssuedAt,
	})
	if err != nil {
		return wrap(err, "insert moderation action")
	}
	for _, id := range a.StatusIDs {
		if err := q.InsertModerationActionStatus(ctx, postgresgen.InsertModerationActionStatusParams{ActionID: a.ID, StatusID: id}); err != nil {
			return wrap(err, "insert moderation action status")
		}
	}
	return nil
}

// actionStatusIDs returns the statuses of an action on statuses, nil for none.
func actionStatusIDs(ctx context.Context, q *postgresgen.Queries, actionID string) ([]string, error) {
	ids, err := q.ListModerationActionStatusIDs(ctx, actionID)
	if err != nil {
		return nil, wrap(err, "list moderation action statuses")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

// Action implements moderation.Repository.
func (r *ModerationRepository) Action(ctx context.Context, tx uow.Tx, id string) (moderation.Action, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.Action{}, err
	}
	row, err := q.GetModerationAction(ctx, id)
	if err != nil {
		return moderation.Action{}, notFound(err, appmoderation.ErrNotFound, "moderation action "+id)
	}
	statusIDs, err := actionStatusIDs(ctx, q, id)
	if err != nil {
		return moderation.Action{}, err
	}
	return moderation.Action{
		ID: row.ID, TargetAccountID: row.TargetAccountID, Kind: moderation.ActionKind(row.Kind),
		ReasonCategory: moderation.Category(row.ReasonCategory), PublicReason: fromNullString(row.PublicReason),
		PrivateNote: fromNullString(row.PrivateNote), ReportID: fromNullString(row.ReportID), Issuer: moderation.Issuer(row.Issuer),
		IssuedByUserID: fromNullString(row.IssuedByUserID), NotifyTarget: row.NotifyTarget,
		ExpiresAt: utcPtr(row.ExpiresAt), IssuedAt: utc(row.IssuedAt), StatusIDs: statusIDs,
	}, nil
}

// InsertActionLift implements moderation.Repository.
func (r *ModerationRepository) InsertActionLift(ctx context.Context, tx uow.Tx, l moderation.Lift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertModerationActionLift(ctx, postgresgen.InsertModerationActionLiftParams{
		ActionID: l.ActionID, LiftedByUserID: toNullString(l.LiftedByUserID), Reason: toNullString(l.Reason),
		LiftedAt: l.LiftedAt,
	})
	if err != nil {
		return wrap(err, "insert moderation action lift")
	}
	if n == 0 {
		return fmt.Errorf("lift of moderation action %s: %w", l.ActionID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// InsertDomainAction implements moderation.Repository.
func (r *ModerationRepository) InsertDomainAction(ctx context.Context, tx uow.Tx, a moderation.DomainAction) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertDomainModerationAction(ctx, postgresgen.InsertDomainModerationActionParams{
		ID: a.ID, Domain: a.Domain, Kind: string(a.Kind), PublicComment: toNullString(a.PublicComment),
		PrivateNote: toNullString(a.PrivateNote), Obfuscate: a.Obfuscate,
		IssuedByUserID: toNullString(a.IssuedByUserID), ExpiresAt: a.ExpiresAt, IssuedAt: a.IssuedAt,
	}), "insert domain moderation action")
}

// DomainAction implements moderation.Repository.
func (r *ModerationRepository) DomainAction(ctx context.Context, tx uow.Tx, id string) (moderation.DomainAction, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.DomainAction{}, err
	}
	row, err := q.GetDomainModerationAction(ctx, id)
	if err != nil {
		return moderation.DomainAction{}, notFound(err, appmoderation.ErrNotFound, "domain moderation action "+id)
	}
	return moderation.DomainAction{
		ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: fromNullString(row.PublicComment),
		PrivateNote: fromNullString(row.PrivateNote), Obfuscate: row.Obfuscate, IssuedByUserID: fromNullString(row.IssuedByUserID),
		ExpiresAt: utcPtr(row.ExpiresAt), IssuedAt: utc(row.IssuedAt),
	}, nil
}

// InsertDomainActionLift implements moderation.Repository.
func (r *ModerationRepository) InsertDomainActionLift(ctx context.Context, tx uow.Tx, l moderation.DomainLift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertDomainModerationActionLift(ctx, postgresgen.InsertDomainModerationActionLiftParams{
		ActionID: l.ActionID, LiftedByUserID: toNullString(l.LiftedByUserID), LiftedAt: l.LiftedAt,
	})
	if err != nil {
		return wrap(err, "insert domain moderation action lift")
	}
	if n == 0 {
		return fmt.Errorf("lift of domain moderation action %s: %w", l.ActionID, appmoderation.ErrAlreadyRecorded)
	}
	return nil
}

// InsertReport implements moderation.Repository.
func (r *ModerationRepository) InsertReport(ctx context.Context, tx uow.Tx, rep moderation.Report) (bool, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return false, err
	}
	n, err := q.InsertReport(ctx, postgresgen.InsertReportParams{
		ID: rep.ID, Uri: toNullString(rep.URI), ReporterAccountID: rep.ReporterAccountID, TargetAccountID: rep.TargetAccountID,
		Category: string(rep.Category), Comment: rep.Comment, Forwarded: rep.Forwarded, FiledAt: rep.FiledAt,
	})
	if err != nil || n == 0 {
		return false, wrap(err, "insert report")
	}
	for _, id := range rep.StatusIDs {
		if err := q.InsertReportStatus(ctx, postgresgen.InsertReportStatusParams{ReportID: rep.ID, StatusID: id}); err != nil {
			return false, wrap(err, "insert report status")
		}
	}
	for _, id := range rep.RuleIDs {
		if err := q.InsertReportRuleCitation(ctx, postgresgen.InsertReportRuleCitationParams{ReportID: rep.ID, RuleID: id}); err != nil {
			return false, wrap(err, "insert report rule citation")
		}
	}
	if v := rep.VoiceRoom; v != nil {
		err := q.InsertReportVoiceRoom(ctx, postgresgen.InsertReportVoiceRoomParams{ReportID: rep.ID, RoomID: v.RoomID, RevisionID: v.RevisionID})
		if err != nil {
			return false, wrap(err, "insert report voice room")
		}
	}
	return true, nil
}

func restrictionRowsFrom(target string, restriction string, source string, until *time.Time) moderation.RestrictionRow {
	return moderation.RestrictionRow{
		Target: target, Restriction: moderation.Restriction(restriction), SourceActionID: source, EffectiveUntil: utcPtr(until),
	}
}

// InsertAccountRestriction implements moderation.Repository.
func (r *ModerationRepository) InsertAccountRestriction(ctx context.Context, tx uow.Tx, row moderation.RestrictionRow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertAccountRestriction(ctx, postgresgen.InsertAccountRestrictionParams{
		AccountID: row.Target, Restriction: string(row.Restriction), SourceActionID: row.SourceActionID,
		EffectiveUntil: row.EffectiveUntil,
	}), "insert account restriction")
}

// DeleteAccountRestrictionsOf implements moderation.Repository.
func (r *ModerationRepository) DeleteAccountRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteAccountRestrictionsOfAction(ctx, actionID), "delete account restrictions")
}

// LockAccountRestrictions implements moderation.Repository with a transaction-scoped advisory
// lock on the account, so that the actions and lifts on it serialize under READ COMMITTED.
func (r *ModerationRepository) LockAccountRestrictions(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.LockModerationTarget(ctx, "moderation:account:"+accountID), "lock the account restrictions")
}

// LockDomainRestrictions implements moderation.Repository with a transaction-scoped advisory
// lock on the domain, so that the actions and lifts on it serialize under READ COMMITTED.
func (r *ModerationRepository) LockDomainRestrictions(ctx context.Context, tx uow.Tx, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.LockModerationTarget(ctx, "moderation:domain:"+domain), "lock the domain restrictions")
}

// AccountRestrictionRows implements moderation.Repository.
func (r *ModerationRepository) AccountRestrictionRows(ctx context.Context, tx uow.Tx, accountID string) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAccountRestrictionRows(ctx, accountID)
	if err != nil {
		return nil, wrap(err, "list account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// DeleteExpiredAccountRestrictions implements moderation.Repository.
func (r *ModerationRepository) DeleteExpiredAccountRestrictions(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]moderation.RestrictionRow, error) {
	n, err := toInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteExpiredAccountRestrictions(ctx, postgresgen.DeleteExpiredAccountRestrictionsParams{Now: now, BatchLimit: n})
	if err != nil {
		return nil, wrap(err, "delete expired account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// InsertDomainRestriction implements moderation.Repository.
func (r *ModerationRepository) InsertDomainRestriction(ctx context.Context, tx uow.Tx, row moderation.RestrictionRow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertDomainRestriction(ctx, postgresgen.InsertDomainRestrictionParams{
		Domain: row.Target, Restriction: string(row.Restriction), SourceActionID: row.SourceActionID,
		EffectiveUntil: row.EffectiveUntil,
	}), "insert domain restriction")
}

// DeleteDomainRestrictionsOf implements moderation.Repository.
func (r *ModerationRepository) DeleteDomainRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.DeleteDomainRestrictionsOfAction(ctx, actionID), "delete domain restrictions")
}

// DomainRestrictionRows implements moderation.Repository.
func (r *ModerationRepository) DomainRestrictionRows(ctx context.Context, tx uow.Tx, domain string) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDomainRestrictionRows(ctx, domain)
	if err != nil {
		return nil, wrap(err, "list domain restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.Domain, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// UpsertReportStatus implements moderation.Repository.
func (r *ModerationRepository) UpsertReportStatus(ctx context.Context, tx uow.Tx, s moderation.ReportStatus) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.UpsertReportState(ctx, postgresgen.UpsertReportStateParams{
		ReportID: s.ReportID, State: string(s.State), Outcome: toNullString(string(s.Outcome)),
		AssigneeUserID: toNullString(s.AssigneeUserID), Since: s.Since,
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
func (r *ModerationRepository) AccountRestrictionsInEffect(ctx context.Context, accountID string, now time.Time) ([]moderation.Restriction, error) {
	rows, err := r.db.reader().ListAccountRestrictionsInEffect(ctx, postgresgen.ListAccountRestrictionsInEffectParams{
		AccountID: accountID, Now: now,
	})
	if err != nil {
		return nil, wrap(err, "list account restrictions in effect")
	}
	return restrictionsFrom(rows), nil
}

// DomainRestrictionsInEffect implements moderation.Repository.
func (r *ModerationRepository) DomainRestrictionsInEffect(ctx context.Context, domains []string, now time.Time) ([]moderation.Restriction, error) {
	rows, err := r.db.reader().ListDomainRestrictionsInEffect(ctx, postgresgen.ListDomainRestrictionsInEffectParams{
		Domains: domains, Now: now,
	})
	if err != nil {
		return nil, wrap(err, "list domain restrictions in effect")
	}
	return restrictionsFrom(rows), nil
}

func (r *ModerationRepository) reportFrom(ctx context.Context, row postgresgen.Report) (moderation.Report, error) {
	ids, err := r.db.reader().ListReportStatusIDs(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, wrap(err, "list report statuses")
	}
	rules, err := r.db.reader().ListReportRuleIDs(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, wrap(err, "list report rule citations")
	}
	rooms, err := r.db.reader().GetReportVoiceRoom(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, wrap(err, "read report voice room")
	}
	var room *moderation.ReportVoiceRoom
	if len(rooms) > 0 {
		room = &moderation.ReportVoiceRoom{RoomID: rooms[0].RoomID, RevisionID: rooms[0].RevisionID}
	}
	return moderation.Report{
		ID: row.ID, URI: fromNullString(row.Uri), ReporterAccountID: row.ReporterAccountID, TargetAccountID: row.TargetAccountID,
		Category: moderation.Category(row.Category), Comment: row.Comment, Forwarded: row.Forwarded,
		FiledAt: utc(row.FiledAt), StatusIDs: ids, RuleIDs: rules, VoiceRoom: room,
	}, nil
}

// FindReport implements moderation.Repository.
func (r *ModerationRepository) FindReport(ctx context.Context, id string) (moderation.Report, error) {
	row, err := r.db.reader().GetReport(ctx, id)
	if err != nil {
		return moderation.Report{}, notFound(err, appmoderation.ErrNotFound, "report "+id)
	}
	return r.reportFrom(ctx, row)
}

// FindReportByURI implements moderation.Repository.
func (r *ModerationRepository) FindReportByURI(ctx context.Context, uri string) (moderation.Report, error) {
	row, err := r.db.reader().GetReportByURI(ctx, toNullString(uri))
	if err != nil {
		return moderation.Report{}, notFound(err, appmoderation.ErrNotFound, "report "+uri)
	}
	return r.reportFrom(ctx, row)
}

// FindReportStatus implements moderation.Repository.
func (r *ModerationRepository) FindReportStatus(ctx context.Context, reportID string) (moderation.ReportStatus, error) {
	row, err := r.db.reader().GetReportState(ctx, reportID)
	if err != nil {
		return moderation.ReportStatus{}, notFound(err, appmoderation.ErrNotFound, "report state "+reportID)
	}
	return reportStatusFrom(row), nil
}

func reportStatusFrom(row postgresgen.ReportState) moderation.ReportStatus {
	return moderation.ReportStatus{
		ReportID: row.ReportID, State: moderation.ReportState(row.State), Outcome: moderation.ReportOutcome(fromNullString(row.Outcome)),
		AssigneeUserID: fromNullString(row.AssigneeUserID), Since: utc(row.Since),
	}
}

// ListReportsOf implements moderation.Repository.
func (r *ModerationRepository) ListReportsOf(ctx context.Context, targetAccountID string) ([]moderation.Report, error) {
	rows, err := r.db.reader().ListReportsOfTarget(ctx, targetAccountID)
	if err != nil {
		return nil, wrap(err, "list reports")
	}
	out := make([]moderation.Report, len(rows))
	for i, row := range rows {
		if out[i], err = r.reportFrom(ctx, row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// InsertReportAssignment implements moderation.Repository.
func (r *ModerationRepository) InsertReportAssignment(ctx context.Context, tx uow.Tx, a moderation.ReportAssignment) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertReportAssignment(ctx, postgresgen.InsertReportAssignmentParams{
		ID: a.ID, ReportID: a.ReportID, AssigneeUserID: toNullString(a.AssigneeUserID),
		AssignedByUserID: toNullString(a.AssignedByUserID), AssignedAt: a.AssignedAt,
	}), "insert report assignment")
}

// InsertReportResolution implements moderation.Repository.
func (r *ModerationRepository) InsertReportResolution(ctx context.Context, tx uow.Tx, res moderation.ReportResolution) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.InsertReportResolution(ctx, postgresgen.InsertReportResolutionParams{
		ID: res.ID, ReportID: res.ReportID, Outcome: string(res.Outcome),
		ResolvedByUserID: toNullString(res.ResolvedByUserID), ResolvedAt: res.ResolvedAt,
	}), "insert report resolution")
}

// ReportStatusForUpdate implements moderation.Repository; it locks the row until the
// transaction ends.
func (r *ModerationRepository) ReportStatusForUpdate(ctx context.Context, tx uow.Tx, reportID string) (moderation.ReportStatus, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.ReportStatus{}, err
	}
	row, err := q.GetReportStateForUpdate(ctx, reportID)
	if err != nil {
		return moderation.ReportStatus{}, notFound(err, appmoderation.ErrNotFound, "report state "+reportID)
	}
	return reportStatusFrom(row), nil
}

// ListReports implements moderation.Repository.
func (r *ModerationRepository) ListReports(ctx context.Context, q appmoderation.ReportQuery, limit int) ([]moderation.ReportRecord, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListReports(ctx, postgresgen.ListReportsParams{State: string(q.State), Before: toNullString(q.Before), Lim: lim})
	if err != nil {
		return nil, wrap(err, "list reports")
	}
	out := make([]moderation.ReportRecord, len(rows))
	for i, row := range rows {
		if out[i], err = r.reportRecordFrom(ctx, row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// reportRecordFrom reads the statuses and rules of the report of row.
func (r *ModerationRepository) reportRecordFrom(ctx context.Context, row postgresgen.ListReportsRow) (moderation.ReportRecord, error) {
	rep, err := r.reportFrom(ctx, postgresgen.Report{
		ID: row.ID, Uri: row.Uri, ReporterAccountID: row.ReporterAccountID, TargetAccountID: row.TargetAccountID,
		Category: row.Category, Comment: row.Comment, Forwarded: row.Forwarded, FiledAt: row.FiledAt,
	})
	if err != nil {
		return moderation.ReportRecord{}, err
	}
	return moderation.ReportRecord{Report: rep, Status: reportStatusFrom(postgresgen.ReportState{
		ReportID: row.ID, State: row.State, Outcome: row.Outcome, AssigneeUserID: row.AssigneeUserID, Since: row.Since,
	})}, nil
}

// ListReportsByReporter implements moderation.Repository.
func (r *ModerationRepository) ListReportsByReporter(ctx context.Context, reporterAccountID, before string, limit int) ([]moderation.ReportRecord, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListReportsByReporter(ctx, postgresgen.ListReportsByReporterParams{
		ReporterAccountID: reporterAccountID, Before: toNullString(before), Lim: lim,
	})
	if err != nil {
		return nil, wrap(err, "list reports by reporter")
	}
	out := make([]moderation.ReportRecord, len(rows))
	for i, row := range rows {
		if out[i], err = r.reportRecordFrom(ctx, postgresgen.ListReportsRow(row)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func domainActionRecordFrom(row postgresgen.ListDomainModerationActionsRow) moderation.DomainActionRecord {
	rec := moderation.DomainActionRecord{Action: moderation.DomainAction{
		ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: fromNullString(row.PublicComment),
		PrivateNote: fromNullString(row.PrivateNote), Obfuscate: row.Obfuscate, IssuedByUserID: fromNullString(row.IssuedByUserID),
		ExpiresAt: utcPtr(row.ExpiresAt), IssuedAt: utc(row.IssuedAt),
	}}
	if row.LiftActionID != nil && row.LiftedAt != nil {
		rec.Lift = &moderation.DomainLift{ActionID: row.ID, LiftedByUserID: fromNullString(row.LiftedByUserID), LiftedAt: utc(*row.LiftedAt)}
	}
	return rec
}

// FindDomainActionRecord implements moderation.Repository.
func (r *ModerationRepository) FindDomainActionRecord(ctx context.Context, id string) (moderation.DomainActionRecord, error) {
	row, err := r.db.reader().GetDomainModerationActionRecord(ctx, id)
	if err != nil {
		return moderation.DomainActionRecord{}, notFound(err, appmoderation.ErrNotFound, "domain moderation action "+id)
	}
	return domainActionRecordFrom(postgresgen.ListDomainModerationActionsRow(row)), nil
}

// ListDomainActions implements moderation.Repository.
func (r *ModerationRepository) ListDomainActions(ctx context.Context, q appmoderation.DomainActionQuery, limit int) ([]moderation.DomainActionRecord, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainModerationActions(ctx, postgresgen.ListDomainModerationActionsParams{
		Domain: q.Domain, Kinds: domainActionKindStrings(q.Kinds), InEffectAt: q.InEffectAt, Before: toNullString(q.Before), Lim: lim,
	})
	if err != nil {
		return nil, wrap(err, "list domain moderation actions")
	}
	out := make([]moderation.DomainActionRecord, len(rows))
	for i, row := range rows {
		out[i] = domainActionRecordFrom(row)
	}
	return out, nil
}

func actionRecordFrom(row postgresgen.ListModerationActionsRow) moderation.ActionRecord {
	rec := moderation.ActionRecord{Action: moderation.Action{
		ID: row.ID, TargetAccountID: row.TargetAccountID, Kind: moderation.ActionKind(row.Kind),
		ReasonCategory: moderation.Category(row.ReasonCategory), PublicReason: fromNullString(row.PublicReason),
		PrivateNote: fromNullString(row.PrivateNote), ReportID: fromNullString(row.ReportID), Issuer: moderation.Issuer(row.Issuer),
		IssuedByUserID: fromNullString(row.IssuedByUserID), NotifyTarget: row.NotifyTarget,
		ExpiresAt: utcPtr(row.ExpiresAt), IssuedAt: utc(row.IssuedAt),
	}}
	if row.LiftActionID != nil && row.LiftedAt != nil {
		rec.Lift = &moderation.Lift{
			ActionID: row.ID, LiftedByUserID: fromNullString(row.LiftedByUserID), Reason: fromNullString(row.LiftReason),
			LiftedAt: utc(*row.LiftedAt),
		}
	}
	return rec
}

// FindActionRecord implements moderation.Repository.
func (r *ModerationRepository) FindActionRecord(ctx context.Context, id string) (moderation.ActionRecord, error) {
	row, err := r.db.reader().GetModerationActionRecord(ctx, id)
	if err != nil {
		return moderation.ActionRecord{}, notFound(err, appmoderation.ErrNotFound, "moderation action "+id)
	}
	rec := actionRecordFrom(postgresgen.ListModerationActionsRow(row))
	if rec.Action.StatusIDs, err = actionStatusIDs(ctx, r.db.reader(), id); err != nil {
		return moderation.ActionRecord{}, err
	}
	return rec, nil
}

// ListActions implements moderation.Repository.
func (r *ModerationRepository) ListActions(ctx context.Context, q appmoderation.ActionQuery, limit int) ([]moderation.ActionRecord, error) {
	lim, err := toInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListModerationActions(ctx, postgresgen.ListModerationActionsParams{
		AccountID: toNullString(q.AccountID), InEffectAt: q.InEffectAt, Before: toNullString(q.Before), Lim: lim,
	})
	if err != nil {
		return nil, wrap(err, "list moderation actions")
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
