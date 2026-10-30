// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The custom emoji of the holders of create_custom_emojis (ADR-0070 §6): their requests
// (/custom_emoji_requests) and the emoji they made (/custom_emojis/mine), in the user namespace
// with the scopes read:custom_emojis and write:custom_emojis, and the review of the requests by
// the holders of manage_custom_emojis on /admin/custom_emoji_requests.

// ProblemTypeCustomEmojiRequestConflict is the problem type of a custom emoji request that
// cannot be decided or withdrawn (409), whose reason says why.
const ProblemTypeCustomEmojiRequestConflict = "/api/v1/problems/custom-emoji-request-conflict"

// CustomEmojiRequests runs the use cases of the custom emoji requests and of the creators' own
// emoji. *appcontent.CustomEmojiService implements it.
type CustomEmojiRequests interface {
	SubmitRequest(ctx context.Context, in appcontent.SubmitCustomEmojiRequestInput) (appcontent.CustomEmojiRequestView, error)
	WithdrawRequest(ctx context.Context, id, userID string) (appcontent.CustomEmojiRequestView, error)
	ApproveRequest(ctx context.Context, id, byUserID string) (appcontent.CustomEmojiRequestView, error)
	RejectRequest(ctx context.Context, id, reason, byUserID string) (appcontent.CustomEmojiRequestView, error)
	ListRequests(ctx context.Context, q appcontent.CustomEmojiRequestQuery, limit int) ([]appcontent.CustomEmojiRequestView, error)
	RequestsByID(ctx context.Context, ids []string) ([]appcontent.CustomEmojiRequestView, error)
	Usage(ctx context.Context, userID string) (appcontent.CustomEmojiUsage, error)
	ListOwn(ctx context.Context, userID, before string, limit int) ([]appcontent.AdminCustomEmoji, error)
	UpdateOwn(ctx context.Context, id, userID string, u appcontent.OwnCustomEmojiUpdate) (appcontent.AdminCustomEmoji, error)
	DeleteOwn(ctx context.Context, id, userID string) error
}

// requireEmojiCreator checks the create_custom_emojis permission and that the use cases are
// configured; it returns the problem to answer otherwise.
func (s *Server) requireEmojiCreator(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermCreateCustomEmojis); err != nil || p != nil {
		return p, err
	}
	if s.o.CustomEmojiRequests == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// requireEmojiReviewer checks the manage_custom_emojis permission and that the use cases are
