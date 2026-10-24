// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The OAuth clients and the denied hosts of client metadata documents (ADR-0090 §1,
// manage_oauth_apps): the admin operations that list every client, disable one and lift the
// disablement, and deny and allow the hosts of Client ID Metadata Documents.

// OAuthClients runs the use cases of the OAuth clients. *appidentity.Service implements it.
type OAuthClients interface {
	ListOAuthClients(ctx context.Context, registration identity.OAuthClientRegistration, before string, limit int) ([]identity.OAuthClientListing, error)
	FindOAuthClientListing(ctx context.Context, clientID string) (identity.OAuthClientListing, error)
	DisableOAuthClient(ctx context.Context, clientID, byUserID string) error
	EnableOAuthClient(ctx context.Context, clientID, byUserID string) error
	ListOAuthClientHostDenials(ctx context.Context, before string, limit int) ([]identity.OAuthClientHostDenial, error)
	DenyOAuthClientHost(ctx context.Context, host, byUserID string) error
	AllowOAuthClientHost(ctx context.Context, host, byUserID string) error
}

// requireOAuthClients checks the manage_oauth_apps permission and that the use cases are
// configured; it returns the problem to answer otherwise, and the viewer.
func (s *Server) requireOAuthClients(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageOAuthApps); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.OAuthClients == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

func newAdminOAuthClient(l identity.OAuthClientListing) AdminOAuthClient {
	out := AdminOAuthClient{
		Id: l.ID, ClientId: l.ClientID, Name: l.Name, Registration: OAuthClientRegistration(l.Registration),
		Confidential: !l.Public(), Scope: string(l.Scope), Disabled: l.Disabled, CreatedAt: l.CreatedAt.UTC(),
	}
	if l.Website != "" {
		out.Website = &l.Website
	}
	if l.OwnerUserID != "" {
		out.OwnerId = &l.OwnerUserID
	}
	return out
}

func adminOAuthClientResponse(l identity.OAuthClientListing) AdminOAuthClientJSONResponse {
	cache := cacheNoStore
	return AdminOAuthClientJSONResponse{Body: newAdminOAuthClient(l), Headers: AdminOAuthClientResponseHeaders{CacheControl: &cache}}
}

// unprocessableValue is the 422 of a value that the request names and the server refuses.
func (s *Server) unprocessableValue(ctx context.Context, name string) problem {
	l := s.Localizer(ctx)
	return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(name))
}

// ListAdminOAuthClients serves GET /api/v1/admin/oauth_clients, newest first. The cursor is the
// ID of the last client of the previous page.
func (s *Server) ListAdminOAuthClients(ctx context.Context, req ListAdminOAuthClientsRequestObject) (ListAdminOAuthClientsResponseObject, error) {
	_, p, err := s.requireOAuthClients(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return ListAdminOAuthClientsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	var registration identity.OAuthClientRegistration
	if r := req.Params.Registration; r != nil {
		registration = identity.OAuthClientRegistration(*r)
		switch registration {
		case identity.ClientRegisteredFirstParty, identity.ClientRegisteredByUser,
			identity.ClientRegisteredByMetadataDocument, identity.ClientRegisteredDynamically:
		default:
			return ListAdminOAuthClientsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "registration")), nil
		}
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListAdminOAuthClientsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	list, err := s.o.OAuthClients.ListOAuthClients(ctx, registration, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list oauth clients: %w", err)
	}
	list, last := trim(list, q.limit)
	page := AdminOAuthClientPage{Items: make([]AdminOAuthClient, 0, len(list))}
	for _, l := range list {
		page.Items = append(page.Items, newAdminOAuthClient(l))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(list[last].ID)
	}
	cache := cacheNoStore
	return ListAdminOAuthClients200JSONResponse{Body: page, Headers: ListAdminOAuthClients200ResponseHeaders{CacheControl: &cache}}, nil
}

