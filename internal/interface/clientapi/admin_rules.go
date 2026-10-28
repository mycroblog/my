// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
)

// The instance rules (FR-705): the admin operations that manage them (manage_rules), and the
// rules in effect that GET /instance lists. A rule's text and hint hold one value each, shown
// as they are in every locale.

// InstanceRules runs the use cases of the instance rules. *appinstance.Service implements it.
type InstanceRules interface {
	Rules(ctx context.Context, includeRetired bool) ([]instance.Rule, error)
	Rule(ctx context.Context, id string) (instance.Rule, error)
	AddRule(ctx context.Context, in appinstance.AddRuleInput) (instance.Rule, error)
	ReviseRule(ctx context.Context, in appinstance.ReviseRuleInput) (instance.Rule, error)
	MoveRule(ctx context.Context, in appinstance.MoveRuleInput) (instance.Rule, error)
	RetireRule(ctx context.Context, ruleID, retiredByUserID string) (instance.Rule, error)
}

// instanceRules returns the rules in effect; none without Options.Rules.
func (s *Server) instanceRules(ctx context.Context) ([]InstanceRule, error) {
	out := []InstanceRule{}
	if s.o.Rules == nil {
		return out, nil
	}
	rules, err := s.o.Rules.Rules(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("clientapi: instance rules: %w", err)
	}
	for _, r := range rules {
		out = append(out, InstanceRule{Id: r.ID, Text: r.Revision.Text, Hint: optionalString(r.Revision.Hint)})
	}
	return out, nil
}

func newAdminRule(r instance.Rule) AdminRule {
	out := AdminRule{
		Id: r.ID, Position: r.Revision.Position, Text: r.Revision.Text, Hint: optionalString(r.Revision.Hint),
		CreatedAt: r.CreatedAt.UTC(), RevisedAt: r.Revision.RevisedAt.UTC(),
	}
	if r.Retirement != nil {
		t := r.Retirement.RetiredAt.UTC()
		out.RetiredAt = &t
	}
	return out
}

func adminRuleResponse(r instance.Rule) AdminRuleJSONResponse {
	cache := cacheNoStore
	return AdminRuleJSONResponse{Body: newAdminRule(r), Headers: AdminRuleResponseHeaders{CacheControl: &cache}}
}

// ruleHint returns the hint of in, "" when it has none.
func ruleHint(in InstanceRuleInput) string {
	if in.Hint == nil {
		return ""
	}
	return *in.Hint
}

// ruleProblem maps the errors of the rule use cases to problems: 404 for an unknown rule, 409
// for a retired one, and 422 for bad input. It reports false for any other error.
func (s *Server) ruleProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	unprocessable := func(detail string) (problem, bool) {
		return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), detail), true
	}
	var te *instance.RuleTextError
	switch {
	case errors.Is(err, appinstance.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, instance.ErrRuleRetired):
		return s.problem(ctx, http.StatusConflict, ProblemTypeBlank, l.ServerErrorsConflict(), ""), true
	case errors.Is(err, instance.ErrTooManyRules):
		return unprocessable(l.ServerErrorsTooMany(i18n.ServerErrorsTooManyArgs{Max: instance.MaxRules, Name: "rules"}))
	case errors.As(err, &te) && te.Max > 0:
		return unprocessable(l.ServerErrorsTooLong(i18n.ServerErrorsTooLongArgs{Max: te.Max, Name: te.Field}))
	case errors.As(err, &te):
		return unprocessable(l.ServerErrorsInvalidValue(te.Field))
	case errors.Is(err, instance.ErrInvalidValue):
		return unprocessable(l.ServerErrorsInvalidValue("position"))
	}
	return problem{}, false
}