// configured; it returns the problem to answer otherwise.
func (s *Server) requireEmojiReviewer(ctx context.Context) (*problem, error) {
	if p, err := s.requireCustomEmojis(ctx); err != nil || p != nil {
		return p, err
	}
	if s.o.CustomEmojiRequests == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// missingPermission returns the 403 of a viewer who no longer may create custom emoji.
func (s *Server) missingPermission(ctx context.Context) problem {
	l := s.Localizer(ctx)
	return s.problem(ctx, http.StatusForbidden, ProblemTypeMissingPermission, l.ServerErrorsForbidden(),
		l.ServerErrorsMissingPermissionDetail(string(identity.PermCreateCustomEmojis)))
}

// newCustomEmojiRequest presents a request.
func newCustomEmojiRequest(r appcontent.CustomEmojiRequestView) CustomEmojiRequest {
	out := CustomEmojiRequest{
		Id: r.Request.ID, Kind: CustomEmojiRequestKind(r.Request.Kind), State: CustomEmojiRequestState(r.State()),
		Shortcode: r.Request.Shortcode, CreatedAt: r.Request.CreatedAt.UTC(),
	}
	if id := r.Request.EmojiID; id != "" {
		out.EmojiId = &id
	}
	if o := r.Outcome; o != nil {
		at := o.DecidedAt.UTC()
		out.DecidedAt = &at
		if o.EmojiID != "" {
			id := o.EmojiID
			out.EmojiId = &id
		}
		if o.Reason != "" {
			reason := o.Reason
			out.Reason = &reason
		}
	}
	if v := r.View; v != nil {
		out.Url, out.StaticUrl = &v.URL, &v.StaticURL
	}
	return out
}

// userAccounts presents the local accounts of users, keyed by user ID; a user whose account is
// gone is left out.
func (s *Server) userAccounts(ctx context.Context, userIDs []string) (map[string]Account, error) {
	if s.o.UserAccounts == nil || s.o.Accounts == nil {
		return map[string]Account{}, nil
	}
	return s.userAccountsWith(ctx, s.reads(), userIDs)
}

// userAccountsWith is userAccounts through reads.
func (s *Server) userAccountsWith(ctx context.Context, reads presentReads, userIDs []string) (map[string]Account, error) {
	out := map[string]Account{}
	var actors []appaccount.Actor
	var users []string
	seen := map[string]bool{}
	for _, u := range userIDs {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		la, err := reads.LocalAccountOfUser(ctx, u)
		if errors.Is(err, appaccount.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("clientapi: the account of user %s: %w", u, err)
		}
		a, err := reads.FindByID(ctx, la.AccountID)
		if errors.Is(err, appaccount.ErrNotFound) || err == nil && a.Deleted() {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("clientapi: account %s: %w", la.AccountID, err)
		}
		actors, users = append(actors, a), append(users, u)
	}
	presented, err := s.newAccountsWith(ctx, reads, actors)
	if err != nil {
		return nil, err
	}
	for i, a := range presented {
		out[users[i]] = a
	}
	return out, nil
}

// newAdminCustomEmojiRequests presents requests with their requesters.
func (s *Server) newAdminCustomEmojiRequests(ctx context.Context, reqs []appcontent.CustomEmojiRequestView) ([]AdminCustomEmojiRequest, error) {
	ids := make([]string, len(reqs))
	for i, r := range reqs {
		ids[i] = r.Request.RequesterUserID
	}
	accounts, err := s.userAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]AdminCustomEmojiRequest, len(reqs))
	for i, r := range reqs {
		c := newCustomEmojiRequest(r)
		out[i] = AdminCustomEmojiRequest{
			Id: c.Id, Kind: c.Kind, State: c.State, Shortcode: c.Shortcode, EmojiId: c.EmojiId, Url: c.Url, StaticUrl: c.StaticUrl,
			Reason: c.Reason, CreatedAt: c.CreatedAt, DecidedAt: c.DecidedAt,
		}
		if a, ok := accounts[r.Request.RequesterUserID]; ok {
			out[i].Requester = &a
		}
	}
	return out, nil
}

// customEmojiRequestConflict returns the 409 of a request that cannot be decided or withdrawn,
// and false for another error.
func (s *Server) customEmojiRequestConflict(ctx context.Context, err error) (CustomEmojiRequestProblemApplicationProblemPlusJSONResponse, bool) {
	var c *appcontent.CustomEmojiRequestConflict
	if !errors.As(err, &c) {
		return CustomEmojiRequestProblemApplicationProblemPlusJSONResponse{}, false
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusConflict, ProblemTypeCustomEmojiRequestConflict,
		l.ServerErrorsCustomEmojiRequestConflict(), l.ServerErrorsCustomEmojiRequestConflictDetail(string(c.Reason)))
	reason := CustomEmojiRequestConflictReason(c.Reason)
	return CustomEmojiRequestProblemApplicationProblemPlusJSONResponse{
		Body: CustomEmojiRequestProblem{
			Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
		},
		Headers: CustomEmojiRequestProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}, true
}

func customEmojiRequestResponse(body CustomEmojiRequest) CustomEmojiRequestJSONResponse {
	cache := cacheNoStore
	return CustomEmojiRequestJSONResponse{Body: body, Headers: CustomEmojiRequestResponseHeaders{CacheControl: &cache}}
}

func adminCustomEmojiRequestResponse(body AdminCustomEmojiRequest) AdminCustomEmojiRequestJSONResponse {
	cache := cacheNoStore
	return AdminCustomEmojiRequestJSONResponse{Body: body, Headers: AdminCustomEmojiRequestResponseHeaders{CacheControl: &cache}}
}

