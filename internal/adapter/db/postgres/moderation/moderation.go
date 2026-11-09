// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// Repository implements moderation.Repository on PostgreSQL.
type Repository struct {
	db *moduleDB
}

var _ appmoderation.Repository = (*Repository)(nil)

// NewRepository returns the moderation module's repository on d.
func NewRepository(d *postgres.DB) *Repository {
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
		PublicReason: postgres.ToNullString(a.PublicReason), PrivateNote: postgres.ToNullString(a.PrivateNote), ReportID: postgres.ToNullString(a.ReportID),
		Issuer: string(a.Issuer), IssuedByUserID: postgres.ToNullString(a.IssuedByUserID), NotifyTarget: a.NotifyTarget,
		ExpiresAt: a.ExpiresAt, IssuedAt: a.IssuedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert moderation action")
	}
	if len(a.StatusIDs) == 0 {
		return nil
	}
	return postgres.Wrap(q.InsertModerationActionStatuses(ctx, moderationgen.InsertModerationActionStatusesParams{ActionID: a.ID, StatusIds: a.StatusIDs}), "insert moderation action statuses")
}

// actionStatusIDs returns the statuses of an action on statuses, nil for none.
func actionStatusIDs(ctx context.Context, q *moderationgen.Queries, actionID string) ([]string, error) {
	ids, err := q.ListModerationActionStatusIDs(ctx, actionID)
	if err != nil {
		return nil, postgres.Wrap(err, "list moderation action statuses")
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
		return moderation.Action{}, postgres.NotFound(err, appmoderation.ErrNotFound, "moderation action "+id)
	}
	statusIDs, err := actionStatusIDs(ctx, q, id)
	if err != nil {
		return moderation.Action{}, err
	}
	return moderation.Action{
		ID: row.ID, TargetAccountID: row.TargetAccountID, Kind: moderation.ActionKind(row.Kind),
		ReasonCategory: moderation.Category(row.ReasonCategory), PublicReason: postgres.FromNullString(row.PublicReason),
		PrivateNote: postgres.FromNullString(row.PrivateNote), ReportID: postgres.FromNullString(row.ReportID), Issuer: moderation.Issuer(row.Issuer),
		IssuedByUserID: postgres.FromNullString(row.IssuedByUserID), NotifyTarget: row.NotifyTarget,
		ExpiresAt: postgres.UTCPtr(row.ExpiresAt), IssuedAt: postgres.UTC(row.IssuedAt), StatusIDs: statusIDs,
	}, nil
}

