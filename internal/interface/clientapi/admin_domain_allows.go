// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	appmoderation "github.com/mycroblog/my/internal/app/moderation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/moderation"
	"github.com/mycroblog/my/internal/i18n"
)

// The allowlist of federation (ADR-0159): the moderators' operations on /admin/domain_allows
// (manage_domains, since removing an allow cuts the domain off as a suspension does), and the
// impact of switching federation_scope to allowlist (manage_settings, like the setting).

// DomainAllows runs the use cases of the allowlist. *appmoderation.Service implements it.
type DomainAllows interface {
	DomainAllows(ctx context.Context) ([]moderation.DomainAllow, error)
	AllowDomain(ctx context.Context, in appmoderation.AllowDomainInput) (moderation.DomainAllow, bool, error)
	RemoveDomainAllow(ctx context.Context, domain, byUserID string) (bool, error)
	FederationScopeImpact(ctx context.Context) (appmoderation.ScopeImpact, error)
}

// requireDomainAllows checks the permission perm and that the use cases are configured; it returns
// the problem to answer otherwise.
func (s *Server) requireDomainAllows(ctx context.Context, perm identity.Permission) (*problem, error) {
	if p, err := s.requirePermission(ctx, perm); err != nil || p != nil {
		return p, err
	}
	if s.o.DomainAllows == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

func newDomainAllow(a moderation.DomainAllow) DomainAllow {
	return DomainAllow{Domain: a.Domain, Comment: a.Comment, CreatedAt: a.CreatedAt.UTC()}
}

// ListDomainAllows serves GET /api/v1/admin/domain_allows.
func (s *Server) ListDomainAllows(ctx context.Context, _ ListDomainAllowsRequestObject) (ListDomainAllowsResponseObject, error) {
	if p, err := s.requireDomainAllows(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return ListDomainAllowsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	allows, err := s.o.DomainAllows.DomainAllows(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list domain allows: %w", err)
	}
	list := DomainAllowList{Items: make([]DomainAllow, len(allows))}
	for i, a := range allows {
		list.Items[i] = newDomainAllow(a)
	}
	cache := cacheNoStore
	return ListDomainAllows200JSONResponse{Body: list, Headers: ListDomainAllows200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateDomainAllow serves POST /api/v1/admin/domain_allows.
func (s *Server) CreateDomainAllow(ctx context.Context, req CreateDomainAllowRequestObject) (CreateDomainAllowResponseObject, error) {
	if p, err := s.requireDomainAllows(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return CreateDomainAllowdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return CreateDomainAllowdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "domain")), nil
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (CreateDomainAllowResponseObject, error) {
		return CreateDomainAllowdefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	domain, err := appfederation.CanonicalHost(req.Body.Domain)
	if err != nil || !moderation.ValidDomain(domain) {
		return CreateDomainAllowdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "domain")), nil
	}
	a, created, err := s.o.DomainAllows.AllowDomain(ctx, appmoderation.AllowDomainInput{Domain: domain, Comment: deref(req.Body.Comment), ByUserID: v.UserID})
	switch {
	case errors.Is(err, moderation.ErrOwnDomain):
		return unprocessable(l.ServerErrorsInvalidValue("domain"))
	case errors.Is(err, appmoderation.ErrTooManyDomainAllows):
		return unprocessable(l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: moderation.MaxDomainAllows, Name: "domains"}))
	case errors.Is(err, appmoderation.ErrInvalidDomainAllow):
		return unprocessable(l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: moderation.MaxDomainAllowComment, Name: "comment"}))
	case err != nil:
		return nil, fmt.Errorf("clientapi: allow domain %s: %w", domain, err)
	}
	cache := cacheNoStore
	if !created {
		return CreateDomainAllow200JSONResponse{Body: newDomainAllow(a), Headers: CreateDomainAllow200ResponseHeaders{CacheControl: &cache}}, nil
	}
	return CreateDomainAllow201JSONResponse{Body: newDomainAllow(a), Headers: CreateDomainAllow201ResponseHeaders{CacheControl: &cache}}, nil
}

// RemoveDomainAllow serves DELETE /api/v1/admin/domain_allows/{domain}.
func (s *Server) RemoveDomainAllow(ctx context.Context, req RemoveDomainAllowRequestObject) (RemoveDomainAllowResponseObject, error) {
	if p, err := s.requireDomainAllows(ctx, identity.PermManageDomains); err != nil {
		return nil, err
	} else if p != nil {
		return RemoveDomainAllowdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	domain, err := appfederation.CanonicalHost(req.Domain)
	if err != nil {
		return RemoveDomainAllow204Response{}, nil // A domain that cannot be allowed has no allow.
	}
	if _, err := s.o.DomainAllows.RemoveDomainAllow(ctx, domain, v.UserID); err != nil {
		return nil, fmt.Errorf("clientapi: remove the allow of %s: %w", domain, err)
	}
	return RemoveDomainAllow204Response{}, nil
}

// GetFederationScopeImpact serves GET /api/v1/admin/settings/federation_scope/impact.
func (s *Server) GetFederationScopeImpact(ctx context.Context, _ GetFederationScopeImpactRequestObject) (GetFederationScopeImpactResponseObject, error) {
	if p, err := s.requireDomainAllows(ctx, identity.PermManageSettings); err != nil {
		return nil, err
	} else if p != nil {
		return GetFederationScopeImpactdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	impact, err := s.o.DomainAllows.FederationScopeImpact(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: count what the allowlist would end: %w", err)
	}
	body := FederationScopeImpact{
		Scope: FederationScopeImpactScope(impact.Scope), Follows: impact.Follows, LocalAccounts: impact.LocalAccounts,
		Domains: impact.Domains, TopDomains: make([]FederationScopeImpactDomain, len(impact.TopDomains)),
	}
	for i, d := range impact.TopDomains {
		body.TopDomains[i] = FederationScopeImpactDomain{Domain: d.Domain, Follows: d.Follows}
	}
	cache := cacheNoStore
	return GetFederationScopeImpact200JSONResponse{Body: body, Headers: GetFederationScopeImpact200ResponseHeaders{CacheControl: &cache}}, nil
}
