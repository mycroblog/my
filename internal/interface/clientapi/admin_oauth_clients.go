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
	DenyOAuthClientHost(ctx context.Context, host, byUserID string) (identity.OAuthClientHostDenial, bool, error)
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

// knowsUsers reports whether the server can tell the accounts of users; without them, what a user
// did stays unattributed rather than attributed to a deleted account.
func (s *Server) knowsUsers() bool { return s.o.UserAccounts != nil && s.o.Accounts != nil }

// newAdminOAuthClients presents clients with the accounts of their owners, each looked up once.
func (s *Server) newAdminOAuthClients(ctx context.Context, list []identity.OAuthClientListing) ([]AdminOAuthClient, error) {
	ids := make([]string, len(list))
	for i, l := range list {
		ids[i] = l.OwnerUserID
	}
	accounts, err := s.userAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]AdminOAuthClient, len(list))
	for i, l := range list {
		out[i] = newAdminOAuthClient(l)
		if l.OwnerUserID == "" {
			continue
		}
		if a, ok := accounts[l.OwnerUserID]; ok {
			out[i].Owner = &a
		} else if s.knowsUsers() {
			out[i].OwnerDeleted = true
		}
	}
	return out, nil
}

func newAdminOAuthClient(l identity.OAuthClientListing) AdminOAuthClient {
	out := AdminOAuthClient{
		Id: l.ID, ClientId: l.ClientID, Name: l.Name, Registration: OAuthClientRegistration(l.Registration),
		Confidential: !l.Public(), Scopes: l.Scope.Tokens(), State: AdminOAuthClientStateEnabled, CreatedAt: l.CreatedAt.UTC(),
	}
	if l.Disabled {
		out.State = AdminOAuthClientStateDisabled
	}
	if l.Website != "" {
		out.Website = &l.Website
	}
	return out
}

func (s *Server) adminOAuthClientResponse(ctx context.Context, l identity.OAuthClientListing) (AdminOAuthClientJSONResponse, error) {
	presented, err := s.newAdminOAuthClients(ctx, []identity.OAuthClientListing{l})
	if err != nil {
		return AdminOAuthClientJSONResponse{}, err
	}
	cache := cacheNoStore
	return AdminOAuthClientJSONResponse{Body: presented[0], Headers: AdminOAuthClientResponseHeaders{CacheControl: &cache}}, nil
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
	items, err := s.newAdminOAuthClients(ctx, list)
	if err != nil {
		return nil, err
	}
	page := AdminOAuthClientPage{Items: items}
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
	res, err := s.adminOAuthClientResponse(ctx, l)
	if err != nil {
		return nil, err
	}
	return DisableOAuthClient200JSONResponse{res}, nil
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
	res, err := s.adminOAuthClientResponse(ctx, l)
	if err != nil {
		return nil, err
	}
	return EnableOAuthClient200JSONResponse{res}, nil
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
	items, err := s.newOAuthClientHostDenials(ctx, list)
	if err != nil {
		return nil, err
	}
	page := AdminOAuthClientHostDenialPage{Items: items}
	if last >= 0 {
		page.NextCursor = encodeCursor(list[last].ID)
	}
	cache := cacheNoStore
	return ListOAuthClientHostDenials200JSONResponse{Body: page, Headers: ListOAuthClientHostDenials200ResponseHeaders{CacheControl: &cache}}, nil
}

// newOAuthClientHostDenials presents denials as the AdminOAuthClientHostDenial schema.
func (s *Server) newOAuthClientHostDenials(ctx context.Context, list []identity.OAuthClientHostDenial) ([]AdminOAuthClientHostDenial, error) {
	ids := make([]string, len(list))
	for i, d := range list {
		ids[i] = d.DeniedByUserID
	}
	accounts, err := s.userAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]AdminOAuthClientHostDenial, 0, len(list))
	for _, d := range list {
		item := AdminOAuthClientHostDenial{Id: d.ID, Host: d.Host, DeniedAt: d.DeniedAt.UTC()}
		if d.DeniedByUserID != "" {
			if a, ok := accounts[d.DeniedByUserID]; ok {
				item.DeniedBy = &a
			} else if s.knowsUsers() {
				item.DeniedByDeleted = true
			}
		}
		out = append(out, item)
	}
	return out, nil
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
	denial, created, err := s.o.OAuthClients.DenyOAuthClientHost(ctx, req.Body.Host, v.UserID)
	switch {
	case errors.Is(err, identity.ErrInvalidClientIDURL):
		return DenyOAuthClientHostdefaultApplicationProblemPlusJSONResponse(s.unprocessableValue(ctx, "host")), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: deny oauth client host: %w", err)
	}
	items, err := s.newOAuthClientHostDenials(ctx, []identity.OAuthClientHostDenial{denial})
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	if created {
		return DenyOAuthClientHost201JSONResponse{Body: items[0], Headers: DenyOAuthClientHost201ResponseHeaders{CacheControl: &cache}}, nil
	}
	return DenyOAuthClientHost200JSONResponse{Body: items[0], Headers: DenyOAuthClientHost200ResponseHeaders{CacheControl: &cache}}, nil
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
	// A host that cannot be denied is not denied, which is what the lift asks for.
	if err != nil && !errors.Is(err, identity.ErrInvalidClientIDURL) {
		return nil, fmt.Errorf("clientapi: allow oauth client host: %w", err)
	}
	return AllowOAuthClientHost204Response{}, nil
}
