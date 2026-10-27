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

// The external login providers (ADR-0106 §2): the admin operations on /admin/login_providers
// (manage_login_providers, server administration). The client secret is accepted on creation
// and rotation and never returned.

// ProblemTypeInvalidLoginProvider is the problem type of a login provider that breaks a rule
// (422), whose reason says which.
const ProblemTypeInvalidLoginProvider = "/api/v1/problems/invalid-login-provider"

// LoginProviders runs the use cases of the login providers. *appidentity.Service implements it.
type LoginProviders interface {
	ListLoginProviders(ctx context.Context) ([]appidentity.LoginProviderView, error)
	LoginProvider(ctx context.Context, id string) (appidentity.LoginProviderView, error)
	CreateLoginProvider(ctx context.Context, settings identity.LoginProviderSettings, clientSecret, byUserID string) (appidentity.LoginProviderView, error)
	UpdateLoginProvider(ctx context.Context, id string, settings identity.LoginProviderSettings, byUserID string) (appidentity.LoginProviderView, error)
	RotateLoginProviderSecret(ctx context.Context, id, clientSecret, byUserID string) (appidentity.LoginProviderView, error)
	EnableLoginProvider(ctx context.Context, id, byUserID string) (appidentity.LoginProviderView, error)
	DisableLoginProvider(ctx context.Context, id, byUserID string) (appidentity.LoginProviderView, error)
	DeleteLoginProvider(ctx context.Context, id, byUserID string) (appidentity.LoginProviderView, error)
}

// requireLoginProviders checks the manage_login_providers permission and that the use cases are
// configured; it returns the problem to answer otherwise, and the viewer.
func (s *Server) requireLoginProviders(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageLoginProviders); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.LoginProviders == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

// newAdminLoginProvider presents a provider to the administrators, never with its secret.
func (s *Server) newAdminLoginProvider(v appidentity.LoginProviderView) AdminLoginProvider {
	p, set := v.Provider, v.Provider.Settings
	out := AdminLoginProvider{
		Id: p.ID, Kind: LoginProviderKind(set.Kind), DisplayName: set.DisplayName, ClientId: set.ClientID,
		Scopes: nonNil(set.Scopes), AllowSignup: set.AllowSignup, AllowLink: set.AllowLink,
		Claims: make([]LoginProviderClaimCondition, 0, len(set.Claims)), Organizations: nonNil(set.Organizations),
		State: AdminLoginProviderState(p.State), StateChangedAt: p.StateChangedAt.UTC(),
		CallbackUrl: s.origin() + identity.LoginProviderCallbackPath(p.ID), LinkedUsers: v.LinkedUsers,
		CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.RevisedAt.UTC(),
	}
	if set.Issuer != "" {
		out.Issuer = &set.Issuer
	}
	for _, c := range set.Claims {
		out.Claims = append(out.Claims, LoginProviderClaimCondition{Claim: c.Claim, Values: nonNil(c.Values)})
	}
	return out
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func (s *Server) adminLoginProviderResponse(v appidentity.LoginProviderView) AdminLoginProviderJSONResponse {
	cache := cacheNoStore
	return AdminLoginProviderJSONResponse{Body: s.newAdminLoginProvider(v), Headers: AdminLoginProviderResponseHeaders{CacheControl: &cache}}
}

// loginProviderProblem returns the 422 of a provider that breaks a rule, and false for another
// error.
func (s *Server) loginProviderProblem(ctx context.Context, err error) (LoginProviderProblemApplicationProblemPlusJSONResponse, bool) {
	var reason LoginProviderProblemReason
	switch {
	case errors.Is(err, identity.ErrInvalidLoginProviderKind), errors.Is(err, identity.ErrLoginProviderKindChanged):
		reason = LoginProviderProblemReasonKind
	case errors.Is(err, identity.ErrInvalidLoginProviderName):
		reason = LoginProviderProblemReasonDisplayName
	case errors.Is(err, identity.ErrInvalidLoginProviderURL):
		reason = LoginProviderProblemReasonIssuer
	case errors.Is(err, identity.ErrInvalidLoginProviderClientID):
		reason = LoginProviderProblemReasonClientId
	case errors.Is(err, identity.ErrInvalidLoginProviderClientSecret):
		reason = LoginProviderProblemReasonClientSecret
	case errors.Is(err, identity.ErrInvalidLoginProviderScopes):
		reason = LoginProviderProblemReasonScopes
	case errors.Is(err, identity.ErrInvalidLoginProviderConditions):
		reason = LoginProviderProblemReasonConditions
	default:
		return LoginProviderProblemApplicationProblemPlusJSONResponse{}, false
	}
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidLoginProvider,
		l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(string(reason)))
	return LoginProviderProblemApplicationProblemPlusJSONResponse{
		Body: LoginProviderProblem{
			Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
		},
		Headers: LoginProviderProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}, true
}

