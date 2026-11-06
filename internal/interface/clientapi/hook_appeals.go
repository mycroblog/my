// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	apphook "github.com/mycroblog/my/internal/app/hook"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/hook"
	"github.com/mycroblog/my/internal/i18n"
)

// The refusal of a post by the decision hook local.status and its appeal (ADR-0147 §9, §12): the
// author sees fixed text and the decision to appeal, appeals it with the text of the post, and a
// moderator with manage_reports decides the appeal under /admin/hook_appeals. No plugin is called
// on any of these paths.

// ProblemTypeStatusRejected is the problem of a post that the hook local.status refused.
const ProblemTypeStatusRejected = "/api/v1/problems/status-rejected"

// HookAppeals runs the use cases of the appeals of refused posts. *apphook.Service implements it.
type HookAppeals interface {
	FileAppeal(ctx context.Context, in apphook.AppealInput) (hook.AppealRecord, error)
	// AppealDeadline returns the last moment a decision made at decidedAt can be appealed.
	AppealDeadline(ctx context.Context, decidedAt time.Time) (time.Time, error)
	ListAppeals(ctx context.Context, q apphook.AppealQuery) ([]hook.AppealRecord, error)
	DecideAppeal(ctx context.Context, appealID string, outcome hook.AppealOutcome, byUserID string) (hook.AppealRecord, error)
}

// statusRejected returns the answer to a post that local.status refused: fixed text that holds
// nothing of the plugin's answer, and the decision with its deadline. ok is false for another error.
func (s *Server) statusRejected(ctx context.Context, err error) (CreateStatusResponseObject, bool, error) {
	var rejected *appcontent.RejectedByReviewError
	if !errors.As(err, &rejected) {
		return nil, false, nil
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeStatusRejected,
		l.ServerErrorsStatusRejected(), l.ServerErrorsStatusRejectedDetail())
	out := StatusRejectedProblem{
		Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId,
	}
	if rejected.DecisionID != "" {
		out.HookDecisionId = &rejected.DecisionID
		if s.o.HookAppeals != nil {
			deadline, err := s.o.HookAppeals.AppealDeadline(ctx, rejected.DecidedAt)
			if err != nil {
				return nil, true, fmt.Errorf("clientapi: the appeal deadline of decision %s: %w", rejected.DecisionID, err)
			}
			deadline = deadline.UTC()
			out.AppealDeadline = &deadline
		}
	}
	return CreateStatus422ApplicationProblemPlusJSONResponse{StatusRejectedProblemApplicationProblemPlusJSONResponse{
		Body: out, Headers: StatusRejectedProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}}, true, nil
}

func hookAppealState(r hook.AppealRecord) HookAppealState {
	if r.Review == nil {
		return HookAppealStatePending
	}
	return HookAppealState(r.Review.Outcome)
}

// AppealHookDecision serves POST /api/v1/hook_decisions/{id}/appeal.
func (s *Server) AppealHookDecision(ctx context.Context, req AppealHookDecisionRequestObject) (AppealHookDecisionResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.HookAppeals == nil || !kernel.ValidID(req.Id) {
		return AppealHookDecisiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return AppealHookDecisiondefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "status_text")), nil
	}
	l := s.Localizer(ctx)
	refuse := func(typ, title, detail string) (AppealHookDecisionResponseObject, error) {
		return AppealHookDecisiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, typ, title, detail)), nil
	}
	a, err := s.o.HookAppeals.FileAppeal(ctx, apphook.AppealInput{
		AccountID: v.AccountID, DecisionID: req.Id, StatusText: req.Body.StatusText, Comment: deref(req.Body.Comment),
	})
	var textErr *hook.AppealTextError
	switch {
	case errors.Is(err, apphook.ErrAppealNotFound):
		return AppealHookDecisiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, apphook.ErrAlreadyAppealed):
		return refuse(ProblemTypeAlreadyAppealed, l.ServerErrorsHookAppealAlreadyAppealed(), "")
	case errors.Is(err, hook.ErrAppealWindowClosed):
		return refuse(ProblemTypeAppealWindowClosed, l.ServerErrorsHookAppealWindowClosed(), "")
	case errors.Is(err, hook.ErrNotAppealable):
		return refuse(ProblemTypeNotAppealable, l.ServerErrorsHookAppealNotAppealable(), "")
	case errors.As(err, &textErr) && textErr.Max > 0:
		return refuse(ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: textErr.Max, Name: textErr.Field}))
	case errors.As(err, &textErr):
		return refuse(ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(textErr.Field))
	case err != nil:
		return nil, fmt.Errorf("clientapi: appeal hook decision %s of account %s: %w", req.Id, v.AccountID, err)
	}
	cache := cacheNoStore
	return AppealHookDecision200JSONResponse{
		Body: OwnHookAppeal{
			Id: a.Appeal.ID, HookDecisionId: a.Appeal.DecisionID, State: hookAppealState(a), SubmittedAt: a.Appeal.SubmittedAt.UTC(),
		},
		Headers: AppealHookDecision200ResponseHeaders{CacheControl: &cache},
	}, nil
}

