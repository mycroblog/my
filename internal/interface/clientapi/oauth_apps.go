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

// The OAuth apps that the viewer registers (ADR-0090 §1, §2): registering, listing, changing,
// and deleting them, new secrets of confidential apps, and tokens of an app for the viewer's
// own account. They are account security operations, which only the first-party clients call
// (x-first-party-only).

// Problem types of the viewer's OAuth apps.
const (
	// ProblemTypeTooManyOAuthApps is the problem of a registration by a user with the most apps.
	ProblemTypeTooManyOAuthApps = "/api/v1/problems/too-many-oauth-apps"
	// ProblemTypeOAuthAppRegistrationDisabled is the problem of a registration while the
	// instance does not let users register apps.
	ProblemTypeOAuthAppRegistrationDisabled = "/api/v1/problems/oauth-app-registration-disabled"
	// ProblemTypePublicOAuthApp is the problem of a new secret for a public app.
	ProblemTypePublicOAuthApp = "/api/v1/problems/public-oauth-app"
	// ProblemTypeOAuthAppDisabled is the problem of tokens of an app that an administrator
	// disabled.
	ProblemTypeOAuthAppDisabled = "/api/v1/problems/oauth-app-disabled"
)

// OAuthAppSettings runs the viewer's OAuth apps. *appidentity.Service implements it.
type OAuthAppSettings interface {
	OAuthApps(ctx context.Context, userID string) ([]appidentity.OAuthApp, error)
	OAuthApp(ctx context.Context, userID, id string) (appidentity.OAuthApp, error)
	RegisterOAuthApp(ctx context.Context, userID string, in appidentity.OAuthAppInput) (appidentity.RegisteredOAuthApp, error)
	UpdateOAuthApp(ctx context.Context, userID, id string, ch appidentity.OAuthAppChanges) (appidentity.OAuthApp, error)
	DeleteOwnOAuthApp(ctx context.Context, userID, id string) error
	RotateOAuthAppSecret(ctx context.Context, userID, id string) (string, error)
	IssueOAuthAppTokens(ctx context.Context, userID, id, scope string) (appidentity.IssuedTokens, error)
}

func newOAuthApp(a appidentity.OAuthApp) OAuthApp {
	c := a.Client
	out := OAuthApp{
		Id: c.ID, ClientId: c.ClientID, Name: c.Name, RedirectUris: a.RedirectURIs, Scope: string(c.Scope),
		ClientType: OAuthAppClientTypeConfidential, Disabled: a.Disabled, CreatedAt: c.CreatedAt.UTC(),
	}
	if out.RedirectUris == nil {
		out.RedirectUris = []string{}
	}
	if c.Public() {
		out.ClientType = OAuthAppClientTypePublic
	}
	if c.Website != "" {
		w := c.Website
		out.Website = &w
	}
	return out
}

func oauthAppResponse(a appidentity.OAuthApp) OAuthAppJSONResponse {
	cache := cacheNoStore
	return OAuthAppJSONResponse{Body: newOAuthApp(a), Headers: OAuthAppResponseHeaders{CacheControl: &cache}}
}

// oauthAppProblem returns the problem of an error of the OAuth app use cases, or nil for an
// unexpected error.
func (s *Server) oauthAppProblem(ctx context.Context, err error) *problem {
	l := s.Localizer(ctx)
	invalid := func(name string) *problem {
		p := s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(name))
		return &p
	}
	var oauthErr *appidentity.OAuthError
	var p problem
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		p = s.notFoundResponse(ctx)
	case errors.Is(err, identity.ErrInvalidOAuthClientName):
		return invalid("name")
	case errors.Is(err, identity.ErrInvalidOAuthClientWebsite):
		return invalid("website")
	case errors.Is(err, identity.ErrInvalidOAuthClientRedirectURIs):
		return invalid("redirect_uris")
	case errors.Is(err, identity.ErrInvalidOAuthClientScope),
		errors.As(err, &oauthErr) && oauthErr.Code == appidentity.OAuthInvalidScope:
		return invalid("scope")
	case errors.Is(err, appidentity.ErrTooManyOAuthApps):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeTooManyOAuthApps, l.ServerErrorsOauthAppTooMany(), "")
	case errors.Is(err, appidentity.ErrOAuthAppRegistrationDisabled):
		p = s.problem(ctx, http.StatusForbidden, ProblemTypeOAuthAppRegistrationDisabled, l.ServerErrorsOauthAppRegistrationDisabled(), "")
	case errors.Is(err, appidentity.ErrPublicOAuthApp):
		p = s.problem(ctx, http.StatusConflict, ProblemTypePublicOAuthApp, l.ServerErrorsOauthAppPublic(), "")
	case errors.Is(err, appidentity.ErrOAuthAppDisabled):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeOAuthAppDisabled, l.ServerErrorsOauthAppDisabled(), "")
	default:
		return nil
	}
	return &p
}

