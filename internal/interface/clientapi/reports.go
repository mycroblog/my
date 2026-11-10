// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/kernel"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// ReportLimit bounds the reports that one account files (createReport): each report lands in
// the moderators' queue.
var ReportLimit = kernel.Limit{Name: "api.report", Max: 30, Window: time.Hour}

// Reports files the reports of local users (FR-701). *appmoderation.LocalReports implements it.
type Reports interface {
	File(ctx context.Context, in appmoderation.LocalReportInput) (moderation.Report, error)
}

// FiledReports reads the reports that local users filed and whether each is resolved (FR-711).
// *appmoderation.Service implements it.
type FiledReports interface {
	ReportsFiledBy(ctx context.Context, reporterAccountID, before string, limit int) ([]moderation.ReportRecord, error)
	ReportFiledBy(ctx context.Context, reporterAccountID, id string) (moderation.ReportRecord, error)
}

// CreateReport serves POST /api/v1/reports: the viewer reports an account and some of its
// statuses to the moderators. The response presents the report without its reporter.
func (s *Server) CreateReport(ctx context.Context, req CreateReportRequestObject) (CreateReportResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if p, h, limited := s.LimitProblem(ctx, ReportLimit, v.AccountID); limited {
		return CreateReportdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	b := req.Body
	if b == nil {
		return CreateReportdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "account_id")), nil
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (CreateReportResponseObject, error) {
		return CreateReportdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	in := appmoderation.LocalReportInput{
		ReporterAccountID: v.AccountID, TargetAccountID: knownID(b.AccountId), StatusIDs: knownIDs(deref(b.StatusIds)),
		Category: moderation.Category(b.Category), RuleIDs: knownIDs(deref(b.RuleIds)), Comment: deref(b.Comment),
		Forward: deref(b.Forward),
	}
	if b.VoiceRoomId != nil {
		// A room that the reporter can see, whose host is the account reported (ADR-0100 §11).
		if s.o.VoiceModeration == nil {
			return unprocessable(l.ServerErrorsInvalidValue("voice_room_id"))
		}
		t, err := s.o.VoiceModeration.ReportTarget(ctx, v.AccountID, knownID(*b.VoiceRoomId))
		switch {
		case errors.Is(err, appvoice.ErrNotFound), errors.Is(err, appvoice.ErrDisabled):
			return unprocessable(l.ServerErrorsInvalidValue("voice_room_id"))
		case err != nil:
			return nil, fmt.Errorf("clientapi: the voice chat room %s of a report: %w", *b.VoiceRoomId, err)
		case t.HostAccountID != in.TargetAccountID:
			return unprocessable(l.ServerErrorsInvalidValue("account_id"))
		}
		in.VoiceRoom = &moderation.ReportVoiceRoom{RoomID: t.RoomID, RevisionID: t.RevisionID}
	}
	r, err := s.o.Reports.File(ctx, in)
	var (
		fieldErr *appmoderation.ReportFieldError
		textErr  *moderation.TextError
		countErr *moderation.CountError
	)
	switch {
	case errors.As(err, &fieldErr):
		return unprocessable(l.ServerErrorsInvalidValue(fieldErr.Field))
	case errors.As(err, &countErr):
		return unprocessable(l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: countErr.Max, Name: countErr.Field}))
	case errors.As(err, &textErr) && textErr.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: textErr.Max, Name: textErr.Field}))
	case errors.As(err, &textErr):
		return unprocessable(l.ServerErrorsInvalidValue(textErr.Field))
	case err != nil:
		return nil, fmt.Errorf("clientapi: file a report on %s: %w", b.AccountId, err)
	}
	items, err := s.newReports(ctx, v, []moderation.ReportRecord{{Report: r, Status: moderation.ReportStatus{State: moderation.ReportOpen}}})
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, fmt.Errorf("clientapi: the target %s of report %s: %w", r.TargetAccountID, r.ID, appaccount.ErrNotFound)
	}
	cache := cacheNoStore
	return CreateReport201JSONResponse{
		Body:    items[0],
		Headers: CreateReport201ResponseHeaders{CacheControl: &cache},
	}, nil
}

// ListOwnReports serves GET /api/v1/reports: a page of the reports that the viewer filed,
// newest first, each with whether it is resolved. The cursor is the ID of the last report of
// the previous page.
func (s *Server) ListOwnReports(ctx context.Context, req ListOwnReportsRequestObject) (ListOwnReportsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListOwnReportsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	recs, err := s.o.FiledReports.ReportsFiledBy(ctx, v.AccountID, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the reports of %s: %w", v.AccountID, err)
	}
	recs, last := trim(recs, q.limit)
	items, err := s.newReports(ctx, v, recs)
	if err != nil {
		return nil, err
	}
	page := ReportPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(recs[last].Report.ID)
	}
	cache := cacheNoStore
	return ListOwnReports200JSONResponse{Body: page, Headers: ListOwnReports200ResponseHeaders{CacheControl: &cache}}, nil
}

// newReports presents the reports recs of the reporter viewer, in order, as the Report schema, with
// each target account read once. The reporter sees of the target what getAccount shows them, so a
// target that is not presentable to them (suspended, deleted, or blocking them) is left out of a
// report that stays listed. A report whose target account no longer exists is left out.
func (s *Server) newReports(ctx context.Context, viewer Viewer, recs []moderation.ReportRecord) ([]Report, error) {
	var targets []string
	for _, rec := range recs {
		if id := rec.Report.TargetAccountID; !slices.Contains(targets, id) {
			targets = append(targets, id)
		}
	}
	found, err := s.o.Accounts.FindByIDs(ctx, targets)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the targets of the reports: %w", err)
	}
	shown, err := s.presentableOf(ctx, &viewer, found, targets, true)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the targets of the reports: %w", err)
	}
	var actors []appaccount.Actor
	for _, id := range targets {
		if a, ok := shown[id]; ok {
			actors = append(actors, a)
		}
	}
	accounts, err := s.newAccounts(ctx, actors)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*Account, len(accounts))
	for i, a := range actors {
		byID[a.Account.ID] = &accounts[i]
	}
	out := make([]Report, 0, len(recs))
	for _, rec := range recs {
		if _, ok := found[rec.Report.TargetAccountID]; ok {
			out = append(out, newReport(rec, byID[rec.Report.TargetAccountID]))
		}
	}
	return out, nil
}

// newReport presents a report to its reporter: whether it is open or resolved, never its
// outcome or who handles it. target is nil when the reporter may not see the account.
func newReport(rec moderation.ReportRecord, target *Account) Report {
	r := rec.Report
	state := ReportStateOpen
	if rec.Status.State == moderation.ReportResolved {
		state = ReportStateResolved
	}
	out := Report{
		Id: r.ID, Category: ReportCategory(r.Category), Comment: r.Comment, Forwarded: r.Forwarded,
		FiledAt: r.FiledAt.UTC(), TargetAccount: target, State: state,
		StatusIds: append([]string{}, r.StatusIDs...), RuleIds: append([]string{}, r.RuleIDs...),
	}
	if r.VoiceRoom != nil {
		id := r.VoiceRoom.RoomID
		out.VoiceRoomId = &id
	}
	return out
}
