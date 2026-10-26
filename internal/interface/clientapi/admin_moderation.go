// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
)

// The admin operations of moderation: the actions on domains (FR-702, manage_domains) and the
// reports (FR-701, manage_reports). The moderation module records them; issuing or lifting a
// domain action changes the restrictions that federation and the timelines read at once.

// Moderation runs the moderators' use cases and lists the public domain actions.
// *appmoderation.Service implements it.
type Moderation interface {
	DomainActions(ctx context.Context, f appmoderation.DomainActionFilter, limit int) ([]appmoderation.DomainActionView, error)
	DomainAction(ctx context.Context, id string) (appmoderation.DomainActionView, error)
	IssueDomainAction(ctx context.Context, in appmoderation.DomainActionInput) (moderation.DomainAction, error)
	LiftDomainAction(ctx context.Context, actionID, liftedByUserID string) error
	Reports(ctx context.Context, q appmoderation.ReportQuery, limit int) ([]moderation.ReportRecord, error)
	ReportRecord(ctx context.Context, id string) (moderation.ReportRecord, error)
	AssignReport(ctx context.Context, reportID, assigneeUserID, byUserID string) (moderation.ReportRecord, error)
	ResolveReport(ctx context.Context, reportID string, outcome moderation.ReportOutcome, byUserID string) (moderation.ReportRecord, error)
	// ReportDetail, AddReportNote, and ReopenReport serve the report's detail, its notes, and
	// its reopening (admin_report_notes.go).
	ReportDetail(ctx context.Context, id string) (moderation.ReportDetail, error)
	AddReportNote(ctx context.Context, reportID, body, byUserID string) (moderation.ReportNote, error)
	ReopenReport(ctx context.Context, reportID, byUserID string) (moderation.ReportRecord, error)
	// ReportStatuses presents the statuses of a report whatever their visibility, deleted ones
	// included, for the report's detail (admin_report_notes.go).
	ReportStatuses(ctx context.Context, reportID string) ([]appcontent.ReviewedStatus, error)
	// PublicDomainActions lists the domain actions that the instance lists publicly
	// (listInstanceDomainActions, instance_domain_actions.go).
	PublicDomainActions(ctx context.Context, before string, limit int) ([]moderation.PublicDomainAction, error)
	// ExportDomainBlocks, PreviewDomainBlocks, and ImportDomainBlocks serve the domain block
	// list in Mastodon's CSV (admin_domain_blocks.go).
	ExportDomainBlocks(ctx context.Context, w io.Writer) error
	PreviewDomainBlocks(ctx context.Context, text string) ([]appmoderation.DomainBlockPreviewRow, error)
	ImportDomainBlocks(ctx context.Context, in appmoderation.DomainBlockImportInput) (appmoderation.DomainBlockImportResult, error)
}

// UserAccounts finds the local account of a user, to present the moderator of a report by
// account. *appaccount.Service implements it.
type UserAccounts interface {
	LocalAccountOfUser(ctx context.Context, userID string) (account.LocalAccount, error)
}

func newAdminDomainAction(v appmoderation.DomainActionView) AdminDomainAction {
	a := v.Action
	out := AdminDomainAction{
		Id: a.ID, Domain: a.Domain, Kind: AdminDomainActionKind(a.Kind), Obfuscate: a.Obfuscate,
		State: AdminDomainActionState(v.State), IssuedAt: a.IssuedAt.UTC(),
	}
	if a.PublicComment != "" {
		out.PublicComment = &a.PublicComment
	}
	if a.PrivateNote != "" {
		out.PrivateNote = &a.PrivateNote
	}
	if a.ExpiresAt != nil {
		t := a.ExpiresAt.UTC()
		out.ExpiresAt = &t
	}
	if v.Lift != nil {
		t := v.Lift.LiftedAt.UTC()
		out.LiftedAt = &t
	}
	return out
}

