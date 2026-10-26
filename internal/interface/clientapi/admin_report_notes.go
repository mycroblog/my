// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The detail of a report (FR-710): its moderators' notes and the history of its target, and the
// operations that write a note and reopen a resolved report. Only the admin API (manage_reports)
// presents the notes; the reporter's and the target's endpoints never do.

// reportDetailResponse presents a report with its notes and the history of its target.
func (s *Server) reportDetailResponse(ctx context.Context, d moderation.ReportDetail) (AdminReportDetailJSONResponse, error) {
	rs, err := s.newAdminReports(ctx, []moderation.ReportRecord{d.ReportRecord})
	if err != nil {
		return AdminReportDetailJSONResponse{}, err
	}
	r := rs[0]
	body := AdminReportDetail{
		AssigneeAccountId: r.AssigneeAccountId, Category: AdminReportDetailCategory(r.Category), Comment: r.Comment,
		FiledAt: r.FiledAt, Forwarded: r.Forwarded, Id: r.Id, Outcome: r.Outcome, ReporterAccount: r.ReporterAccount,
		ReporterAccountId: r.ReporterAccountId, RuleIds: r.RuleIds, State: r.State, StateChangedAt: r.StateChangedAt,
		StatusIds: r.StatusIds, TargetAccount: r.TargetAccount, TargetAccountId: r.TargetAccountId, Uri: r.Uri,
		Notes:         make([]AdminReportNote, 0, len(d.Notes)),
		TargetHistory: AdminReportTargetHistory{ReportsCount: d.History.Reports, ActionsCount: d.History.Actions},
	}
	if body.Statuses, err = s.reportStatuses(ctx, r.Id); err != nil {
		return AdminReportDetailJSONResponse{}, err
	}
	body.VoiceRoomId = r.VoiceRoomId
	if body.VoiceRoom, err = s.reportVoiceRoom(ctx, d.Report); err != nil {
		return AdminReportDetailJSONResponse{}, err
	}
	authors := map[string]string{}
	for _, n := range d.Notes {
		note := AdminReportNote{Id: n.ID, Body: n.Body, CreatedAt: n.WrittenAt.UTC()}
		if u := n.AuthorUserID; u != "" && s.o.UserAccounts != nil {
			id, ok := authors[u]
			if !ok {
				la, err := s.o.UserAccounts.LocalAccountOfUser(ctx, u)
				if err != nil && !errors.Is(err, appaccount.ErrNotFound) {
					return AdminReportDetailJSONResponse{}, fmt.Errorf("clientapi: the account of user %s: %w", u, err)
				}
				id = la.AccountID
				authors[u] = id
			}
			if id != "" {
				note.AuthorAccountId = &id
			}
		}
		body.Notes = append(body.Notes, note)
	}
	cache := cacheNoStore
	return AdminReportDetailJSONResponse{Body: body, Headers: AdminReportDetailResponseHeaders{CacheControl: &cache}}, nil
}

// reportStatuses presents the statuses that a report points at, whatever their visibility and
// deleted ones included, for the moderators (FR-710, NFR-MOD-4). The moderation module reads
// only the report's statuses; the caller has checked the manage_reports permission.
func (s *Server) reportStatuses(ctx context.Context, reportID string) ([]AdminReportStatus, error) {
	reviewed, err := s.o.Moderation.ReportStatuses(ctx, reportID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the statuses of report %s: %w", reportID, err)
	}
	views := make([]appcontent.StatusView, 0, len(reviewed))
	for _, r := range reviewed {
		views = append(views, r.StatusView)
	}
	sts, err := s.presentStatuses(ctx, views)
	if err != nil {
		return nil, err
	}
	out := make([]AdminReportStatus, 0, len(reviewed))
	for i, r := range reviewed {
		item := AdminReportStatus{Status: sts[i]}
		if !r.DeletedAt.IsZero() {
			at := r.DeletedAt.UTC()
			item.DeletedAt = &at
		}
		out = append(out, item)
	}
	return out, nil
}

// AddReportNote serves POST /api/v1/admin/reports/{id}/notes.
func (s *Server) AddReportNote(ctx context.Context, req AddReportNoteRequestObject) (AddReportNoteResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return AddReportNotedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return AddReportNotedefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	_, err = s.o.Moderation.AddReportNote(ctx, req.Id, req.Body.Body, v.UserID)
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		return AddReportNotedefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, moderation.ErrInvalidReportNote):
		l := s.Localizer(ctx)
		return AddReportNotedefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("body"))), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: add a note to report %s: %w", req.Id, err)
	}
	d, err := s.o.Moderation.ReportDetail(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: report %s: %w", req.Id, err)
	}
	res, err := s.reportDetailResponse(ctx, d)
	if err != nil {
		return nil, err
	}
	return AddReportNote200JSONResponse{res}, nil
}

// ReopenReport serves POST /api/v1/admin/reports/{id}/reopen.
func (s *Server) ReopenReport(ctx context.Context, req ReopenReportRequestObject) (ReopenReportResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return ReopenReportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	rec, err := s.o.Moderation.ReopenReport(ctx, req.Id, v.UserID)
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		return ReopenReportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appmoderation.ErrReportNotResolved):
		l := s.Localizer(ctx)
		return ReopenReportdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), l.ServerErrorsReportNotResolved())), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: reopen report %s: %w", req.Id, err)
	}
	res, err := s.reportResponse(ctx, rec)
	if err != nil {
		return nil, err
	}
	return ReopenReport200JSONResponse{res}, nil
}