// InsertActionLift implements moderation.Repository.
func (r *Repository) InsertActionLift(ctx context.Context, tx uow.Tx, l moderation.Lift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertModerationActionLift(ctx, moderationgen.InsertModerationActionLiftParams{
		ActionID: l.ActionID, LiftedByUserID: postgres.ToNullString(l.LiftedByUserID), Reason: postgres.ToNullString(l.Reason),
		LiftedAt: l.LiftedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert moderation action lift")
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
	return postgres.Wrap(q.InsertDomainModerationAction(ctx, moderationgen.InsertDomainModerationActionParams{
		ID: a.ID, Domain: a.Domain, Kind: string(a.Kind), PublicComment: postgres.ToNullString(a.PublicComment),
		PrivateNote: postgres.ToNullString(a.PrivateNote), Obfuscate: a.Obfuscate,
		IssuedByUserID: postgres.ToNullString(a.IssuedByUserID), ExpiresAt: a.ExpiresAt, IssuedAt: a.IssuedAt,
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
		return moderation.DomainAction{}, postgres.NotFound(err, appmoderation.ErrNotFound, "domain moderation action "+id)
	}
	return moderation.DomainAction{
		ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: postgres.FromNullString(row.PublicComment),
		PrivateNote: postgres.FromNullString(row.PrivateNote), Obfuscate: row.Obfuscate, IssuedByUserID: postgres.FromNullString(row.IssuedByUserID),
		ExpiresAt: postgres.UTCPtr(row.ExpiresAt), IssuedAt: postgres.UTC(row.IssuedAt),
	}, nil
}

// InsertDomainActionLift implements moderation.Repository.
func (r *Repository) InsertDomainActionLift(ctx context.Context, tx uow.Tx, l moderation.DomainLift) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	n, err := q.InsertDomainModerationActionLift(ctx, moderationgen.InsertDomainModerationActionLiftParams{
		ActionID: l.ActionID, LiftedByUserID: postgres.ToNullString(l.LiftedByUserID), LiftedAt: l.LiftedAt,
	})
	if err != nil {
		return postgres.Wrap(err, "insert domain moderation action lift")
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
			return false, postgres.Wrap(err, "insert report hook origin")
		}
	}
	n, err := q.InsertReport(ctx, moderationgen.InsertReportParams{
		ID: rep.ID, Uri: postgres.ToNullString(rep.URI), ReporterAccountID: rep.ReporterAccountID, TargetAccountID: rep.TargetAccountID,
		Category: string(rep.Category), Comment: rep.Comment, Forwarded: rep.Forwarded, FiledAt: rep.FiledAt,
	})
	if err != nil || n == 0 {
		return false, postgres.Wrap(err, "insert report")
	}
	if len(rep.StatusIDs) > 0 {
		if err := q.InsertReportStatuses(ctx, moderationgen.InsertReportStatusesParams{ReportID: rep.ID, StatusIds: rep.StatusIDs}); err != nil {
			return false, postgres.Wrap(err, "insert report statuses")
		}
	}
	if len(rep.RuleIDs) > 0 {
		if err := q.InsertReportRuleCitations(ctx, moderationgen.InsertReportRuleCitationsParams{ReportID: rep.ID, RuleIds: rep.RuleIDs}); err != nil {
			return false, postgres.Wrap(err, "insert report rule citations")
		}
	}
	if v := rep.VoiceRoom; v != nil {
		err := q.InsertReportVoiceRoom(ctx, moderationgen.InsertReportVoiceRoomParams{ReportID: rep.ID, RoomID: v.RoomID, RevisionID: v.RevisionID})
		if err != nil {
			return false, postgres.Wrap(err, "insert report voice room")
		}
	}
	return true, nil
}

func restrictionRowsFrom(target string, restriction string, source string, until *time.Time) moderation.RestrictionRow {
	return moderation.RestrictionRow{
		Target: target, Restriction: moderation.Restriction(restriction), SourceActionID: source, EffectiveUntil: postgres.UTCPtr(until),
	}
}

// InsertAccountRestriction implements moderation.Repository.
func (r *Repository) InsertAccountRestriction(ctx context.Context, tx uow.Tx, row moderation.RestrictionRow) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertAccountRestriction(ctx, moderationgen.InsertAccountRestrictionParams{
		AccountID: row.Target, Restriction: string(row.Restriction), SourceActionID: row.SourceActionID,
		EffectiveUntil: row.EffectiveUntil,
	}), "insert account restriction")
}

// DeleteAccountRestrictionsOf implements moderation.Repository.
func (r *Repository) DeleteAccountRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteAccountRestrictionsOfAction(ctx, actionID), "delete account restrictions")
}

// LockAccountRestrictions implements moderation.Repository with a transaction-scoped advisory
// lock on the account, so that the actions and lifts on it serialize under READ COMMITTED.
func (r *Repository) LockAccountRestrictions(ctx context.Context, tx uow.Tx, accountID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.LockModerationTarget(ctx, "moderation:account:"+accountID), "lock the account restrictions")
}

// LockDomainRestrictions implements moderation.Repository with a transaction-scoped advisory
// lock on the domain, so that the actions and lifts on it serialize under READ COMMITTED.
func (r *Repository) LockDomainRestrictions(ctx context.Context, tx uow.Tx, domain string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.LockModerationTarget(ctx, "moderation:domain:"+domain), "lock the domain restrictions")
}

// AccountRestrictionRows implements moderation.Repository.
func (r *Repository) AccountRestrictionRows(ctx context.Context, tx uow.Tx, accountID string) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAccountRestrictionRows(ctx, accountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// AccountRestrictionRowsOf implements moderation.Repository.
func (r *Repository) AccountRestrictionRowsOf(ctx context.Context, tx uow.Tx, accountIDs []string) (map[string][]moderation.RestrictionRow, error) {
	out := make(map[string][]moderation.RestrictionRow, len(accountIDs))
	if len(accountIDs) == 0 {
		return out, nil
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	ids := accountIDs
	rows, err := q.ListAccountRestrictionRowsOf(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list account restrictions of accounts")
	}
	for _, row := range rows {
		out[row.AccountID] = append(out[row.AccountID], restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil))
	}
	return out, nil
}

// DeleteExpiredAccountRestrictions implements moderation.Repository.
func (r *Repository) DeleteExpiredAccountRestrictions(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]moderation.RestrictionRow, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteExpiredAccountRestrictions(ctx, moderationgen.DeleteExpiredAccountRestrictionsParams{Now: now, BatchLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "delete expired account restrictions")
	}
	out := make([]moderation.RestrictionRow, len(rows))
	for i, row := range rows {
		out[i] = restrictionRowsFrom(row.AccountID, row.Restriction, row.SourceActionID, row.EffectiveUntil)
	}
	return out, nil
}