// ListInstanceRules serves GET /api/v1/admin/rules.
func (s *Server) ListInstanceRules(ctx context.Context, req ListInstanceRulesRequestObject) (ListInstanceRulesResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRules); err != nil {
		return nil, err
	} else if p != nil {
		return ListInstanceRulesdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	rules, err := s.o.Rules.Rules(ctx, req.Params.IncludeRetired != nil && *req.Params.IncludeRetired)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list rules: %w", err)
	}
	list := AdminRuleList{Items: make([]AdminRule, 0, len(rules))}
	for _, r := range rules {
		list.Items = append(list.Items, newAdminRule(r))
	}
	cache := cacheNoStore
	return ListInstanceRules200JSONResponse{Body: list, Headers: ListInstanceRules200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateInstanceRule serves POST /api/v1/admin/rules.
func (s *Server) CreateInstanceRule(ctx context.Context, req CreateInstanceRuleRequestObject) (CreateInstanceRuleResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRules); err != nil {
		return nil, err
	} else if p != nil {
		return CreateInstanceRuledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return CreateInstanceRuledefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "text")), nil
	}
	r, err := s.o.Rules.AddRule(ctx, appinstance.AddRuleInput{Text: req.Body.Text, Hint: ruleHint(*req.Body), CreatedByUserID: v.UserID})
	if p, ok := s.ruleProblem(ctx, err); ok {
		return CreateInstanceRuledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: add a rule: %w", err)
	}
	return CreateInstanceRule200JSONResponse{adminRuleResponse(r)}, nil
}

// GetInstanceRule serves GET /api/v1/admin/rules/{id}.
func (s *Server) GetInstanceRule(ctx context.Context, req GetInstanceRuleRequestObject) (GetInstanceRuleResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRules); err != nil {
		return nil, err
	} else if p != nil {
		return GetInstanceRuledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	r, err := s.o.Rules.Rule(ctx, req.Id)
	if p, ok := s.ruleProblem(ctx, err); ok {
		return GetInstanceRuledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: rule %s: %w", req.Id, err)
	}
	return GetInstanceRule200JSONResponse{adminRuleResponse(r)}, nil
}

// UpdateInstanceRule serves PATCH /api/v1/admin/rules/{id}.
func (s *Server) UpdateInstanceRule(ctx context.Context, req UpdateInstanceRuleRequestObject) (UpdateInstanceRuleResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRules); err != nil {
		return nil, err
	} else if p != nil {
		return UpdateInstanceRuledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return UpdateInstanceRuledefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "text")), nil
	}
	r, err := s.o.Rules.ReviseRule(ctx, appinstance.ReviseRuleInput{RuleID: req.Id, Text: req.Body.Text, Hint: ruleHint(*req.Body), RevisedByUserID: v.UserID})
	if p, ok := s.ruleProblem(ctx, err); ok {
		return UpdateInstanceRuledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: revise rule %s: %w", req.Id, err)
	}
	return UpdateInstanceRule200JSONResponse{adminRuleResponse(r)}, nil
}

// MoveInstanceRule serves POST /api/v1/admin/rules/{id}/move.
func (s *Server) MoveInstanceRule(ctx context.Context, req MoveInstanceRuleRequestObject) (MoveInstanceRuleResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRules); err != nil {
		return nil, err
	} else if p != nil {
		return MoveInstanceRuledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return MoveInstanceRuledefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "position")), nil
	}
	r, err := s.o.Rules.MoveRule(ctx, appinstance.MoveRuleInput{RuleID: req.Id, Position: req.Body.Position, RevisedByUserID: v.UserID})
	if p, ok := s.ruleProblem(ctx, err); ok {
		return MoveInstanceRuledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: move rule %s: %w", req.Id, err)
	}
	return MoveInstanceRule200JSONResponse{adminRuleResponse(r)}, nil
}

// RetireInstanceRule serves POST /api/v1/admin/rules/{id}/retire.
func (s *Server) RetireInstanceRule(ctx context.Context, req RetireInstanceRuleRequestObject) (RetireInstanceRuleResponseObject, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageRules); err != nil {
		return nil, err
	} else if p != nil {
		return RetireInstanceRuledefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.o.Rules.RetireRule(ctx, req.Id, v.UserID)
	if p, ok := s.ruleProblem(ctx, err); ok {
		return RetireInstanceRuledefaultApplicationProblemPlusJSONResponse(p), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: retire rule %s: %w", req.Id, err)
	}
	return RetireInstanceRule200JSONResponse{adminRuleResponse(r)}, nil
}