// domainActionResponse answers an operation on one domain action.
func domainActionResponse(v appmoderation.DomainActionView) AdminDomainActionJSONResponse {
	cache := cacheNoStore
	return AdminDomainActionJSONResponse{Body: newAdminDomainAction(v), Headers: AdminDomainActionResponseHeaders{CacheControl: &cache}}
}

// ListDomainActions serves GET /api/v1/admin/domain_actions, newest first. The cursor is the ID
// of the last action of the previous page.
func (s *Server) ListDomainActions(ctx context.Context, req ListDomainActionsRequestObject) (ListDomainActionsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListDomainActionsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListDomainActionsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	f := appmoderation.DomainActionFilter{Before: q.before, Active: req.Params.Active != nil && *req.Params.Active}
	if req.Params.Domain != nil {
		d, err := appfederation.CanonicalHost(*req.Params.Domain)
		if err != nil {
			return ListDomainActionsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "domain")), nil
		}
		f.Domain = d
	}
	vs, err := s.o.Moderation.DomainActions(ctx, f, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list domain actions: %w", err)
	}
	vs, last := trim(vs, q.limit)
	page := AdminDomainActionPage{Items: make([]AdminDomainAction, 0, len(vs))}
	for _, v := range vs {
		page.Items = append(page.Items, newAdminDomainAction(v))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(vs[last].Action.ID)
	}
	cache := cacheNoStore
	return ListDomainActions200JSONResponse{Body: page, Headers: ListDomainActions200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateDomainAction serves POST /api/v1/admin/domain_actions.
func (s *Server) CreateDomainAction(ctx context.Context, req CreateDomainActionRequestObject) (CreateDomainActionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return CreateDomainActiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (CreateDomainActionResponseObject, error) {
		return CreateDomainActiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	b := req.Body
	if b == nil {
		return CreateDomainActiondefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "domain")), nil
	}
	in := appmoderation.DomainActionInput{Kind: moderation.DomainActionKind(b.Kind), IssuedByUserID: v.UserID}
	if !in.Kind.Valid() {
		return unprocessable(l.ServerErrorsInvalidValue("kind"))
	}
	if in.Domain, err = appfederation.CanonicalHost(b.Domain); err != nil || !moderation.ValidDomain(in.Domain) {
		return unprocessable(l.ServerErrorsInvalidValue("domain"))
	}
	if b.PublicComment != nil {
		in.PublicComment = *b.PublicComment
	}
	if b.PrivateNote != nil {
		in.PrivateNote = *b.PrivateNote
	}
	in.Obfuscate = b.Obfuscate != nil && *b.Obfuscate
	if b.ExpiresIn != nil {
		if *b.ExpiresIn < 1 {
			return unprocessable(l.ServerErrorsInvalidValue("expires_in"))
		}
		in.Duration = time.Duration(*b.ExpiresIn) * time.Second
	}
	a, err := s.o.Moderation.IssueDomainAction(ctx, in)
	var textErr *moderation.TextError
	switch {
	case errors.As(err, &textErr) && textErr.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(textErr.Max, textErr.Field))
	case errors.As(err, &textErr):
		return unprocessable(l.ServerErrorsInvalidValue(textErr.Field))
	case err != nil:
		return nil, fmt.Errorf("clientapi: issue a domain action on %s: %w", in.Domain, err)
	}
	view, err := s.o.Moderation.DomainAction(ctx, a.ID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: domain action %s: %w", a.ID, err)
	}
	return CreateDomainAction200JSONResponse{domainActionResponse(view)}, nil
}

// GetDomainAction serves GET /api/v1/admin/domain_actions/{id}.
func (s *Server) GetDomainAction(ctx context.Context, req GetDomainActionRequestObject) (GetDomainActionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return GetDomainActiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	view, err := s.o.Moderation.DomainAction(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return GetDomainActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: domain action %s: %w", req.Id, err)
	}
	return GetDomainAction200JSONResponse{domainActionResponse(view)}, nil
}

