// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// ProblemTypeCurrentSession is the problem of signing out the browser that asks through the list
// of browsers (ADR-0090 §12).
const ProblemTypeCurrentSession = "/api/v1/problems/current-session"

// SignedInBrowsers runs the browsers signed in to the viewer's account (ADR-0090).
// *appidentity.Service implements it.
type SignedInBrowsers interface {
	SignedInBrowsers(ctx context.Context, userID, currentFamilyID string) ([]appidentity.SignedInBrowser, error)
	RevokeSignedInBrowser(ctx context.Context, userID, currentFamilyID, sessionID string) error
	RevokeOtherSignedInBrowsers(ctx context.Context, userID, currentFamilyID string) error
}

func newBrowserSession(b appidentity.SignedInBrowser) BrowserSession {
	out := BrowserSession{Id: b.SessionID, Current: b.Current, SignedInAt: b.SignedInAt.UTC()}
	if b.IP != "" {
		out.Ip = &b.IP
	}
	if b.UserAgent != "" {
		out.UserAgent = &b.UserAgent
	}
	if b.LastUsedAt != nil {
		t := b.LastUsedAt.UTC()
		out.LastUsedAt = &t
	}
	return out
}

// ListSessions serves GET /api/v1/sessions.
func (s *Server) ListSessions(ctx context.Context, _ ListSessionsRequestObject) (ListSessionsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.SignedInBrowsers == nil {
		return ListSessionsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	browsers, err := s.o.SignedInBrowsers.SignedInBrowsers(ctx, v.UserID, v.FamilyID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the signed-in browsers of %s: %w", v.UserID, err)
	}
	items := make([]BrowserSession, len(browsers))
	for i, b := range browsers {
		items[i] = newBrowserSession(b)
	}
	cache := cacheNoStore
	return ListSessions200JSONResponse{Body: BrowserSessionList{Items: items}, Headers: ListSessions200ResponseHeaders{CacheControl: &cache}}, nil
}

// RevokeSession serves DELETE /api/v1/sessions/{id}.
func (s *Server) RevokeSession(ctx context.Context, req RevokeSessionRequestObject) (RevokeSessionResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.SignedInBrowsers == nil {
		return RevokeSessiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	err = s.o.SignedInBrowsers.RevokeSignedInBrowser(ctx, v.UserID, v.FamilyID, req.Id)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return RevokeSessiondefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrCurrentBrowserSession):
		return RevokeSessiondefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusConflict, ProblemTypeCurrentSession, s.Localizer(ctx).ServerErrorsCurrentSession(), "")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: revoke session %s of %s: %w", req.Id, v.UserID, err)
	}
	return RevokeSession204Response{}, nil
}

// RevokeOtherSessions serves POST /api/v1/sessions/revoke_others.
func (s *Server) RevokeOtherSessions(ctx context.Context, _ RevokeOtherSessionsRequestObject) (RevokeOtherSessionsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.SignedInBrowsers == nil {
		return RevokeOtherSessionsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err := s.o.SignedInBrowsers.RevokeOtherSignedInBrowsers(ctx, v.UserID, v.FamilyID); err != nil {
		return nil, fmt.Errorf("clientapi: revoke the other sessions of %s: %w", v.UserID, err)
	}
	return RevokeOtherSessions204Response{}, nil
}