// DeleteExpiredDomainRestrictions implements moderation.Repository.
func (r *Repository) DeleteExpiredDomainRestrictions(ctx context.Context, tx uow.Tx, now time.Time, limit int) ([]moderation.RestrictionRow, error) {
	n, err := postgres.ToInt32(limit, "batch limit")
	if err != nil {
		return nil, err
	}
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.DeleteExpiredDomainRestrictions(ctx, moderationgen.DeleteExpiredDomainRestrictionsParams{Now: now, BatchLimit: n})
	if err != nil {
		return nil, postgres.Wrap(err, "delete expired domain restrictions")
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
	return postgres.Wrap(q.InsertDomainRestriction(ctx, moderationgen.InsertDomainRestrictionParams{
		Domain: row.Target, Restriction: string(row.Restriction), SourceActionID: row.SourceActionID,
		EffectiveUntil: row.EffectiveUntil,
	}), "insert domain restriction")
}

// DeleteDomainRestrictionsOf implements moderation.Repository.
func (r *Repository) DeleteDomainRestrictionsOf(ctx context.Context, tx uow.Tx, actionID string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.DeleteDomainRestrictionsOfAction(ctx, actionID), "delete domain restrictions")
}

// DomainRestrictionRows implements moderation.Repository.
func (r *Repository) DomainRestrictionRows(ctx context.Context, tx uow.Tx, domain string) ([]moderation.RestrictionRow, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDomainRestrictionRows(ctx, domain)
	if err != nil {
		return nil, postgres.Wrap(err, "list domain restrictions")
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
	return postgres.Wrap(q.UpsertReportState(ctx, moderationgen.UpsertReportStateParams{
		ReportID: s.ReportID, State: string(s.State), Outcome: postgres.ToNullString(string(s.Outcome)),
		AssigneeUserID: postgres.ToNullString(s.AssigneeUserID), Since: s.Since,
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
		AccountID: accountID, Now: now,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list account restrictions in effect")
	}
	return restrictionsFrom(rows), nil
}

// DomainRestrictionsInEffect implements moderation.Repository.
func (r *Repository) DomainRestrictionsInEffect(ctx context.Context, domains []string, now time.Time) ([]moderation.Restriction, error) {
	rows, err := r.db.reader().ListDomainRestrictionsInEffect(ctx, moderationgen.ListDomainRestrictionsInEffectParams{
		Domains: domains, Now: now,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list domain restrictions in effect")
	}
	return restrictionsFrom(rows), nil
}

func (r *Repository) reportFrom(ctx context.Context, row moderationgen.Report) (moderation.Report, error) {
	ids, err := r.db.reader().ListReportStatusIDs(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, postgres.Wrap(err, "list report statuses")
	}
	rules, err := r.db.reader().ListReportRuleIDs(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, postgres.Wrap(err, "list report rule citations")
	}
	rooms, err := r.db.reader().GetReportVoiceRoom(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, postgres.Wrap(err, "read report voice room")
	}
	var room *moderation.ReportVoiceRoom
	if len(rooms) > 0 {
		room = &moderation.ReportVoiceRoom{RoomID: rooms[0].RoomID, RevisionID: rooms[0].RevisionID}
	}
	origins, err := r.db.reader().GetReportHookOrigin(ctx, row.ID)
	if err != nil {
		return moderation.Report{}, postgres.Wrap(err, "read report hook origin")
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
		return moderation.Report{}, postgres.NotFound(err, appmoderation.ErrNotFound, "report "+id)
	}
	return r.reportFrom(ctx, row)
}

// FindReportByURI implements moderation.Repository.
func (r *Repository) FindReportByURI(ctx context.Context, uri string) (moderation.Report, error) {
	row, err := r.db.reader().GetReportByURI(ctx, postgres.ToNullString(uri))
	if err != nil {
		return moderation.Report{}, postgres.NotFound(err, appmoderation.ErrNotFound, "report "+uri)
	}
	return r.reportFrom(ctx, row)
}

// FindReportStatus implements moderation.Repository.
func (r *Repository) FindReportStatus(ctx context.Context, reportID string) (moderation.ReportStatus, error) {
	row, err := r.db.reader().GetReportState(ctx, reportID)
	if err != nil {
		return moderation.ReportStatus{}, postgres.NotFound(err, appmoderation.ErrNotFound, "report state "+reportID)
	}
	return reportStatusFrom(row), nil
}

func reportStatusFrom(row moderationgen.ReportState) moderation.ReportStatus {
	return moderation.ReportStatus{
		ReportID: row.ReportID, State: moderation.ReportState(row.State), Outcome: moderation.ReportOutcome(postgres.FromNullString(row.Outcome)),
		AssigneeUserID: postgres.FromNullString(row.AssigneeUserID), Since: postgres.UTC(row.Since),
	}
}

// ListReportsOf implements moderation.Repository.
func (r *Repository) ListReportsOf(ctx context.Context, targetAccountID string) ([]moderation.Report, error) {
	rows, err := r.db.reader().ListReportsOfTarget(ctx, targetAccountID)
	if err != nil {
		return nil, postgres.Wrap(err, "list reports")
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
	return postgres.Wrap(q.InsertReportAssignment(ctx, moderationgen.InsertReportAssignmentParams{
		ID: a.ID, ReportID: a.ReportID, AssigneeUserID: postgres.ToNullString(a.AssigneeUserID),
		AssignedByUserID: postgres.ToNullString(a.AssignedByUserID), AssignedAt: a.AssignedAt,
	}), "insert report assignment")
}

// InsertReportResolution implements moderation.Repository.
func (r *Repository) InsertReportResolution(ctx context.Context, tx uow.Tx, res moderation.ReportResolution) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return postgres.Wrap(q.InsertReportResolution(ctx, moderationgen.InsertReportResolutionParams{
		ID: res.ID, ReportID: res.ReportID, Outcome: string(res.Outcome),
		ResolvedByUserID: postgres.ToNullString(res.ResolvedByUserID), ResolvedAt: res.ResolvedAt,
	}), "insert report resolution")
}

// ReportStatusForUpdate implements moderation.Repository; it locks the row until the
// transaction ends.
func (r *Repository) ReportStatusForUpdate(ctx context.Context, tx uow.Tx, reportID string) (moderation.ReportStatus, error) {
	q, err := r.db.queries(tx)
	if err != nil {
		return moderation.ReportStatus{}, err
	}
	row, err := q.GetReportStateForUpdate(ctx, reportID)
	if err != nil {
		return moderation.ReportStatus{}, postgres.NotFound(err, appmoderation.ErrNotFound, "report state "+reportID)
	}
	return reportStatusFrom(row), nil
}

// ListReports implements moderation.Repository.
func (r *Repository) ListReports(ctx context.Context, q appmoderation.ReportQuery, limit int) ([]moderation.ReportRecord, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	var rows []moderationgen.ListReportsRow
	if q.OldestFirst {
		var oldest []moderationgen.ListReportsOldestFirstRow
		oldest, err = r.db.reader().ListReportsOldestFirst(ctx, moderationgen.ListReportsOldestFirstParams{
			State: string(q.State), Category: string(q.Category), Assignee: postgres.ToNullString(q.AssigneeUserID), Unassigned: q.Unassigned,
			After: postgres.ToNullString(q.After), Lim: lim,
		})
		for _, row := range oldest {
			rows = append(rows, moderationgen.ListReportsRow(row))
		}
	} else {
		rows, err = r.db.reader().ListReports(ctx, moderationgen.ListReportsParams{
			State: string(q.State), Category: string(q.Category), Assignee: postgres.ToNullString(q.AssigneeUserID), Unassigned: q.Unassigned,
			Before: postgres.ToNullString(q.After), Lim: lim,
		})
	}
	if err != nil {
		return nil, postgres.Wrap(err, "list reports")
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
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListReportsByReporter(ctx, moderationgen.ListReportsByReporterParams{
		ReporterAccountID: reporterAccountID, Before: postgres.ToNullString(before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list reports by reporter")
	}
	records := make([]moderationgen.ListReportsRow, len(rows))
	for i, row := range rows {
		records[i] = moderationgen.ListReportsRow(row)
	}
	return r.reportRecordsFrom(ctx, records)
}

func domainActionRecordFrom(row moderationgen.ListDomainModerationActionsRow) moderation.DomainActionRecord {
	rec := moderation.DomainActionRecord{Action: moderation.DomainAction{
		ID: row.ID, Domain: row.Domain, Kind: moderation.DomainActionKind(row.Kind), PublicComment: postgres.FromNullString(row.PublicComment),
		PrivateNote: postgres.FromNullString(row.PrivateNote), Obfuscate: row.Obfuscate, IssuedByUserID: postgres.FromNullString(row.IssuedByUserID),
		ExpiresAt: postgres.UTCPtr(row.ExpiresAt), IssuedAt: postgres.UTC(row.IssuedAt),
	}}
	if row.LiftActionID != nil && row.LiftedAt != nil {
		rec.Lift = &moderation.DomainLift{ActionID: row.ID, LiftedByUserID: postgres.FromNullString(row.LiftedByUserID), LiftedAt: postgres.UTC(*row.LiftedAt)}
	}
	return rec
}

// FindDomainActionRecord implements moderation.Repository.
func (r *Repository) FindDomainActionRecord(ctx context.Context, id string) (moderation.DomainActionRecord, error) {
	row, err := r.db.reader().GetDomainModerationActionRecord(ctx, id)
	if err != nil {
		return moderation.DomainActionRecord{}, postgres.NotFound(err, appmoderation.ErrNotFound, "domain moderation action "+id)
	}
	return domainActionRecordFrom(moderationgen.ListDomainModerationActionsRow(row)), nil
}

// ListDomainActions implements moderation.Repository.
func (r *Repository) ListDomainActions(ctx context.Context, q appmoderation.DomainActionQuery, limit int) ([]moderation.DomainActionRecord, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListDomainModerationActions(ctx, moderationgen.ListDomainModerationActionsParams{
		Domain: q.Domain, Kinds: domainActionKindStrings(q.Kinds), InEffectAt: q.InEffectAt, Before: postgres.ToNullString(q.Before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list domain moderation actions")
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
		ReasonCategory: moderation.Category(row.ReasonCategory), PublicReason: postgres.FromNullString(row.PublicReason),
		PrivateNote: postgres.FromNullString(row.PrivateNote), ReportID: postgres.FromNullString(row.ReportID), Issuer: moderation.Issuer(row.Issuer),
		IssuedByUserID: postgres.FromNullString(row.IssuedByUserID), NotifyTarget: row.NotifyTarget,
		ExpiresAt: postgres.UTCPtr(row.ExpiresAt), IssuedAt: postgres.UTC(row.IssuedAt),
	}}
	if row.LiftActionID != nil && row.LiftedAt != nil {
		rec.Lift = &moderation.Lift{
			ActionID: row.ID, LiftedByUserID: postgres.FromNullString(row.LiftedByUserID), Reason: postgres.FromNullString(row.LiftReason),
			LiftedAt: postgres.UTC(*row.LiftedAt),
		}
	}
	return rec
}

// FindActionRecord implements moderation.Repository.
func (r *Repository) FindActionRecord(ctx context.Context, id string) (moderation.ActionRecord, error) {
	row, err := r.db.reader().GetModerationActionRecord(ctx, id)
	if err != nil {
		return moderation.ActionRecord{}, postgres.NotFound(err, appmoderation.ErrNotFound, "moderation action "+id)
	}
	rec := actionRecordFrom(moderationgen.ListModerationActionsRow(row))
	if rec.Action.StatusIDs, err = actionStatusIDs(ctx, r.db.reader(), id); err != nil {
		return moderation.ActionRecord{}, err
	}
	return rec, nil
}

// ListActions implements moderation.Repository.
func (r *Repository) ListActions(ctx context.Context, q appmoderation.ActionQuery, limit int) ([]moderation.ActionRecord, error) {
	lim, err := postgres.ToInt32(limit, "limit")
	if err != nil {
		return nil, err
	}
	rows, err := r.db.reader().ListModerationActions(ctx, moderationgen.ListModerationActionsParams{
		AccountID: postgres.ToNullString(q.AccountID), InEffectAt: q.InEffectAt, Before: postgres.ToNullString(q.Before), Lim: lim,
	})
	if err != nil {
		return nil, postgres.Wrap(err, "list moderation actions")
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	list := ids
	statuses, err := r.db.reader().ListModerationActionStatusIDsOf(ctx, list)
	if err != nil {
		return nil, postgres.Wrap(err, "list moderation action statuses")
	}
	statusesOf := map[string][]string{}
	for _, st := range statuses {
		statusesOf[st.ActionID] = append(statusesOf[st.ActionID], st.StatusID)
	}
	out := make([]moderation.ActionRecord, len(rows))
	for i, row := range rows {
		out[i] = actionRecordFrom(row)
		out[i].Action.StatusIDs = statusesOf[row.ID]
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
		ID: row.ID, URI: postgres.FromNullString(row.Uri), ReporterAccountID: row.ReporterAccountID, TargetAccountID: row.TargetAccountID,
		Category: moderation.Category(row.Category), Comment: row.Comment, Forwarded: row.Forwarded,
		FiledAt: postgres.UTC(row.FiledAt), StatusIDs: ids, RuleIDs: rules, VoiceRoom: room, HookOrigin: origin, Forward: forward,
	}
}
