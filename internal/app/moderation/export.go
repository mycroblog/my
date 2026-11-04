// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The disclosure of an account's moderation records (ADR-0058): an operator answers a request
// of access (GDPR art. 15, APPI art. 33) with every record about the account, the private notes
// included, and only the reporters' identities withheld: a report about the account names
// neither its reporter nor the server it came from. The records are what the retention has kept;
// a hold (retention.go) keeps them while the request is handled.

// exportPage is the number of rows each list of the export reads per page.
const exportPage = 100

// RecordExport is every moderation record about one account, as `my admin account
// export-moderation` writes it in JSON. Times are RFC 3339 in UTC.
type RecordExport struct {
	AccountID  string    `json:"account_id"`
	ExportedAt time.Time `json:"exported_at"`
	// Actions are the actions on the account, newest first.
	Actions []ExportedAction `json:"actions"`
	// ReportsAbout are the reports of the account, oldest first, without their reporters.
	ReportsAbout []ExportedReport `json:"reports_about_account"`
	// ReportsFiled are the reports that the account filed, newest first.
	ReportsFiled []ExportedFiledReport `json:"reports_filed"`
	// AuditLog are the audit log entries on the account and on its reports and appeals, newest
	// first.
	AuditLog []ExportedAuditEntry `json:"audit_log"`
}

// ExportedAction is an action on the account with its lift and appeal.
type ExportedAction struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	ReasonCategory string              `json:"reason_category"`
	PublicReason   string              `json:"public_reason"`
	PrivateNote    string              `json:"private_note"`
	ReportID       string              `json:"report_id,omitempty"`
	Issuer         string              `json:"issuer"`
	IssuedByUserID string              `json:"issued_by_user_id,omitempty"`
	NotifyTarget   bool                `json:"notify_target"`
	StatusIDs      []string            `json:"status_ids"`
	IssuedAt       time.Time           `json:"issued_at"`
	ExpiresAt      *time.Time          `json:"expires_at"`
	Lift           *ExportedActionLift `json:"lift"`
	Appeal         *ExportedAppeal     `json:"appeal"`
}

// ExportedActionLift is the lift of an action.
type ExportedActionLift struct {
	LiftedByUserID string    `json:"lifted_by_user_id,omitempty"`
	Reason         string    `json:"reason"`
	LiftedAt       time.Time `json:"lifted_at"`
}

// ExportedAppeal is the account's appeal of an action with the decision on it.
type ExportedAppeal struct {
	ID          string                  `json:"id"`
	Text        string                  `json:"text"`
	SubmittedAt time.Time               `json:"submitted_at"`
	Decision    *ExportedAppealDecision `json:"decision"`
}

// ExportedAppealDecision is the decision on an appeal.
type ExportedAppealDecision struct {
	Decision        string    `json:"decision"`
	PublicReason    string    `json:"public_reason"`
	DecidedByUserID string    `json:"decided_by_user_id,omitempty"`
	DecidedAt       time.Time `json:"decided_at"`
}

// ExportedReport is a report of the account with what the moderators did with it. It has no
// reporter, URI (which names the reporter's server), or forwarding.
type ExportedReport struct {
	ID          string                     `json:"id"`
	Category    string                     `json:"category"`
	Comment     string                     `json:"comment"`
	StatusIDs   []string                   `json:"status_ids"`
	RuleIDs     []string                   `json:"rule_ids"`
	FiledAt     time.Time                  `json:"filed_at"`
	State       string                     `json:"state"`
	Outcome     string                     `json:"outcome,omitempty"`
	Assignments []ExportedReportAssignment `json:"assignments"`
	Resolutions []ExportedReportResolution `json:"resolutions"`
	Reopenings  []ExportedReportReopening  `json:"reopenings"`
	Notes       []ExportedReportNote       `json:"notes"`
}

// ExportedReportAssignment is an assignment of a report; an empty assignee gave it up.
type ExportedReportAssignment struct {
	AssigneeUserID   string    `json:"assignee_user_id,omitempty"`
	AssignedByUserID string    `json:"assigned_by_user_id,omitempty"`
	AssignedAt       time.Time `json:"assigned_at"`
}

// ExportedReportResolution is a resolution of a report.
type ExportedReportResolution struct {
	Outcome          string    `json:"outcome"`
	ResolvedByUserID string    `json:"resolved_by_user_id,omitempty"`
	ResolvedAt       time.Time `json:"resolved_at"`
}

// ExportedReportReopening is a reopening of a report.
type ExportedReportReopening struct {
	ReopenedByUserID string    `json:"reopened_by_user_id,omitempty"`
	ReopenedAt       time.Time `json:"reopened_at"`
}

