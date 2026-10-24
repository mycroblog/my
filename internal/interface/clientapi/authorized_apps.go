// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// AuthorizedApps runs the viewer's authorized OAuth apps (ADR-0090 §5). *appidentity.Service
// implements it.
type AuthorizedApps interface {
	AuthorizedApps(ctx context.Context, userID string) ([]appidentity.AuthorizedApp, error)
	RevokeAuthorizedApp(ctx context.Context, userID, clientID string) error
}

func newAuthorizedApp(a appidentity.AuthorizedApp) AuthorizedApp {
	out := AuthorizedApp{
		Id: a.Client.ID, ClientId: a.Client.ClientID, Name: a.Client.Name,
		Registration: AuthorizedAppRegistration(a.Client.Registration), FirstParty: a.Client.FirstParty(),
		Scope: string(a.Scope), AuthorizedAt: a.AuthorizedAt.UTC(),
	}
	if a.Client.Website != "" {
		out.Website = &a.Client.Website
	}
	if a.OwnerUsername != "" {
		out.RegisteredBy = &a.OwnerUsername
	}
	if a.LastUsedAt != nil {
		t := a.LastUsedAt.UTC()
		out.LastUsedAt = &t
	}
	return out
}

// ListAuthorizedApps serves GET /api/v1/authorized_apps.
func (s *Server) ListAuthorizedApps(ctx context.Context, _ ListAuthorizedAppsRequestObject) (ListAuthorizedAppsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.AuthorizedApps == nil {
		return ListAuthorizedAppsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	apps, err := s.o.AuthorizedApps.AuthorizedApps(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the authorized apps of %s: %w", v.UserID, err)
	}
	items := make([]AuthorizedApp, len(apps))
	for i, a := range apps {
		items[i] = newAuthorizedApp(a)
	}
	cache := cacheNoStore
	return ListAuthorizedApps200JSONResponse{Body: AuthorizedAppList{Items: items}, Headers: ListAuthorizedApps200ResponseHeaders{CacheControl: &cache}}, nil
}

// RevokeAuthorizedApp serves DELETE /api/v1/authorized_apps/{id}.
func (s *Server) RevokeAuthorizedApp(ctx context.Context, req RevokeAuthorizedAppRequestObject) (RevokeAuthorizedAppResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.AuthorizedApps == nil {
		return RevokeAuthorizedAppdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	err = s.o.AuthorizedApps.RevokeAuthorizedApp(ctx, v.UserID, req.Id)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return RevokeAuthorizedAppdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: revoke app %s of %s: %w", req.Id, v.UserID, err)
	}
	return RevokeAuthorizedApp204Response{}, nil
}