// ListCustomEmojiRequests serves GET /api/v1/custom_emoji_requests.
func (s *Server) ListCustomEmojiRequests(ctx context.Context, req ListCustomEmojiRequestsRequestObject) (ListCustomEmojiRequestsResponseObject, error) {
	if p, err := s.requireEmojiCreator(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListCustomEmojiRequestsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListCustomEmojiRequestsdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	reqs, err := s.o.CustomEmojiRequests.ListRequests(ctx, appcontent.CustomEmojiRequestQuery{RequesterUserID: v.UserID, Before: pq.before}, pq.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the custom emoji requests of %s: %w", v.UserID, err)
	}
	reqs, last := trim(reqs, pq.limit)
	page := CustomEmojiRequestPage{Items: make([]CustomEmojiRequest, 0, len(reqs))}
	for _, r := range reqs {
		page.Items = append(page.Items, newCustomEmojiRequest(r))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(reqs[last].Request.ID)
	}
	cache := cacheNoStore
	return ListCustomEmojiRequests200JSONResponse{Body: page, Headers: ListCustomEmojiRequests200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateCustomEmojiRequest serves POST /api/v1/custom_emoji_requests.
func (s *Server) CreateCustomEmojiRequest(ctx context.Context, req CreateCustomEmojiRequestRequestObject) (CreateCustomEmojiRequestResponseObject, error) {
	if p, err := s.requireEmojiCreator(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return CreateCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return CreateCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	in := appcontent.SubmitCustomEmojiRequestInput{
		Kind: content.CustomEmojiRequestKind(req.Body.Kind), MediaID: knownID(req.Body.MediaId), RequesterUserID: v.UserID, RequesterAccountID: v.AccountID,
	}
	switch in.Kind {
	case content.RequestCreate:
		if req.Body.Shortcode == nil {
			return CreateCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "shortcode")), nil
		}
		in.Shortcode = *req.Body.Shortcode
	case content.RequestImage:
		if req.Body.EmojiId == nil {
			return CreateCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "emoji_id")), nil
		}
		in.EmojiID = knownID(*req.Body.EmojiId)
	default:
		return CreateCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "kind")), nil
	}
	r, err := s.o.CustomEmojiRequests.SubmitRequest(ctx, in)
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return CreateCustomEmojiRequest422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return CreateCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appcontent.ErrNotCustomEmojiCreator):
		return CreateCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.missingPermission(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: submit custom emoji request: %w", err)
	}
	return CreateCustomEmojiRequest200JSONResponse{customEmojiRequestResponse(newCustomEmojiRequest(r))}, nil
}

