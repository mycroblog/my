// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
)

// InteractionRequests lists and answers the requests of replies, boosts, and quotes of the
// viewer's statuses that wait for the viewer's approval (ADR-0155 §10). *appcontent.Interactions
// implements it.
type InteractionRequests interface {
	PendingInteractionRequests(ctx context.Context, accountID string, kinds []content.InteractionKind, before string, limit int) (appcontent.InteractionRequestPage, error)
	AnswerInteractionRequest(ctx context.Context, accountID, requestID string, accept bool) (content.InteractionState, error)
	FindInteractionRequest(ctx context.Context, accountID, id string) (appcontent.InteractionRequestPage, content.InteractionState, error)
}

// ListInteractionRequests serves GET /api/v1/interaction_requests.
func (s *Server) ListInteractionRequests(ctx context.Context, req ListInteractionRequestsRequestObject) (ListInteractionRequestsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListInteractionRequestsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	var kinds []content.InteractionKind
	if k := req.Params.Kind; k != nil {
		kind := content.InteractionKind(*k)
		if !kind.Valid() {
			return ListInteractionRequestsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "kind")), nil
		}
		kinds = []content.InteractionKind{kind}
	}
	out := InteractionRequestPage{Items: []InteractionRequest{}}
	if s.o.InteractionRequests == nil {
		return ListInteractionRequests200JSONResponse(out), nil
	}
	page, err := s.o.InteractionRequests.PendingInteractionRequests(ctx, v.AccountID, kinds, q.before, q.limit)
	if err != nil {
		return nil, err
	}
	states := make(map[string]content.InteractionState, len(page.Requests))
	for _, r := range page.Requests {
		states[r.ID] = content.InteractionState{RequestID: r.ID, Kind: r.Kind, State: content.InteractionRequestPending}
	}
	if out.Items, err = s.presentInteractionRequests(ctx, v, page, states); err != nil {
		return nil, err
	}
	if page.Next != "" {
		out.NextCursor = encodeCursor(page.Next)
	}
	return ListInteractionRequests200JSONResponse(out), nil
}

// AcceptInteractionRequest serves POST /api/v1/interaction_requests/{id}/accept.
func (s *Server) AcceptInteractionRequest(ctx context.Context, req AcceptInteractionRequestRequestObject) (AcceptInteractionRequestResponseObject, error) {
	res, p, err := s.answerInteractionRequest(ctx, req.Id, true)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return AcceptInteractionRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return AcceptInteractionRequest200JSONResponse(*res), nil
}

// RejectInteractionRequest serves POST /api/v1/interaction_requests/{id}/reject.
func (s *Server) RejectInteractionRequest(ctx context.Context, req RejectInteractionRequestRequestObject) (RejectInteractionRequestResponseObject, error) {
	res, p, err := s.answerInteractionRequest(ctx, req.Id, false)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return RejectInteractionRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	return RejectInteractionRequest200JSONResponse(*res), nil
}

// answerInteractionRequest answers the request id of the viewer and presents it as it is then. A
// request answered already is a 409; one of another account's status, or of a deleted status, a
// 404.
func (s *Server) answerInteractionRequest(ctx context.Context, id string, accept bool) (*InteractionRequest, *problem, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	l := s.Localizer(ctx)
	notFound := s.problem(ctx, http.StatusNotFound, ProblemTypeBlank, l.ServerErrorsNotFound(), "")
	if s.o.InteractionRequests == nil {
		return nil, &notFound, nil
	}
	state, err := s.o.InteractionRequests.AnswerInteractionRequest(ctx, v.AccountID, id, accept)
	switch {
	case errors.Is(err, appcontent.ErrNotFound), errors.Is(err, appcontent.ErrNotAuthor):
		return nil, &notFound, nil
	case errors.Is(err, appcontent.ErrRequestAnswered):
		p := s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), l.ServerErrorsInteractionRequestAnswered())
		return nil, &p, nil
	case err != nil:
		return nil, nil, err
	}
	page, _, err := s.o.InteractionRequests.FindInteractionRequest(ctx, v.AccountID, id)
	if err != nil {
		return nil, nil, err
	}
	items, err := s.presentInteractionRequests(ctx, v, page, map[string]content.InteractionState{id: state})
	if err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		return nil, &notFound, nil
	}
	return &items[0], nil, nil
}

// presentInteractionRequests presents the requests of page in order with the states of states,
// reading the accounts and the statuses of all of them together: the statuses of the viewer, and
// the replies and quoting statuses that the viewer may see. A request whose status the viewer may
// not see any more (deleted), or whose account is not presentable, is left out.
func (s *Server) presentInteractionRequests(ctx context.Context, v Viewer, page appcontent.InteractionRequestPage, states map[string]content.InteractionState) ([]InteractionRequest, error) {
	out := []InteractionRequest{}
	if len(page.Requests) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(page.StatusIDs))
	for _, id := range page.StatusIDs {
		ids = append(ids, id)
	}
	accountIDs := make([]string, 0, len(page.Requests))
	for _, r := range page.Requests {
		accountIDs = append(accountIDs, r.InteractingAccountID)
	}
	views, err := s.o.TimelineReader.Views(ctx, localContentViewer(v), ids)
	if err != nil {
		return nil, err
	}
	statuses, err := s.presentStatuses(ctx, views)
	if err != nil {
		return nil, err
	}
	byURI := make(map[string]Status, len(views))
	for i, view := range views {
		byURI[view.Status.URI] = statuses[i]
	}
	accounts, err := s.presentAccounts(ctx, &v, accountIDs, true)
	if err != nil {
		return nil, err
	}
	byAccount := make(map[string]Account, len(accounts))
	for _, a := range accounts {
		byAccount[string(a.Id)] = a
	}
	for _, r := range page.Requests {
		target, ok := byURI[r.TargetURI]
		account, known := byAccount[r.InteractingAccountID]
		if !ok || !known {
			continue
		}
		item := InteractionRequest{
			Id: Id(r.ID), Kind: InteractionKind(r.Kind), State: InteractionRequestState(states[r.ID].State),
			CreatedAt: r.CreatedAt, Account: account, Status: target,
		}
		if r.Kind != content.InteractionAnnounce {
			if st, ok := byURI[r.InteractingURI]; ok {
				item.InteractingStatus = &st
			}
		}
		out = append(out, item)
	}
	return out, nil
}
