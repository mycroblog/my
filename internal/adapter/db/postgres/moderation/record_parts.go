// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// FindActionRecords implements moderation.Repository.
func (r *Repository) FindActionRecords(ctx context.Context, ids []string) (map[string]moderation.ActionRecord, error) {
	out := map[string]moderation.ActionRecord{}
	if len(ids) == 0 {
		return out, nil
	}
	q := r.db.reader()
	rows, err := q.ListModerationActionRecordsByIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list moderation actions")
	}
	statuses, err := q.ListModerationActionStatusIDsOf(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list moderation action statuses")
	}
	statusesOf := map[string][]string{}
	for _, st := range statuses {
		statusesOf[st.ActionID] = append(statusesOf[st.ActionID], st.StatusID)
	}
	for _, row := range rows {
		rec := actionRecordFrom(moderationgen.ListModerationActionsRow(row))
		rec.Action.StatusIDs = statusesOf[row.ID]
		out[row.ID] = rec
	}
	return out, nil
}

// FindAppeals implements moderation.Repository.
func (r *Repository) FindAppeals(ctx context.Context, ids []string) (map[string]moderation.AppealRecord, error) {
	out := map[string]moderation.AppealRecord{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListModerationAppealsByIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list moderation appeals")
	}
	for _, row := range rows {
		out[row.ID] = appealRecord(moderationgen.GetModerationAppealRow(row))
	}
	return out, nil
}

// FindSeverances implements moderation.Repository.
func (r *Repository) FindSeverances(ctx context.Context, accountID string, ids []string) (map[string]moderation.SeveranceRecord, error) {
	out := map[string]moderation.SeveranceRecord{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListRelationshipSeverancesOf(ctx, moderationgen.ListRelationshipSeverancesOfParams{AccountID: accountID, Ids: ids})
	if err != nil {
		return nil, postgres.Wrap(err, "list relationship severances")
	}
	for _, row := range rows {
		out[row.ID] = severanceFrom(moderationgen.GetRelationshipSeveranceRow(row))
	}
	return out, nil
}

// FindReports implements moderation.Repository: the reports and each of their parts with one query,
// built as reportFrom builds one.
func (r *Repository) FindReports(ctx context.Context, ids []string) (map[string]moderation.Report, error) {
	out := map[string]moderation.Report{}
	if len(ids) == 0 {
		return out, nil
	}
	q := r.db.reader()
	rows, err := q.ListReportsByIDs(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list reports")
	}
	return r.reportsWithParts(ctx, rows)
}

// reportsWithParts returns the reports of rows with their parts, by ID, reading each part of all
// of them with one query, as reportFrom reads those of one.
func (r *Repository) reportsWithParts(ctx context.Context, rows []moderationgen.Report) (map[string]moderation.Report, error) {
	out := make(map[string]moderation.Report, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	q := r.db.reader()
	statusRows, err := q.ListReportStatusIDsOf(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list report statuses")
	}
	statusesOf := map[string][]string{}
	for _, st := range statusRows {
		statusesOf[st.ReportID] = append(statusesOf[st.ReportID], st.StatusID)
	}
	ruleRows, err := q.ListReportRuleIDsOf(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "list report rule citations")
	}
	rulesOf := map[string][]string{}
	for _, c := range ruleRows {
		rulesOf[c.ReportID] = append(rulesOf[c.ReportID], c.RuleID)
	}
	roomRows, err := q.ListReportVoiceRoomsOf(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "read report voice rooms")
	}
	roomOf := map[string]*moderation.ReportVoiceRoom{}
	for _, v := range roomRows {
		if _, ok := roomOf[v.ReportID]; !ok {
			roomOf[v.ReportID] = &moderation.ReportVoiceRoom{RoomID: v.RoomID, RevisionID: v.RevisionID}
		}
	}
	originRows, err := q.ListReportHookOriginsOf(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "read report hook origins")
	}
	originOf := map[string]*moderation.ReportHookOrigin{}
	for _, o := range originRows {
		if _, ok := originOf[o.ReportID]; !ok {
			originOf[o.ReportID] = &moderation.ReportHookOrigin{DecisionID: o.HookDecisionID, Hook: o.Hook, ReasonCode: o.ReasonCode}
		}
	}
	forwardRows, err := q.ListReportForwardsOf(ctx, ids)
	if err != nil {
		return nil, postgres.Wrap(err, "read report forwards")
	}
	forwardOf := map[string]*moderation.ReportForward{}
	for _, f := range forwardRows {
		if _, ok := forwardOf[f.ReportID]; !ok {
			forwardOf[f.ReportID] = reportForwardFrom(f)
		}
	}
	for _, row := range rows {
		// A report without statuses or rules has empty lists, as the queries of one return them.
		out[row.ID] = reportOf(row, append([]string{}, statusesOf[row.ID]...), append([]string{}, rulesOf[row.ID]...), roomOf[row.ID], originOf[row.ID], forwardOf[row.ID])
	}
	return out, nil
}

// FindReportStatuses implements moderation.Repository.
func (r *Repository) FindReportStatuses(ctx context.Context, reportIDs []string) (map[string]moderation.ReportStatus, error) {
	out := map[string]moderation.ReportStatus{}
	if len(reportIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.reader().ListReportStatesOf(ctx, reportIDs)
	if err != nil {
		return nil, postgres.Wrap(err, "list report states")
	}
	for _, row := range rows {
		out[row.ReportID] = reportStatusFrom(row)
	}
	return out, nil
}