// WithdrawCustomEmojiRequest serves POST /api/v1/custom_emoji_requests/{id}/withdraw.
func (s *Server) WithdrawCustomEmojiRequest(ctx context.Context, req WithdrawCustomEmojiRequestRequestObject) (WithdrawCustomEmojiRequestResponseObject, error) {
	if p, err := s.requireEmojiCreator(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return WithdrawCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.o.CustomEmojiRequests.WithdrawRequest(ctx, req.Id, v.UserID)
	if p, ok := s.customEmojiRequestConflict(ctx, err); ok {
		return WithdrawCustomEmojiRequest409ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return WithdrawCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: withdraw custom emoji request %s: %w", req.Id, err)
	}
	return WithdrawCustomEmojiRequest200JSONResponse{customEmojiRequestResponse(newCustomEmojiRequest(r))}, nil
}

// newOwnCustomEmoji presents an emoji to its creator.
func newOwnCustomEmoji(e appcontent.AdminCustomEmoji) OwnCustomEmoji {
	out := OwnCustomEmoji{
		Id: e.Emoji.ID, Shortcode: e.Emoji.Shortcode, Enabled: e.Settings.Enabled, VisibleInPicker: e.Settings.VisibleInPicker,
		Aliases: e.Aliases, CreatedAt: e.Emoji.CreatedAt.UTC(), UpdatedAt: e.Image.RevisedAt.UTC(),
	}
	if out.Aliases == nil {
		out.Aliases = []string{}
	}
	if e.Settings.Category != "" {
		out.Category = &e.Settings.Category
	}
	if v := e.View; v != nil {
		out.Url, out.StaticUrl = &v.URL, &v.StaticURL
	}
	return out
}

// ListOwnCustomEmojis serves GET /api/v1/custom_emojis/mine.
func (s *Server) ListOwnCustomEmojis(ctx context.Context, req ListOwnCustomEmojisRequestObject) (ListOwnCustomEmojisResponseObject, error) {
	if p, err := s.requireEmojiCreator(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListOwnCustomEmojisdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListOwnCustomEmojisdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	emojis, err := s.o.CustomEmojiRequests.ListOwn(ctx, v.UserID, pq.before, pq.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the custom emoji of %s: %w", v.UserID, err)
	}
	usage, err := s.o.CustomEmojiRequests.Usage(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the custom emoji usage of %s: %w", v.UserID, err)
	}
	emojis, last := trim(emojis, pq.limit)
	page := OwnCustomEmojiPage{Items: make([]OwnCustomEmoji, 0, len(emojis)), Used: usage.Used, Pending: usage.Pending, ResetsAt: usage.ResetsAt.UTC()}
	if usage.Limited {
		limit := usage.Limit
		page.Limit = &limit
	}
	for _, e := range emojis {
		page.Items = append(page.Items, newOwnCustomEmoji(e))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(emojis[last].Emoji.ID)
	}
	cache := cacheNoStore
	return ListOwnCustomEmojis200JSONResponse{Body: page, Headers: ListOwnCustomEmojis200ResponseHeaders{CacheControl: &cache}}, nil
}

// UpdateOwnCustomEmoji serves PATCH /api/v1/custom_emojis/mine/{id}.
func (s *Server) UpdateOwnCustomEmoji(ctx context.Context, req UpdateOwnCustomEmojiRequestObject) (UpdateOwnCustomEmojiResponseObject, error) {
	if p, err := s.requireEmojiCreator(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return UpdateOwnCustomEmojidefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return UpdateOwnCustomEmojidefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	e, err := s.o.CustomEmojiRequests.UpdateOwn(ctx, req.Id, v.UserID, appcontent.OwnCustomEmojiUpdate{
		VisibleInPicker: req.Body.VisibleInPicker, Category: req.Body.Category, Aliases: req.Body.Aliases,
	})
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return UpdateOwnCustomEmoji422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return UpdateOwnCustomEmojidefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appcontent.ErrNotCustomEmojiCreator):
		return UpdateOwnCustomEmojidefaultApplicationProblemPlusJSONResponse(s.missingPermission(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: update own custom emoji %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	return UpdateOwnCustomEmoji200JSONResponse{OwnCustomEmojiJSONResponse{
		Body: newOwnCustomEmoji(e), Headers: OwnCustomEmojiResponseHeaders{CacheControl: &cache},
	}}, nil
}

// DeleteOwnCustomEmoji serves DELETE /api/v1/custom_emojis/mine/{id}.
func (s *Server) DeleteOwnCustomEmoji(ctx context.Context, req DeleteOwnCustomEmojiRequestObject) (DeleteOwnCustomEmojiResponseObject, error) {
	if p, err := s.requireEmojiCreator(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return DeleteOwnCustomEmojidefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.CustomEmojiRequests.DeleteOwn(ctx, req.Id, v.UserID)
	switch {
	case customEmojiNotFound(err):
		return DeleteOwnCustomEmojidefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appcontent.ErrNotCustomEmojiCreator):
		return DeleteOwnCustomEmojidefaultApplicationProblemPlusJSONResponse(s.missingPermission(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: delete own custom emoji %s: %w", req.Id, err)
	}
	return DeleteOwnCustomEmoji204Response{}, nil
}

// ListAdminCustomEmojiRequests serves GET /api/v1/admin/custom_emoji_requests.
func (s *Server) ListAdminCustomEmojiRequests(ctx context.Context, req ListAdminCustomEmojiRequestsRequestObject) (ListAdminCustomEmojiRequestsResponseObject, error) {
	if p, err := s.requireEmojiReviewer(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListAdminCustomEmojiRequestsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	pq := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if pq.problem != nil {
		return ListAdminCustomEmojiRequestsdefaultApplicationProblemPlusJSONResponse(*pq.problem), nil
	}
	q := appcontent.CustomEmojiRequestQuery{State: content.RequestPending, Before: pq.before}
	if st := req.Params.State; st != nil {
		q.State = content.CustomEmojiRequestState(*st)
		if !q.State.Valid() {
			return ListAdminCustomEmojiRequestsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
		}
	}
	reqs, err := s.o.CustomEmojiRequests.ListRequests(ctx, q, pq.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list custom emoji requests: %w", err)
	}
	reqs, last := trim(reqs, pq.limit)
	items, err := s.newAdminCustomEmojiRequests(ctx, reqs)
	if err != nil {
		return nil, err
	}
	page := AdminCustomEmojiRequestPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(reqs[last].Request.ID)
	}
	cache := cacheNoStore
	return ListAdminCustomEmojiRequests200JSONResponse{Body: page, Headers: ListAdminCustomEmojiRequests200ResponseHeaders{CacheControl: &cache}}, nil
}

// adminCustomEmojiRequest presents one request with its requester.
func (s *Server) adminCustomEmojiRequest(ctx context.Context, r appcontent.CustomEmojiRequestView) (AdminCustomEmojiRequest, error) {
	items, err := s.newAdminCustomEmojiRequests(ctx, []appcontent.CustomEmojiRequestView{r})
	if err != nil {
		return AdminCustomEmojiRequest{}, err
	}
	return items[0], nil
}

// ApproveCustomEmojiRequest serves POST /api/v1/admin/custom_emoji_requests/{id}/approve.
func (s *Server) ApproveCustomEmojiRequest(ctx context.Context, req ApproveCustomEmojiRequestRequestObject) (ApproveCustomEmojiRequestResponseObject, error) {
	if p, err := s.requireEmojiReviewer(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ApproveCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.o.CustomEmojiRequests.ApproveRequest(ctx, req.Id, v.UserID)
	if p, ok := s.customEmojiRequestConflict(ctx, err); ok {
		return ApproveCustomEmojiRequest409ApplicationProblemPlusJSONResponse{p}, nil
	}
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return ApproveCustomEmojiRequest422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return ApproveCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: approve custom emoji request %s: %w", req.Id, err)
	}
	out, err := s.adminCustomEmojiRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	return ApproveCustomEmojiRequest200JSONResponse{adminCustomEmojiRequestResponse(out)}, nil
}

// RejectCustomEmojiRequest serves POST /api/v1/admin/custom_emoji_requests/{id}/reject.
func (s *Server) RejectCustomEmojiRequest(ctx context.Context, req RejectCustomEmojiRequestRequestObject) (RejectCustomEmojiRequestResponseObject, error) {
	if p, err := s.requireEmojiReviewer(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return RejectCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	reason := ""
	if req.Body != nil && req.Body.Reason != nil {
		reason = *req.Body.Reason
	}
	r, err := s.o.CustomEmojiRequests.RejectRequest(ctx, req.Id, reason, v.UserID)
	if p, ok := s.customEmojiRequestConflict(ctx, err); ok {
		return RejectCustomEmojiRequest409ApplicationProblemPlusJSONResponse{p}, nil
	}
	if p, ok := s.customEmojiProblem(ctx, err); ok {
		return RejectCustomEmojiRequest422ApplicationProblemPlusJSONResponse{p}, nil
	}
	switch {
	case customEmojiNotFound(err):
		return RejectCustomEmojiRequestdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: reject custom emoji request %s: %w", req.Id, err)
	}
	out, err := s.adminCustomEmojiRequest(ctx, r)
	if err != nil {
		return nil, err
	}
	return RejectCustomEmojiRequest200JSONResponse{adminCustomEmojiRequestResponse(out)}, nil
}
