// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	appidentity "github.com/mycroblog/my/internal/app/identity"
)

// The viewer's accounts at external login providers (FR-010): listing and unlinking them,
// starting a link once the viewer confirmed it, and starting a confirmation with a linked
// provider, which the operations on the viewer's own account take in place of the password
// (external_confirmation). The browser finishes both at the provider through the
// server-rendered start and callback (authpages).

// Problem types of linked identities.
const (
	// ProblemTypeExternalConfirmationExpired is the problem of an unknown, expired, used, or
	// another operation's confirmation with a linked login provider.
	ProblemTypeExternalConfirmationExpired = "/api/v1/problems/external-confirmation-expired"
	// ProblemTypeLoginProviderUnavailable is the problem of a provider that is unknown,
	// disabled, does not allow links, or that the viewer links no account at.
	ProblemTypeLoginProviderUnavailable = "/api/v1/problems/login-provider-unavailable"
	// ProblemTypeLoginProviderLinked is the problem of a link to a provider that the viewer
	// links an account at already.
	ProblemTypeLoginProviderLinked = "/api/v1/problems/login-provider-linked"
)

// LinkedIdentitySettings runs the viewer's linked identities. *appidentity.Service implements
// it.
type LinkedIdentitySettings interface {
	LinkedIdentities(ctx context.Context, userID string) (appidentity.LinkedIdentities, error)
	UnlinkExternalIdentity(ctx context.Context, userID, linkID string) error
	BeginExternalLink(ctx context.Context, userID, providerID string, c appidentity.Confirmation, returnTo string) (appidentity.ExternalIntent, error)
	BeginExternalConfirmation(ctx context.Context, userID, providerID string, op appidentity.ConfirmedOperation, returnTo string) (appidentity.ExternalIntent, error)
}

// linkedIdentityProblem returns the problem of an error of the identity module's link use
// cases, or nil for another error.
func (s *Server) linkedIdentityProblem(ctx context.Context, err error) *problem {
	if p := s.passkeyProblem(ctx, err); p != nil {
		return p
	}
	l := s.Localizer(ctx)
	var p problem
	switch {
	case errors.Is(err, appidentity.ErrLoginProviderUnavailable), errors.Is(err, appidentity.ErrLoginProviderNotLinked):
		p = s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeLoginProviderUnavailable, l.ServerErrorsLoginProviderUnavailable(), "")
	case errors.Is(err, appidentity.ErrLoginProviderLinked):
		p = s.problem(ctx, http.StatusConflict, ProblemTypeLoginProviderLinked, l.ServerErrorsLoginProviderLinked(), "")
	default:
		return nil
	}
	return &p
}

// returnToOf returns the return target of a request: a path on this site, or "" for the
// default. The start and the callback check it again before they redirect to it.
func returnToOf(p *ReturnTo) string {
	if p == nil || !strings.HasPrefix(*p, "/") || strings.HasPrefix(*p, "//") || strings.HasPrefix(*p, "/\\") {
		return ""
	}
	return *p
}

// ListLinkedIdentities serves GET /api/v1/linked_identities.
func (s *Server) ListLinkedIdentities(ctx context.Context, _ ListLinkedIdentitiesRequestObject) (ListLinkedIdentitiesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.LinkedIdentities == nil {
		return ListLinkedIdentitiesdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	list, err := s.o.LinkedIdentities.LinkedIdentities(ctx, v.UserID)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the linked identities of %s: %w", v.UserID, err)
	}
	body := LinkedIdentityList{Items: make([]LinkedIdentity, len(list.Links)), Providers: make([]LinkableLoginProvider, len(list.Providers))}
	for i, l := range list.Links {
		body.Items[i] = LinkedIdentity{
			Id: l.LinkID, ProviderId: l.ProviderID, ProviderKind: LoginProviderKind(l.ProviderKind),
			ProviderName: l.ProviderName, LinkedAt: l.LinkedAt.UTC(), Available: l.Available,
		}
	}
	for i, p := range list.Providers {
		body.Providers[i] = LinkableLoginProvider{
			Id: p.ID, Kind: LoginProviderKind(p.Kind), DisplayName: p.DisplayName, AllowLink: p.AllowLink,
		}
	}
	cache := cacheNoStore
	return ListLinkedIdentities200JSONResponse{Body: body, Headers: ListLinkedIdentities200ResponseHeaders{CacheControl: &cache}}, nil
}

// BeginLinkIdentity serves POST /api/v1/linked_identities.
func (s *Server) BeginLinkIdentity(ctx context.Context, req BeginLinkIdentityRequestObject) (BeginLinkIdentityResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.LinkedIdentities == nil {
		return BeginLinkIdentitydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil || req.Body.ProviderId == "" {
		return BeginLinkIdentitydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "provider_id")), nil
	}
	c, param, ok := confirmationOf(req.Body.Password, req.Body.Passkey, req.Body.ExternalConfirmation, "password")
	if !ok {
		return BeginLinkIdentitydefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, param)), nil
	}
	intent, err := s.o.LinkedIdentities.BeginExternalLink(ctx, v.UserID, req.Body.ProviderId, c, returnToOf(req.Body.ReturnTo))
	if p := s.linkedIdentityProblem(ctx, err); p != nil {
		return BeginLinkIdentitydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: begin a link of %s: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return BeginLinkIdentity200JSONResponse{
		Body:    ExternalAuthorizationStart{StartUrl: intent.StartURL, ExpiresAt: intent.ExpiresAt.UTC()},
		Headers: BeginLinkIdentity200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// BeginExternalConfirmation serves POST /api/v1/linked_identities/confirmation.
func (s *Server) BeginExternalConfirmation(ctx context.Context, req BeginExternalConfirmationRequestObject) (BeginExternalConfirmationResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.LinkedIdentities == nil {
		return BeginExternalConfirmationdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if req.Body == nil || req.Body.ProviderId == "" {
		return BeginExternalConfirmationdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "provider_id")), nil
	}
	op := appidentity.ConfirmedOperation(req.Body.Operation)
	if !op.Valid() {
		return BeginExternalConfirmationdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "operation")), nil
	}
	intent, err := s.o.LinkedIdentities.BeginExternalConfirmation(ctx, v.UserID, req.Body.ProviderId, op, returnToOf(req.Body.ReturnTo))
	if p := s.linkedIdentityProblem(ctx, err); p != nil {
		return BeginExternalConfirmationdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: begin a confirmation of %s with a login provider: %w", v.UserID, err)
	}
	cache := cacheNoStore
	return BeginExternalConfirmation200JSONResponse{
		Body:    ExternalAuthorizationStart{StartUrl: intent.StartURL, ExpiresAt: intent.ExpiresAt.UTC()},
		Headers: BeginExternalConfirmation200ResponseHeaders{CacheControl: &cache},
	}, nil
}

// UnlinkIdentity serves DELETE /api/v1/linked_identities/{id}.
func (s *Server) UnlinkIdentity(ctx context.Context, req UnlinkIdentityRequestObject) (UnlinkIdentityResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if s.o.LinkedIdentities == nil {
		return UnlinkIdentitydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	err = s.o.LinkedIdentities.UnlinkExternalIdentity(ctx, v.UserID, req.Id)
	if p := s.linkedIdentityProblem(ctx, err); p != nil {
		return UnlinkIdentitydefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	switch {
	case errors.Is(err, appidentity.ErrNotFound):
		return UnlinkIdentitydefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	case err != nil:
		return nil, fmt.Errorf("clientapi: unlink %s of %s: %w", req.Id, v.UserID, err)
	}
	return UnlinkIdentity204Response{}, nil
}