// ListOAuthApps serves GET /api/v1/oauth_apps.
func (s *Server) ListOAuthApps(ctx context.Context, _ ListOAuthAppsRequestObject) (ListOAuthAppsResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OAuthApps == nil {
		return ListOAuthAppsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	apps, err := s.o.OAuthApps.OAuthApps(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the OAuth apps of %s: %w", v.UserID, err)
	}
	items := make([]OAuthApp, len(apps))
	for i, a := range apps {
		items[i] = newOAuthApp(a)
	}
	cache := cacheNoStore
	return ListOAuthApps200JSONResponse{Body: OAuthAppList{Items: items}, Headers: ListOAuthApps200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateOAuthApp serves POST /api/v1/oauth_apps.
func (s *Server) CreateOAuthApp(ctx context.Context, req CreateOAuthAppRequestObject) (CreateOAuthAppResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OAuthApps == nil {
		return CreateOAuthAppdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return CreateOAuthAppdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	if !req.Body.ClientType.Valid() {
		return CreateOAuthAppdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "client_type")), nil
	}
	if p, h, limited := s.LimitProblem(ctx, OAuthAppRegisterLimit, v.UserID); limited {
		return CreateOAuthAppdefaultApplicationProblemPlusJSONResponse{Body: p, Headers: h, StatusCode: http.StatusTooManyRequests}, nil
	}
	in := appidentity.OAuthAppInput{
		Name: req.Body.Name, RedirectURIs: req.Body.RedirectUris, Scope: req.Body.Scope,
		Confidential: req.Body.ClientType == OAuthAppClientTypeConfidential,
	}
	if w := req.Body.Website; w != nil {
		in.Website = *w
	}
	reg, err := s.o.OAuthApps.RegisterOAuthApp(ctx, v.UserID, in)
	if p := s.oauthAppProblem(ctx, err); p != nil {
		return CreateOAuthAppdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: register an OAuth app of %s: %w", v.UserID, err)
	}
	body := OAuthAppCreated{App: newOAuthApp(reg.OAuthApp)}
	if reg.ClientSecret != "" {
		secret := reg.ClientSecret
		body.ClientSecret = &secret
	}
	cache := cacheNoStore
	return CreateOAuthApp201JSONResponse{Body: body, Headers: CreateOAuthApp201ResponseHeaders{CacheControl: &cache}}, nil
}

// GetOAuthApp serves GET /api/v1/oauth_apps/{id}.
func (s *Server) GetOAuthApp(ctx context.Context, req GetOAuthAppRequestObject) (GetOAuthAppResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OAuthApps == nil {
		return GetOAuthAppdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	a, err := s.o.OAuthApps.OAuthApp(ctx, v.UserID, req.Id)
	if p := s.oauthAppProblem(ctx, err); p != nil {
		return GetOAuthAppdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: OAuth app %s: %w", req.Id, err)
	}
	return GetOAuthApp200JSONResponse{oauthAppResponse(a)}, nil
}

// UpdateOAuthApp serves PATCH /api/v1/oauth_apps/{id}.
func (s *Server) UpdateOAuthApp(ctx context.Context, req UpdateOAuthAppRequestObject) (UpdateOAuthAppResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OAuthApps == nil {
		return UpdateOAuthAppdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil {
		return UpdateOAuthAppdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	ch := appidentity.OAuthAppChanges{Name: req.Body.Name, Website: req.Body.Website, Scope: req.Body.Scope}
	if uris := req.Body.RedirectUris; uris != nil {
		ch.RedirectURIs = *uris
		if ch.RedirectURIs == nil {
			ch.RedirectURIs = []string{}
		}
	}
	a, err := s.o.OAuthApps.UpdateOAuthApp(ctx, v.UserID, req.Id, ch)
	if p := s.oauthAppProblem(ctx, err); p != nil {
		return UpdateOAuthAppdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update OAuth app %s: %w", req.Id, err)
	}
	return UpdateOAuthApp200JSONResponse{oauthAppResponse(a)}, nil
}

// DeleteOAuthApp serves DELETE /api/v1/oauth_apps/{id}.
func (s *Server) DeleteOAuthApp(ctx context.Context, req DeleteOAuthAppRequestObject) (DeleteOAuthAppResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OAuthApps == nil {
		return DeleteOAuthAppdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	err = s.o.OAuthApps.DeleteOwnOAuthApp(ctx, v.UserID, req.Id)
	if p := s.oauthAppProblem(ctx, err); p != nil {
		return DeleteOAuthAppdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: delete OAuth app %s: %w", req.Id, err)
	}
	return DeleteOAuthApp204Response{}, nil
}

// RotateOAuthAppSecret serves POST /api/v1/oauth_apps/{id}/secret.
func (s *Server) RotateOAuthAppSecret(ctx context.Context, req RotateOAuthAppSecretRequestObject) (RotateOAuthAppSecretResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OAuthApps == nil {
		return RotateOAuthAppSecretdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	secret, err := s.o.OAuthApps.RotateOAuthAppSecret(ctx, v.UserID, req.Id)
	if p := s.oauthAppProblem(ctx, err); p != nil {
		return RotateOAuthAppSecretdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: rotate the secret of OAuth app %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	return RotateOAuthAppSecret200JSONResponse{
		Body: OAuthAppSecret{ClientSecret: secret}, Headers: RotateOAuthAppSecret200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// IssueOAuthAppToken serves POST /api/v1/oauth_apps/{id}/tokens.
func (s *Server) IssueOAuthAppToken(ctx context.Context, req IssueOAuthAppTokenRequestObject) (IssueOAuthAppTokenResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.OAuthApps == nil {
		return IssueOAuthAppTokendefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	var scope string
	if req.Body != nil && req.Body.Scope != nil {
		scope = *req.Body.Scope
	}
	issued, err := s.o.OAuthApps.IssueOAuthAppTokens(ctx, v.UserID, req.Id, scope)
	if p := s.oauthAppProblem(ctx, err); p != nil {
		return IssueOAuthAppTokendefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: issue tokens of OAuth app %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	body := OAuthAppToken{
		AccessToken: issued.AccessToken, TokenType: OAuthAppTokenTokenTypeBearer, RefreshToken: issued.RefreshToken,
		ExpiresIn: int(issued.Token.ExpiresAt.Sub(issued.Token.IssuedAt).Seconds()), Scope: string(issued.Token.Scope),
	}
	return IssueOAuthAppToken200JSONResponse{Body: body, Headers: IssueOAuthAppToken200ResponseHeaders{CacheControl: &cache}}, nil
}