// ExportedReportNote is a moderator's note on a report.
type ExportedReportNote struct {
	AuthorUserID string    `json:"author_user_id,omitempty"`
	Body         string    `json:"body"`
	WrittenAt    time.Time `json:"written_at"`
}

// ExportedFiledReport is a report that the account filed, as its reporter sees it (FR-711):
// whether it is open or resolved, not how.
type ExportedFiledReport struct {
	ID              string    `json:"id"`
	TargetAccountID string    `json:"target_account_id"`
	Category        string    `json:"category"`
	Comment         string    `json:"comment"`
	StatusIDs       []string  `json:"status_ids"`
	RuleIDs         []string  `json:"rule_ids"`
	Forwarded       bool      `json:"forwarded"`
	FiledAt         time.Time `json:"filed_at"`
	State           string    `json:"state"`
}

// ExportedAuditEntry is an entry of the audit log.
type ExportedAuditEntry struct {
	ID            string    `json:"id"`
	ActorKind     string    `json:"actor_kind"`
	ActorUserID   string    `json:"actor_user_id,omitempty"`
	Action        string    `json:"action"`
	TargetKind    string    `json:"target_kind"`
	TargetID      string    `json:"target_id"`
	TargetLabel   string    `json:"target_label"`
	SourceEventID string    `json:"source_event_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// ExportRecords returns every moderation record about an account for a request of access; an
// unknown account fails with ErrAccountNotFound.
func (s *Service) ExportRecords(ctx context.Context, accountID string) (RecordExport, error) {
	switch _, err := s.d.Accounts.FindByID(ctx, accountID); {
	case errors.Is(err, appaccount.ErrNotFound):
		return RecordExport{}, fmt.Errorf("%w: %s", ErrAccountNotFound, accountID)
	case err != nil:
		return RecordExport{}, err
	}
	out := RecordExport{
		AccountID: accountID, ExportedAt: kernel.Normalize(s.d.Clock.Now()).UTC(),
		Actions: []ExportedAction{}, ReportsAbout: []ExportedReport{}, ReportsFiled: []ExportedFiledReport{}, AuditLog: []ExportedAuditEntry{},
	}
	var err error
	if out.Actions, err = s.exportActions(ctx, accountID); err != nil {
		return RecordExport{}, err
	}
	reportIDs, err := s.exportReportsAbout(ctx, accountID, &out)
	if err != nil {
		return RecordExport{}, err
	}
	if out.ReportsFiled, err = s.exportReportsFiled(ctx, accountID); err != nil {
		return RecordExport{}, err
	}
	targets := []AuditQuery{{TargetKind: moderation.AuditTargetAccount, TargetID: accountID}}
	for _, id := range reportIDs {
		targets = append(targets, AuditQuery{TargetKind: moderation.AuditTargetReport, TargetID: id})
	}
	for _, a := range out.Actions {
		if a.Appeal != nil {
			targets = append(targets, AuditQuery{TargetKind: moderation.AuditTargetAppeal, TargetID: a.Appeal.ID})
		}
	}
	if out.AuditLog, err = s.exportAuditLog(ctx, targets); err != nil {
		return RecordExport{}, err
	}
	return out, nil
}

func (s *Service) exportActions(ctx context.Context, accountID string) ([]ExportedAction, error) {
	repo := s.d.Repository
	var recs []moderation.ActionRecord
	q := ActionQuery{AccountID: accountID}
	for {
		page, err := repo.ListActions(ctx, q, exportPage)
		if err != nil {
			return nil, err
		}
		recs = append(recs, page...)
		if len(page) < exportPage {
			break
		}
		q.Before = page[len(page)-1].Action.ID
	}
	ids := make([]string, len(recs))
	for i, r := range recs {
		ids[i] = r.Action.ID
	}
	appeals := map[string]moderation.AppealRecord{}
	if len(ids) > 0 {
		as, err := repo.AppealsOfActions(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, a := range as {
			appeals[a.Appeal.ActionID] = a
		}
	}
	out := make([]ExportedAction, 0, len(recs))
	for _, r := range recs {
		a := r.Action
		e := ExportedAction{
			ID: a.ID, Kind: string(a.Kind), ReasonCategory: string(a.ReasonCategory), PublicReason: a.PublicReason,
			PrivateNote: a.PrivateNote, ReportID: a.ReportID, Issuer: string(a.Issuer), IssuedByUserID: a.IssuedByUserID,
			NotifyTarget: a.NotifyTarget, StatusIDs: nonNil(a.StatusIDs), IssuedAt: a.IssuedAt.UTC(), ExpiresAt: utc(a.ExpiresAt),
		}
		if l := r.Lift; l != nil {
			e.Lift = &ExportedActionLift{LiftedByUserID: l.LiftedByUserID, Reason: l.Reason, LiftedAt: l.LiftedAt.UTC()}
		}
		if ap, ok := appeals[a.ID]; ok {
			e.Appeal = &ExportedAppeal{ID: ap.Appeal.ID, Text: ap.Appeal.Text, SubmittedAt: ap.Appeal.SubmittedAt.UTC()}
			if d := ap.Decision; d != nil {
				e.Appeal.Decision = &ExportedAppealDecision{
					Decision: string(d.Decision), PublicReason: d.PublicReason, DecidedByUserID: d.DecidedByUserID, DecidedAt: d.DecidedAt.UTC(),
				}
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// exportReportsAbout adds the reports of the account to out and returns their IDs.
func (s *Service) exportReportsAbout(ctx context.Context, accountID string, out *RecordExport) ([]string, error) {
	repo := s.d.Repository
	reports, err := repo.ListReportsOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(reports))
	for _, r := range reports {
		st, err := repo.FindReportStatus(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		h, err := repo.ReportHistory(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		notes, err := repo.ListReportNotes(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		e := ExportedReport{
			ID: r.ID, Category: string(r.Category), Comment: r.Comment, StatusIDs: nonNil(r.StatusIDs), RuleIDs: nonNil(r.RuleIDs),
			FiledAt: r.FiledAt.UTC(), State: string(st.State), Outcome: string(st.Outcome),
			Assignments: make([]ExportedReportAssignment, 0, len(h.Assignments)),
			Resolutions: make([]ExportedReportResolution, 0, len(h.Resolutions)),
			Reopenings:  make([]ExportedReportReopening, 0, len(h.Reopenings)),
			Notes:       make([]ExportedReportNote, 0, len(notes)),
		}
		for _, a := range h.Assignments {
			e.Assignments = append(e.Assignments, ExportedReportAssignment{AssigneeUserID: a.AssigneeUserID, AssignedByUserID: a.AssignedByUserID, AssignedAt: a.AssignedAt.UTC()})
		}
		for _, res := range h.Resolutions {
			e.Resolutions = append(e.Resolutions, ExportedReportResolution{Outcome: string(res.Outcome), ResolvedByUserID: res.ResolvedByUserID, ResolvedAt: res.ResolvedAt.UTC()})
		}
		for _, ro := range h.Reopenings {
			e.Reopenings = append(e.Reopenings, ExportedReportReopening{ReopenedByUserID: ro.ReopenedByUserID, ReopenedAt: ro.ReopenedAt.UTC()})
		}
		for _, n := range notes {
			e.Notes = append(e.Notes, ExportedReportNote{AuthorUserID: n.AuthorUserID, Body: n.Body, WrittenAt: n.WrittenAt.UTC()})
		}
		out.ReportsAbout = append(out.ReportsAbout, e)
		ids = append(ids, r.ID)
	}
	return ids, nil
}

func (s *Service) exportReportsFiled(ctx context.Context, accountID string) ([]ExportedFiledReport, error) {
	out := []ExportedFiledReport{}
	before := ""
	for {
		page, err := s.d.Repository.ListReportsByReporter(ctx, accountID, before, exportPage)
		if err != nil {
			return nil, err
		}
		for _, rec := range page {
			r := rec.Report
			out = append(out, ExportedFiledReport{
				ID: r.ID, TargetAccountID: r.TargetAccountID, Category: string(r.Category), Comment: r.Comment,
				StatusIDs: nonNil(r.StatusIDs), RuleIDs: nonNil(r.RuleIDs), Forwarded: r.Forwarded, FiledAt: r.FiledAt.UTC(),
				State: string(rec.Status.State),
			})
		}
		if len(page) < exportPage {
			return out, nil
		}
		before = page[len(page)-1].Report.ID
	}
}

// exportAuditLog returns the entries on each of the targets, newest first.
func (s *Service) exportAuditLog(ctx context.Context, targets []AuditQuery) ([]ExportedAuditEntry, error) {
	var entries []moderation.AuditEntry
	for _, q := range targets {
		for {
			page, err := s.d.Repository.ListAuditEntries(ctx, q, exportPage)
			if err != nil {
				return nil, err
			}
			entries = append(entries, page...)
			if len(page) < exportPage {
				break
			}
			q.Before = page[len(page)-1].ID
		}
	}
	// IDs are UUIDv7: the greater, the newer.
	slices.SortFunc(entries, func(a, b moderation.AuditEntry) int { return strings.Compare(b.ID, a.ID) })
	out := make([]ExportedAuditEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, ExportedAuditEntry{
			ID: e.ID, ActorKind: string(e.ActorKind), ActorUserID: e.ActorUserID, Action: string(e.Action),
			TargetKind: string(e.TargetKind), TargetID: e.TargetID, TargetLabel: e.TargetLabel, SourceEventID: e.SourceEventID,
			OccurredAt: e.OccurredAt.UTC(),
		})
	}
	return out, nil
}

func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