func loginProviderNotFound(err error) bool { return errors.Is(err, appidentity.ErrNotFound) }

func loginProviderClaims(in []LoginProviderClaimCondition) []identity.ClaimCondition {
	out := make([]identity.ClaimCondition, len(in))
	for i, c := range in {
		out[i] = identity.ClaimCondition{Claim: c.Claim, Values: c.Values}
	}
	return out
}

// ListAdminLoginProviders serves GET /api/v1/admin/login_providers.
func (s *Server) ListAdminLoginProviders(ctx context.Context, _ ListAdminLoginProvidersRequestObject) (ListAdminLoginProvidersResponseObject, error) {
	_, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return ListAdminLoginProvidersdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	list, err := s.o.LoginProviders.ListLoginProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list login providers: %w", err)
	}
	out := AdminLoginProviderList{Items: make([]AdminLoginProvider, 0, len(list))}
	for _, v := range list {
		out.Items = append(out.Items, s.newAdminLoginProvider(v))
	}
	cache := cacheNoStore
	return ListAdminLoginProviders200JSONResponse{Body: out, Headers: ListAdminLoginProviders200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateLoginProvider serves POST /api/v1/admin/login_providers.
func (s *Server) CreateLoginProvider(ctx context.Context, req CreateLoginProviderRequestObject) (CreateLoginProviderResponseObject, error) {
	v, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return CreateLoginProviderdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return CreateLoginProviderdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	b := req.Body
	settings := identity.NewLoginProviderSettings(identity.LoginProviderKind(b.Kind))
	settings.DisplayName, settings.ClientID = b.DisplayName, b.ClientId
	if b.Issuer != nil {
		settings.Issuer = *b.Issuer
	}
	if b.Scopes != nil {
		settings.Scopes = *b.Scopes
	}
	if b.AllowSignup != nil {
		settings.AllowSignup = *b.AllowSignup
	}
	if b.AllowLink != nil {
		settings.AllowLink = *b.AllowLink
	}
	if b.Claims != nil {
		settings.Claims = loginProviderClaims(*b.Claims)
	}
	if b.Organizations != nil {
		settings.Organizations = *b.Organizations
	}
	created, err := s.o.LoginProviders.CreateLoginProvider(ctx, settings, b.ClientSecret, v.UserID)
	if lp, ok := s.loginProviderProblem(ctx, err); ok {
		return CreateLoginProvider422ApplicationProblemPlusJSONResponse{lp}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create login provider: %w", err)
	}
	return CreateLoginProvider200JSONResponse{s.adminLoginProviderResponse(created)}, nil
}

// GetAdminLoginProvider serves GET /api/v1/admin/login_providers/{id}.
func (s *Server) GetAdminLoginProvider(ctx context.Context, req GetAdminLoginProviderRequestObject) (GetAdminLoginProviderResponseObject, error) {
	_, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetAdminLoginProviderdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	got, err := s.o.LoginProviders.LoginProvider(ctx, req.Id)
	switch {
	case loginProviderNotFound(err):
		return GetAdminLoginProviderdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: login provider %s: %w", req.Id, err)
	}
	return GetAdminLoginProvider200JSONResponse{s.adminLoginProviderResponse(got)}, nil
}

// UpdateLoginProvider serves PATCH /api/v1/admin/login_providers/{id}.
func (s *Server) UpdateLoginProvider(ctx context.Context, req UpdateLoginProviderRequestObject) (UpdateLoginProviderResponseObject, error) {
	v, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return UpdateLoginProviderdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return UpdateLoginProviderdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	old, err := s.o.LoginProviders.LoginProvider(ctx, req.Id)
	switch {
	case loginProviderNotFound(err):
		return UpdateLoginProviderdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: login provider %s: %w", req.Id, err)
	}
	b, settings := req.Body, old.Provider.Settings
	if b.DisplayName != nil {
		settings.DisplayName = *b.DisplayName
	}
	if b.Issuer != nil {
		settings.Issuer = *b.Issuer
	}
	if b.ClientId != nil {
		settings.ClientID = *b.ClientId
	}
	if b.Scopes != nil {
		settings.Scopes = *b.Scopes
	}
	if b.AllowSignup != nil {
		settings.AllowSignup = *b.AllowSignup
	}
	if b.AllowLink != nil {
		settings.AllowLink = *b.AllowLink
	}
	if b.Claims != nil {
		settings.Claims = loginProviderClaims(*b.Claims)
	}
	if b.Organizations != nil {
		settings.Organizations = *b.Organizations
	}
	got, err := s.o.LoginProviders.UpdateLoginProvider(ctx, req.Id, settings, v.UserID)
	if lp, ok := s.loginProviderProblem(ctx, err); ok {
		return UpdateLoginProvider422ApplicationProblemPlusJSONResponse{lp}, nil
	}
	switch {
	case loginProviderNotFound(err):
		return UpdateLoginProviderdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: update login provider %s: %w", req.Id, err)
	}
	return UpdateLoginProvider200JSONResponse{s.adminLoginProviderResponse(got)}, nil
}

// DeleteLoginProvider serves DELETE /api/v1/admin/login_providers/{id}.
func (s *Server) DeleteLoginProvider(ctx context.Context, req DeleteLoginProviderRequestObject) (DeleteLoginProviderResponseObject, error) {
	v, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return DeleteLoginProviderdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	_, err = s.o.LoginProviders.DeleteLoginProvider(ctx, req.Id, v.UserID)
	switch {
	case loginProviderNotFound(err):
		return DeleteLoginProviderdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: delete login provider %s: %w", req.Id, err)
	}
	return DeleteLoginProvider204Response{}, nil
}

// EnableLoginProvider serves POST /api/v1/admin/login_providers/{id}/enable.
func (s *Server) EnableLoginProvider(ctx context.Context, req EnableLoginProviderRequestObject) (EnableLoginProviderResponseObject, error) {
	v, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return EnableLoginProviderdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	got, err := s.o.LoginProviders.EnableLoginProvider(ctx, req.Id, v.UserID)
	switch {
	case loginProviderNotFound(err):
		return EnableLoginProviderdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: enable login provider %s: %w", req.Id, err)
	}
	return EnableLoginProvider200JSONResponse{s.adminLoginProviderResponse(got)}, nil
}

// DisableLoginProvider serves POST /api/v1/admin/login_providers/{id}/disable.
func (s *Server) DisableLoginProvider(ctx context.Context, req DisableLoginProviderRequestObject) (DisableLoginProviderResponseObject, error) {
	v, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return DisableLoginProviderdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	got, err := s.o.LoginProviders.DisableLoginProvider(ctx, req.Id, v.UserID)
	switch {
	case loginProviderNotFound(err):
		return DisableLoginProviderdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: disable login provider %s: %w", req.Id, err)
	}
	return DisableLoginProvider200JSONResponse{s.adminLoginProviderResponse(got)}, nil
}

// RotateLoginProviderSecret serves POST /api/v1/admin/login_providers/{id}/rotate_secret.
func (s *Server) RotateLoginProviderSecret(ctx context.Context, req RotateLoginProviderSecretRequestObject) (RotateLoginProviderSecretResponseObject, error) {
	v, p, err := s.requireLoginProviders(ctx)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return RotateLoginProviderSecretdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if req.Body == nil {
		return RotateLoginProviderSecretdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	got, err := s.o.LoginProviders.RotateLoginProviderSecret(ctx, req.Id, req.Body.ClientSecret, v.UserID)
	if lp, ok := s.loginProviderProblem(ctx, err); ok {
		return RotateLoginProviderSecret422ApplicationProblemPlusJSONResponse{lp}, nil
	}
	switch {
	case loginProviderNotFound(err):
		return RotateLoginProviderSecretdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: rotate the secret of login provider %s: %w", req.Id, err)
	}
	return RotateLoginProviderSecret200JSONResponse{s.adminLoginProviderResponse(got)}, nil
}