// LiftDomainAction serves POST /api/v1/admin/domain_actions/{id}/lift.
func (s *Server) LiftDomainAction(ctx context.Context, req LiftDomainActionRequestObject) (LiftDomainActionResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return LiftDomainActiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.Moderation.LiftDomainAction(ctx, req.Id, v.UserID)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return LiftDomainActiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: lift domain action %s: %w", req.Id, err)
	}
	view, err := s.o.Moderation.DomainAction(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: domain action %s: %w", req.Id, err)
	}
	return LiftDomainAction200JSONResponse{domainActionResponse(view)}, nil
}

// newAdminReports presents reports with their reporters, targets, and assignees, reading each
// account once. An account that is gone is left out of the report.
func (s *Server) newAdminReports(ctx context.Context, recs []moderation.ReportRecord) ([]AdminReport, error) {
	return s.newAdminReportsWith(ctx, s.reads(), recs)
}

// newAdminReportsWith is newAdminReports through reads.
func (s *Server) newAdminReportsWith(ctx context.Context, reads presentReads, recs []moderation.ReportRecord) ([]AdminReport, error) {
	actors := map[string]appaccount.Actor{}
	var order []appaccount.Actor
	assignees := map[string]string{}
	for _, rec := range recs {
		for _, id := range []string{rec.Report.ReporterAccountID, rec.Report.TargetAccountID} {
			if _, ok := actors[id]; ok {
				continue
			}
			a, err := reads.FindByID(ctx, id)
			if errors.Is(err, appaccount.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("clientapi: account %s of a report: %w", id, err)
			}
			actors[id] = a
			order = append(order, a)
		}
		if u := rec.Status.AssigneeUserID; u != "" {
			if _, ok := assignees[u]; ok {
				continue
			}
			la, err := reads.LocalAccountOfUser(ctx, u)
			if err != nil && !errors.Is(err, appaccount.ErrNotFound) {
				return nil, fmt.Errorf("clientapi: the account of user %s: %w", u, err)
			}
			assignees[u] = la.AccountID
		}
	}
	presented, err := s.newAccountsWith(ctx, reads, order)
	if err != nil {
		return nil, err
	}
	accounts := make(map[string]*Account, len(presented))
	for i := range presented {
		accounts[presented[i].Id] = &presented[i]
	}
	out := make([]AdminReport, 0, len(recs))
	for _, rec := range recs {
		r, st := rec.Report, rec.Status
		item := AdminReport{
			Id: r.ID, Category: AdminReportCategory(r.Category), Comment: r.Comment, Forwarded: r.Forwarded,
			FiledAt: r.FiledAt.UTC(), State: AdminReportState(st.State), StateChangedAt: st.Since.UTC(),
			ReporterAccountId: r.ReporterAccountID, ReporterAccount: accounts[r.ReporterAccountID],
			TargetAccountId: r.TargetAccountID, TargetAccount: accounts[r.TargetAccountID],
			StatusIds: append([]string{}, r.StatusIDs...), RuleIds: append([]string{}, r.RuleIDs...),
		}
		if r.URI != "" {
			item.Uri = &r.URI
		}
		if r.VoiceRoom != nil {
			id := r.VoiceRoom.RoomID
			item.VoiceRoomId = &id
		}
		if st.Outcome != "" {
			o := AdminReportOutcome(st.Outcome)
			item.Outcome = &o
		}
		if id := assignees[st.AssigneeUserID]; id != "" {
			item.AssigneeAccountId = &id
		}
		out = append(out, item)
	}
	return out, nil
}

// reportResponse presents one report with its notes and the history of its target, read after
// the operation that changed rec.
func (s *Server) reportResponse(ctx context.Context, rec moderation.ReportRecord) (AdminReportDetailJSONResponse, error) {
	d, err := s.o.Moderation.ReportDetail(ctx, rec.Report.ID)
	if err != nil {
		return AdminReportDetailJSONResponse{}, fmt.Errorf("clientapi: report %s: %w", rec.Report.ID, err)
	}
	return s.reportDetailResponse(ctx, d)
}