func newAdminHookAppeal(r hook.AppealRecord) AdminHookAppeal {
	out := AdminHookAppeal{
		Id: r.Appeal.ID, Decision: newHookDecision(r.Decision), AccountId: r.Appeal.AccountID, StatusText: r.Appeal.StatusText,
		State: hookAppealState(r), SubmittedAt: r.Appeal.SubmittedAt.UTC(),
	}
	if r.Appeal.Comment != "" {
		out.Comment = &r.Appeal.Comment
	}
	if r.Review != nil {
		at := r.Review.DecidedAt.UTC()
		out.DecidedAt = &at
	}
	return out
}

// requireHookAppeals checks manage_reports; a server built without HookAppeals answers 404.
func (s *Server) requireHookAppeals(ctx context.Context) (*problem, error) {
	if p, err := s.requireHookDecisions(ctx); err != nil || p != nil {
		return p, err
	}
	if s.o.HookAppeals == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// ListHookAppeals serves GET /api/v1/admin/hook_appeals.
func (s *Server) ListHookAppeals(ctx context.Context, req ListHookAppealsRequestObject) (ListHookAppealsResponseObject, error) {
	if p, err := s.requireHookAppeals(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListHookAppealsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListHookAppealsdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	aq := apphook.AppealQuery{Before: pq.before, Limit: pq.limit + 1}
	if st := req.Params.State; st != nil {
		switch *st {
		case HookAppealStatePending:
			aq.Pending = true
		case HookAppealStateUpheld, HookAppealStateOverturned:
			aq.Outcome = hook.AppealOutcome(*st)
		default:
			return ListHookAppealsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
		}
	}
	appeals, err := s.o.HookAppeals.ListAppeals(ctx, aq)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list hook appeals: %w", err)
	}
	appeals, last := trim(appeals, pq.limit)
	page := AdminHookAppealPage{Items: make([]AdminHookAppeal, len(appeals))}
	for i, a := range appeals {
		page.Items[i] = newAdminHookAppeal(a)
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(appeals[last].Appeal.ID)
	}
	cache := cacheNoStore
	return ListHookAppeals200JSONResponse{Body: page, Headers: ListHookAppeals200ResponseHeaders{CacheControl: &cache}}, nil
}

// decideHookAppeal runs the review of an appeal, or returns the problem that answers it.
func (s *Server) decideHookAppeal(ctx context.Context, id string, outcome hook.AppealOutcome) (*AdminHookAppeal, *problem, error) {
	if p, err := s.requireHookAppeals(ctx); err != nil || p != nil {
		return nil, p, err
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !kernel.ValidID(id) {
		p := s.notFoundResponse(ctx)
		return nil, &p, nil
	}
	rec, err := s.o.HookAppeals.DecideAppeal(ctx, id, outcome, v.UserID)
	switch {
	case errors.Is(err, apphook.ErrAppealNotFound):
		p := s.notFoundResponse(ctx)
		return nil, &p, nil
	case errors.Is(err, apphook.ErrAppealDecided):
		p := s.problem(ctx, http.StatusConflict, ProblemTypeAppealDecided, s.Localizer(ctx).ServerErrorsAppealDecided(), "")
		return nil, &p, nil
	case err != nil:
		return nil, nil, fmt.Errorf("clientapi: decide hook appeal %s: %w", id, err)
	}
	out := newAdminHookAppeal(rec)
	return &out, nil, nil
}

// UpholdHookAppeal serves POST /api/v1/admin/hook_appeals/{id}/uphold.
func (s *Server) UpholdHookAppeal(ctx context.Context, req UpholdHookAppealRequestObject) (UpholdHookAppealResponseObject, error) {
	out, p, err := s.decideHookAppeal(ctx, req.Id, hook.AppealUpheld)
	if err != nil || p != nil {
		if p == nil {
			return nil, err
		}
		return UpholdHookAppealdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return UpholdHookAppeal200JSONResponse{AdminHookAppealJSONResponse{Body: *out, Headers: AdminHookAppealResponseHeaders{CacheControl: &cache}}}, nil
}

// OverturnHookAppeal serves POST /api/v1/admin/hook_appeals/{id}/overturn.
func (s *Server) OverturnHookAppeal(ctx context.Context, req OverturnHookAppealRequestObject) (OverturnHookAppealResponseObject, error) {
	out, p, err := s.decideHookAppeal(ctx, req.Id, hook.AppealOverturned)
	if err != nil || p != nil {
		if p == nil {
			return nil, err
		}
		return OverturnHookAppealdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return OverturnHookAppeal200JSONResponse{AdminHookAppealJSONResponse{Body: *out, Headers: AdminHookAppealResponseHeaders{CacheControl: &cache}}}, nil
}