// DisableOAuthClient serves POST /api/v1/admin/oauth_clients/{id}/disable.
func (s *Server) DisableOAuthClient(ctx context.Context, req DisableOAuthClientRequestObject) (DisableOAuthClientResponseObject, error) {
	v, p, err := s.requireOAuthClients(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return DisableOAuthClientdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.OAuthClients.DisableOAuthClient(ctx, req.Id, v.UserID)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return DisableOAuthClientdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrFirstPartyClient):
		return DisableOAuthClientdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "id")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: disable oauth client %s: %w", req.Id, err)
	}
	l, err := s.o.OAuthClients.FindOAuthClientListing(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: find oauth client %s: %w", req.Id, err)
	}
	return DisableOAuthClient200JSONResponse{adminOAuthClientResponse(l)}, nil
}

// EnableOAuthClient serves POST /api/v1/admin/oauth_clients/{id}/enable.
func (s *Server) EnableOAuthClient(ctx context.Context, req EnableOAuthClientRequestObject) (EnableOAuthClientResponseObject, error) {
	v, p, err := s.requireOAuthClients(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return EnableOAuthClientdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.OAuthClients.EnableOAuthClient(ctx, req.Id, v.UserID)
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return EnableOAuthClientdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case errors.Is(err, appidentity.ErrFirstPartyClient):
		return EnableOAuthClientdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "id")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: enable oauth client %s: %w", req.Id, err)
	}
	l, err := s.o.OAuthClients.FindOAuthClientListing(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: find oauth client %s: %w", req.Id, err)
	}
	return EnableOAuthClient200JSONResponse{adminOAuthClientResponse(l)}, nil
}

// ListOAuthClientHostDenials serves GET /api/v1/admin/oauth_client_host_denials, newest first.
// The cursor is the ID of the last denial of the previous page.
func (s *Server) ListOAuthClientHostDenials(ctx context.Context, req ListOAuthClientHostDenialsRequestObject) (ListOAuthClientHostDenialsResponseObject, error) {
	_, p, err := s.requireOAuthClients(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return ListOAuthClientHostDenialsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListOAuthClientHostDenialsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	list, err := s.o.OAuthClients.ListOAuthClientHostDenials(ctx, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list oauth client host denials: %w", err)
	}
	list, last := trim(list, q.limit)
	page := AdminOAuthClientHostDenialPage{Items: make([]AdminOAuthClientHostDenial, 0, len(list))}
	for _, d := range list {
		item := AdminOAuthClientHostDenial{Id: d.ID, Host: d.Host, DeniedAt: d.DeniedAt.UTC()}
		if d.DeniedByUserID != "" {
			item.DeniedBy = &d.DeniedByUserID
		}
		page.Items = append(page.Items, item)
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(list[last].ID)
	}
	cache := cacheNoStore
	return ListOAuthClientHostDenials200JSONResponse{Body: page, Headers: ListOAuthClientHostDenials200ResponseHeaders{CacheControl: &cache}}, nil
}

// DenyOAuthClientHost serves POST /api/v1/admin/oauth_client_host_denials.
func (s *Server) DenyOAuthClientHost(ctx context.Context, req DenyOAuthClientHostRequestObject) (DenyOAuthClientHostResponseObject, error) {
	v, p, err := s.requireOAuthClients(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return DenyOAuthClientHostdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return DenyOAuthClientHostdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	err = s.o.OAuthClients.DenyOAuthClientHost(ctx, req.Body.Host, v.UserID)
	switch {
	case errors.Is(err, identity.ErrInvalidClientIDURL):
		return DenyOAuthClientHostdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "host")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: deny oauth client host: %w", err)
	}
	return DenyOAuthClientHost204Response{}, nil
}

// AllowOAuthClientHost serves DELETE /api/v1/admin/oauth_client_host_denials/{host}.
func (s *Server) AllowOAuthClientHost(ctx context.Context, req AllowOAuthClientHostRequestObject) (AllowOAuthClientHostResponseObject, error) {
	v, p, err := s.requireOAuthClients(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return AllowOAuthClientHostdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.OAuthClients.AllowOAuthClientHost(ctx, req.Host, v.UserID)
	switch {
	case errors.Is(err, identity.ErrInvalidClientIDURL):
		return AllowOAuthClientHostdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "host")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: allow oauth client host: %w", err)
	}
	return AllowOAuthClientHost204Response{}, nil
}
