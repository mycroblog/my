// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// The rules of the link card crawler (FR-117): the admin operations on /admin/link_card_domains
// (manage_settings, like the link_cards setting that they refine).

// LinkCardDomainRules runs the use cases of the rules. *appinstance.Service implements it.
type LinkCardDomainRules interface {
	LinkCardDomainRules(ctx context.Context) ([]instance.LinkCardDomainRule, error)
	SetLinkCardDomainRule(ctx context.Context, r instance.LinkCardDomainRule, byUserID string) (instance.LinkCardDomainRule, error)
	RemoveLinkCardDomainRule(ctx context.Context, domain, byUserID string) error
}

// requireLinkCardDomainRules checks the manage_settings permission and that the use cases are
// configured; it returns the problem to answer otherwise.
func (s *Server) requireLinkCardDomainRules(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSettings); err != nil || p != nil {
		return p, err
	}
	if s.o.LinkCardDomainRules == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

func newLinkCardDomainRule(r instance.LinkCardDomainRule) LinkCardDomainRule {
	return LinkCardDomainRule{Domain: r.Domain, Mode: LinkCardDomainMode(r.Mode), Note: r.Note, UpdatedAt: r.UpdatedAt.UTC()}
}

// ListLinkCardDomainRules serves GET /api/v1/admin/link_card_domains.
func (s *Server) ListLinkCardDomainRules(ctx context.Context, _ ListLinkCardDomainRulesRequestObject) (ListLinkCardDomainRulesResponseObject, error) {
	if p, err := s.requireLinkCardDomainRules(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return ListLinkCardDomainRulesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	rules, err := s.o.LinkCardDomainRules.LinkCardDomainRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list link card domain rules: %w", err)
	}
	list := LinkCardDomainRuleList{Items: make([]LinkCardDomainRule, len(rules))}
	for i, r := range rules {
		list.Items[i] = newLinkCardDomainRule(r)
	}
	cache := cacheNoStore
	return ListLinkCardDomainRules200JSONResponse{Body: list, Headers: ListLinkCardDomainRules200ResponseHeaders{CacheControl: &cache}}, nil
}

// SetLinkCardDomainRule serves POST /api/v1/admin/link_card_domains.
func (s *Server) SetLinkCardDomainRule(ctx context.Context, req SetLinkCardDomainRuleRequestObject) (SetLinkCardDomainRuleResponseObject, error) {
	if p, err := s.requireLinkCardDomainRules(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return SetLinkCardDomainRuledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (SetLinkCardDomainRuleResponseObject, error) {
		return SetLinkCardDomainRuledefaultApplicationProblemPlusJSONResponse(
			s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail)), nil
	}
	if req.Body == nil {
		return SetLinkCardDomainRuledefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "domain")), nil
	}
	domain, err := appfederation.CanonicalHost(req.Body.Domain)
	if err != nil {
		return unprocessable(l.ServerErrorsInvalidValue("domain"))
	}
	saved, err := s.o.LinkCardDomainRules.SetLinkCardDomainRule(ctx, instance.LinkCardDomainRule{
		Domain: domain, Mode: instance.LinkCardDomainMode(req.Body.Mode), Note: deref(req.Body.Note),
	}, v.UserID)
	var invalid *instance.LinkCardDomainError
	switch {
	case errors.Is(err, instance.ErrTooManyLinkCardDomainRules):
		return unprocessable(l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: instance.MaxLinkCardDomainRules, Name: "rules"}))
	case errors.As(err, &invalid) && invalid.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: invalid.Max, Name: invalid.Field}))
	case errors.As(err, &invalid):
		return unprocessable(l.ServerErrorsInvalidValue(invalid.Field))
	case err != nil:
		return nil, fmt.Errorf("clientapi: set a link card domain rule: %w", err)
	}
	cache := cacheNoStore
	return SetLinkCardDomainRule200JSONResponse{Body: newLinkCardDomainRule(saved), Headers: SetLinkCardDomainRule200ResponseHeaders{CacheControl: &cache}}, nil
}

// RemoveLinkCardDomainRule serves DELETE /api/v1/admin/link_card_domains/{domain}.
func (s *Server) RemoveLinkCardDomainRule(ctx context.Context, req RemoveLinkCardDomainRuleRequestObject) (RemoveLinkCardDomainRuleResponseObject, error) {
	if p, err := s.requireLinkCardDomainRules(ctx); err != nil {
		return nil, err
	} else if p != nil {
		return RemoveLinkCardDomainRuledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	domain, err := appfederation.CanonicalHost(req.Domain)
	if err != nil {
		return RemoveLinkCardDomainRule204Response{}, nil // A domain that cannot have a rule has none.
	}
	if err := s.o.LinkCardDomainRules.RemoveLinkCardDomainRule(ctx, domain, v.UserID); err != nil {
		return nil, fmt.Errorf("clientapi: remove a link card domain rule: %w", err)
	}
	return RemoveLinkCardDomainRule204Response{}, nil
}
