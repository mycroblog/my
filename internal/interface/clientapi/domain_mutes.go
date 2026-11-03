// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"net/http"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	appgraph "github.com/mycroblog/my/internal/app/graph"
	"github.com/mycroblog/my/internal/domain/graph"
)

// ProblemTypeTooManyDomainMutes is the problem of muting a domain beyond graph.MaxDomainMutes.
const ProblemTypeTooManyDomainMutes = "/api/v1/problems/too-many-domain-mutes"

// The domain mutes of the viewer (FR-304). A page of them is ordered by domain, and its cursor
// is the base64url of the last domain.

func newDomainMute(m graph.DomainMute) DomainMute {
	return DomainMute{Domain: m.Domain, CreatedAt: m.CreatedAt}
}

// ListDomainMutes serves GET /api/v1/domain_mutes.
func (s *Server) ListDomainMutes(ctx context.Context, req ListDomainMutesRequestObject) (ListDomainMutesResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	q := s.parseOpaquePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListDomainMutesdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	mutes, err := s.o.Graph.DomainMutes(ctx, v.AccountID, appgraph.DomainMutePage{After: q.before, Limit: q.limit + 1})
	if err != nil {
		return nil, err
	}
	mutes, last := trim(mutes, q.limit)
	page := DomainMutePage{Items: make([]DomainMute, len(mutes))}
	for i, m := range mutes {
		page.Items[i] = newDomainMute(m)
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(mutes[last].Domain)
	}
	cache := cacheNoStore
	return ListDomainMutes200JSONResponse{Body: page, Headers: ListDomainMutes200ResponseHeaders{CacheControl: &cache}}, nil
}

// MuteDomain serves POST /api/v1/domain_mutes.
func (s *Server) MuteDomain(ctx context.Context, req MuteDomainRequestObject) (MuteDomainResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	invalid := MuteDomaindefaultApplicationProblemPlusJSONResponse(
		s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue("domain")))
	if req.Body == nil {
		return invalid, nil
	}
	domain, err := appfederation.CanonicalHost(req.Body.Domain)
	if err != nil {
		return invalid, nil
	}
	m, err := s.o.Graph.MuteDomain(ctx, v.AccountID, domain)
	switch {
	case errors.Is(err, graph.ErrInvalidDomain):
		return invalid, nil
	case errors.Is(err, graph.ErrTooManyDomainMutes):
		return MuteDomaindefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeTooManyDomainMutes, l.ServerErrorsDomainMutesTooMany(), "")), nil
	case err != nil:
		if p, ok := s.graphProblem(ctx, err); ok {
			return MuteDomaindefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, err
	}
	cache := cacheNoStore
	return MuteDomain200JSONResponse{Body: newDomainMute(m), Headers: MuteDomain200ResponseHeaders{CacheControl: &cache}}, nil
}

// UnmuteDomain serves DELETE /api/v1/domain_mutes/{domain}.
func (s *Server) UnmuteDomain(ctx context.Context, req UnmuteDomainRequestObject) (UnmuteDomainResponseObject, error) {
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	domain, err := appfederation.CanonicalHost(req.Domain)
	if err != nil {
		return UnmuteDomain204Response{}, nil // a domain that is not one is not muted
	}
	if err := s.o.Graph.UnmuteDomain(ctx, v.AccountID, domain); err != nil {
		return nil, err
	}
	return UnmuteDomain204Response{}, nil
}