// ListReports serves GET /api/v1/admin/reports, newest first. The cursor is the ID of the last
// report of the previous page.
func (s *Server) ListReports(ctx context.Context, req ListReportsRequestObject) (ListReportsResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return ListReportsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListReportsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	rq := appmoderation.ReportQuery{Before: q.before}
	if req.Params.State != nil {
		rq.State = moderation.ReportState(*req.Params.State)
	}
	recs, err := s.o.Moderation.Reports(ctx, rq, q.limit+1)
	if errors.Is(err, appmoderation.ErrInvalidQuery) {
		return ListReportsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: list reports: %w", err)
	}
	recs, last := trim(recs, q.limit)
	items, err := s.newAdminReports(ctx, recs)
	if err != nil {
		return nil, err
	}
	page := AdminReportPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(recs[last].Report.ID)
	}
	cache := cacheNoStore
	return ListReports200JSONResponse{Body: page, Headers: ListReports200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetReport serves GET /api/v1/admin/reports/{id}.
func (s *Server) GetReport(ctx context.Context, req GetReportRequestObject) (GetReportResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return GetReportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	d, err := s.o.Moderation.ReportDetail(ctx, req.Id)
	if errors.Is(err, appmoderation.ErrNotFound) {
		return GetReportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: report %s: %w", req.Id, err)
	}
	res, err := s.reportDetailResponse(ctx, d)
	if err != nil {
		return nil, err
	}
	return GetReport200JSONResponse{res}, nil
}

// assignReport makes assignee ("" for nobody) handle a report for the viewer, a moderator.
func (s *Server) assignReport(ctx context.Context, id string, self bool) (*AdminReportDetailJSONResponse, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil || p != nil {
		return nil, p, err
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	assignee := ""
	if self {
		assignee = v.UserID
	}
	rec, err := s.o.Moderation.AssignReport(ctx, id, assignee, v.UserID)
	if errors.Is(err, appmoderation.ErrNotFound) {
		p := s.notFoundResponse(ctx)
		return nil, &p, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("clientapi: assign report %s: %w", id, err)
	}
	res, err := s.reportResponse(ctx, rec)
	return &res, nil, err
}

// AssignReportToSelf serves POST /api/v1/admin/reports/{id}/assign_to_self.
func (s *Server) AssignReportToSelf(ctx context.Context, req AssignReportToSelfRequestObject) (AssignReportToSelfResponseObject, error) {
	res, p, err := s.assignReport(ctx, req.Id, true)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return AssignReportToSelfdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return AssignReportToSelf200JSONResponse{*res}, nil
}

// UnassignReport serves POST /api/v1/admin/reports/{id}/unassign.
func (s *Server) UnassignReport(ctx context.Context, req UnassignReportRequestObject) (UnassignReportResponseObject, error) {
	res, p, err := s.assignReport(ctx, req.Id, false)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UnassignReportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return UnassignReport200JSONResponse{*res}, nil
}

// ResolveReport serves POST /api/v1/admin/reports/{id}/resolve.
func (s *Server) ResolveReport(ctx context.Context, req ResolveReportRequestObject) (ResolveReportResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageReports); err != nil {
		return nil, err
	} else if p != nil {
		return ResolveReportdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return ResolveReportdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "outcome")), nil
	}
	rec, err := s.o.Moderation.ResolveReport(ctx, req.Id, moderation.ReportOutcome(req.Body.Outcome), v.UserID)
	switch {
	case errors.Is(err, appmoderation.ErrNotFound):
		return ResolveReportdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, moderation.ErrInvalidReport):
		l := s.Localizer(ctx)
		return ResolveReportdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("outcome"))), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: resolve report %s: %w", req.Id, err)
	}
	res, err := s.reportResponse(ctx, rec)
	if err != nil {
		return nil, err
	}
	return ResolveReport200JSONResponse{res}, nil
}
