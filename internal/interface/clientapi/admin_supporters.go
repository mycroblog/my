// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	appidentity "github.com/mycroblog/my/internal/app/identity"
	appsupporters "github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/supporters"
)

// The supporter program (ADR-0072 §2, §3, §9, §10, §18): the admin operations under
// /admin/supporter_* (manage_supporters, ADR-0058 server administration), and the public list of
// the plans, which is a 404 while the program is disabled. The subscriptions of the users
// themselves (/supporter/*) are #922's.

// The problem types of the supporter program.
const (
	ProblemTypeRoleManagedByPlan      = "/api/v1/problems/role-managed-by-plan"
	ProblemTypeBreaksSupporterRoles   = "/api/v1/problems/breaks-supporter-roles"
	ProblemTypeSupportersUnavailable  = "/api/v1/problems/supporters-unavailable"
	ProblemTypeLiveSubscriptions      = "/api/v1/problems/live-subscriptions"
	ProblemTypeInvalidSupporterPlan   = "/api/v1/problems/invalid-supporter-plan"
	ProblemTypeSupporterPlanRetired   = "/api/v1/problems/supporter-plan-retired"
	ProblemTypeSubscriptionEnded      = "/api/v1/problems/subscription-ended"
	supporterUnavailableNoProvider    = "no_provider"
	supporterUnavailableNoEmail       = "no_email"
	supporterUnavailableWrongProvider = "provider_mismatch"
)

// Supporters runs the supporter program: *appsupporters.Service implements it.
type Supporters interface {
	Enabled(ctx context.Context) (bool, error)
	Program(ctx context.Context) (appsupporters.Program, error)
	SetProgram(ctx context.Context, enabled bool, byUserID string) (appsupporters.Program, error)
	Plans(ctx context.Context) ([]appsupporters.PlanView, error)
	Plan(ctx context.Context, id string) (appsupporters.PlanView, error)
	OfferedPlans(ctx context.Context) ([]appsupporters.PlanView, error)
	CreatePlan(ctx context.Context, in appsupporters.PlanInput) (appsupporters.PlanView, error)
	RevisePlan(ctx context.Context, in appsupporters.PlanUpdate) (appsupporters.PlanView, error)
	RetirePlan(ctx context.Context, id, byUserID string) error
	Subscriptions(ctx context.Context, f appsupporters.SubscriptionFilter) ([]appsupporters.SubscriptionView, error)
	Subscriber(ctx context.Context, userID string) (appsupporters.User, bool, error)
	CancelByStaff(ctx context.Context, id, byUserID string) error
	Subscription(ctx context.Context, id string) (appsupporters.SubscriptionView, error)
}

// requireSupporters checks the manage_supporters permission and that the use cases are
// configured; it returns the problem to answer otherwise.
func (s *Server) requireSupporters(ctx context.Context) (Viewer, *problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageSupporters); err != nil || p != nil {
		return Viewer{}, p, err
	}
	if s.o.Supporters == nil {
		p := s.notFoundResponse(ctx)
		return Viewer{}, &p, nil
	}
	v, err := requireViewer(ctx)
	return v, nil, err
}

func minorMoney(minor int64, currency string) Money {
	return newMoney(finance.Money{Minor: minor, Currency: finance.Currency(currency)})
}

func newAdminSupporterProgram(p appsupporters.Program) AdminSupporterProgram {
	out := AdminSupporterProgram{
		Enabled: p.Enabled, SendsEmail: p.SendsEmail, CommerceDisclosure: p.CommerceDisclosure,
		LiveSubscriptions: int(p.LiveSubscriptions), CanEnable: p.CanEnable(), Warnings: []SupporterProgramWarning{},
	}
	if p.Provider != "" {
		out.Provider = &p.Provider
	}
	if p.ConfiguredProvider != "" {
		out.ConfiguredProvider = &p.ConfiguredProvider
	} else {
		out.Warnings = append(out.Warnings, SupporterProgramWarningNoProvider)
	}
	if !p.ChangedAt.IsZero() {
		at := p.ChangedAt.UTC()
		out.ChangedAt = &at
	}
	if !p.SendsEmail {
		out.Warnings = append(out.Warnings, SupporterProgramWarningNoEmail)
	}
	if p.ProviderMismatch() {
		out.Warnings = append(out.Warnings, SupporterProgramWarningProviderMismatch)
	}
	if !p.CommerceDisclosure {
		out.Warnings = append(out.Warnings, SupporterProgramWarningNoCommerceDisclosure)
	}
	return out
}

func adminSupporterProgramResponse(p appsupporters.Program) AdminSupporterProgramJSONResponse {
	cache := cacheNoStore
	return AdminSupporterProgramJSONResponse{Body: newAdminSupporterProgram(p), Headers: AdminSupporterProgramResponseHeaders{CacheControl: &cache}}
}

// supportersUnavailable returns the 409 of a program that cannot be enabled or used.
func (s *Server) supportersUnavailable(ctx context.Context, reason string) problem {
	l := s.Localizer(ctx)
	return s.problem(ctx, http.StatusConflict, ProblemTypeSupportersUnavailable, l.ServerErrorsSupportersUnavailable(),
		l.ServerErrorsSupportersUnavailableDetail(reason))
}

// supporterProblem maps the errors of the supporter use cases that are not about a plan's
// values: it reports false for any other error.
func (s *Server) supporterProblem(ctx context.Context, err error) (problem, bool) {
	l := s.Localizer(ctx)
	switch {
	case errors.Is(err, appsupporters.ErrNotFound), errors.Is(err, appidentity.ErrNotFound):
		return s.notFoundResponse(ctx), true
	case errors.Is(err, appsupporters.ErrNoProvider):
		return s.supportersUnavailable(ctx, supporterUnavailableNoProvider), true
	case errors.Is(err, appsupporters.ErrNoMail):
		return s.supportersUnavailable(ctx, supporterUnavailableNoEmail), true
	case errors.Is(err, appsupporters.ErrProviderMismatch):
		return s.supportersUnavailable(ctx, supporterUnavailableWrongProvider), true
	case errors.Is(err, appsupporters.ErrLiveSubscriptions):
		return s.problem(ctx, http.StatusConflict, ProblemTypeLiveSubscriptions, l.ServerErrorsSupportersLiveSubscriptions(),
			l.ServerErrorsSupportersLiveSubscriptionsDetail()), true
	case errors.Is(err, appsupporters.ErrPlanRetired):
		return s.problem(ctx, http.StatusConflict, ProblemTypeSupporterPlanRetired, l.ServerErrorsSupportersPlanRetired(), ""), true
	case errors.Is(err, appsupporters.ErrNoLiveSubscription):
		return s.problem(ctx, http.StatusConflict, ProblemTypeSubscriptionEnded, l.ServerErrorsSupportersSubscriptionEnded(), ""), true
	}
	return problem{}, false
}

// planReason returns the reason of a plan that breaks a rule, and false for another error.
func planReason(err error) (SupporterPlanProblemReason, bool) {
	var pe *supporters.PlanError
	var te *supporters.TierError
	var ue *appidentity.UnmanageableRoleError
	switch {
	case errors.As(err, &pe):
		switch pe.Field {
		case supporters.FieldAmount:
			return SupporterPlanProblemReasonMonthlyPrice, true
		default:
			r := SupporterPlanProblemReason(pe.Field)
			return r, r.Valid()
		}
	case errors.As(err, &te):
		return SupporterPlanProblemReason("order_" + string(te.Rule)), true
	case errors.As(err, &ue):
		r := SupporterPlanProblemReason("role_" + string(ue.Rule))
		if ue.Rule == identity.RuleManaged {
			r = SupporterPlanProblemReasonRoleTaken
		}
		return r, true
	case errors.Is(err, appsupporters.ErrPlanRoleTaken):
		return SupporterPlanProblemReasonRoleTaken, true
	case errors.Is(err, appidentity.ErrInvalidRolePosition), errors.Is(err, appidentity.ErrRoleNotManageable):
		return SupporterPlanProblemReasonRolePosition, true
	case errors.Is(err, appidentity.ErrInvalidRolePermissions):
		return SupporterPlanProblemReasonRolePermissions, true
	}
	return "", false
}

// planProblem returns the 422 of a plan that breaks a rule.
func (s *Server) planProblem(ctx context.Context, reason SupporterPlanProblemReason) SupporterPlanProblemApplicationProblemPlusJSONResponse {
	l := s.Localizer(ctx)
	body, headers := s.NewProblem(ctx, http.StatusUnprocessableEntity, ProblemTypeInvalidSupporterPlan,
		l.ServerErrorsSupportersPlanInvalid(), l.ServerErrorsSupportersPlanInvalidDetail(string(reason)))
	return SupporterPlanProblemApplicationProblemPlusJSONResponse{
		Body: SupporterPlanProblem{
			Type: body.Type, Title: body.Title, Status: body.Status, Detail: body.Detail, RequestId: body.RequestId, Reason: &reason,
		},
		Headers: SupporterPlanProblemResponseHeaders{CacheControl: headers.CacheControl, ContentLanguage: headers.ContentLanguage},
	}
}

func newAdminSupporterPlan(p appsupporters.PlanView) AdminSupporterPlan {
	out := AdminSupporterPlan{
		Id: p.ID, Name: p.Revision.Name, Description: p.Revision.Description, Rank: p.Revision.Rank,
		Visibility:            SupporterPlanVisibility(p.Revision.Visibility),
		MonthlyPrice:          minorMoney(p.Revision.Amount(supporters.IntervalMonth), p.Currency),
		YearlyPrice:           minorMoney(p.Revision.Amount(supporters.IntervalYear), p.Currency),
		AnnualDiscountPercent: p.Revision.AnnualDiscountPercent, EmojiLimit: p.Revision.EmojiLimit,
		Role: AdminSupporterPlanRole{
			Id: p.RoleID, Key: p.Role.Key, Position: p.Role.Position, Permissions: make([]Permission, 0, len(p.Role.Permissions)),
		},
		PricesReady: p.Ready(), Retired: p.Retired, Members: int(p.Members),
		CreatedAt: p.CreatedAt.UTC(), RevisedAt: p.Revision.RevisedAt.UTC(),
	}
	for _, perm := range p.Role.Permissions {
		out.Role.Permissions = append(out.Role.Permissions, Permission(perm))
	}
	if p.Retired {
		at := p.RetiredAt.UTC()
		out.RetiredAt = &at
	}
	return out
}

func adminSupporterPlanResponse(p appsupporters.PlanView) AdminSupporterPlanJSONResponse {
	cache := cacheNoStore
	return AdminSupporterPlanJSONResponse{Body: newAdminSupporterPlan(p), Headers: AdminSupporterPlanResponseHeaders{CacheControl: &cache}}
}

// GetSupporterProgram serves GET /api/v1/admin/supporter_program.
func (s *Server) GetSupporterProgram(ctx context.Context, _ GetSupporterProgramRequestObject) (GetSupporterProgramResponseObject, error) {
	_, p, err := s.requireSupporters(ctx)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return GetSupporterProgramdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	prog, err := s.o.Supporters.Program(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the supporter program: %w", err)
	}
	return GetSupporterProgram200JSONResponse{adminSupporterProgramResponse(prog)}, nil
}

// SetSupporterProgram serves PUT /api/v1/admin/supporter_program.
func (s *Server) SetSupporterProgram(ctx context.Context, req SetSupporterProgramRequestObject) (SetSupporterProgramResponseObject, error) {
	v, p, err := s.requireSupporters(ctx)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return SetSupporterProgramdefaultApplicationProblemPlusJSONResponse(*p), nil
	case req.Body == nil:
		return SetSupporterProgramdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	prog, err := s.o.Supporters.SetProgram(ctx, req.Body.Enabled, v.UserID)
	if pr, ok := s.supporterProblem(ctx, err); ok {
		return SetSupporterProgramdefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: set the supporter program: %w", err)
	}
	return SetSupporterProgram200JSONResponse{adminSupporterProgramResponse(prog)}, nil
}

// ListAdminSupporterPlans serves GET /api/v1/admin/supporter_plans.
func (s *Server) ListAdminSupporterPlans(ctx context.Context, _ ListAdminSupporterPlansRequestObject) (ListAdminSupporterPlansResponseObject, error) {
	_, p, err := s.requireSupporters(ctx)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return ListAdminSupporterPlansdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	plans, err := s.o.Supporters.Plans(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the supporter plans: %w", err)
	}
	list := AdminSupporterPlanList{Items: make([]AdminSupporterPlan, 0, len(plans))}
	for _, pl := range plans {
		list.Items = append(list.Items, newAdminSupporterPlan(pl))
	}
	cache := cacheNoStore
	return ListAdminSupporterPlans200JSONResponse{Body: list, Headers: ListAdminSupporterPlans200ResponseHeaders{CacheControl: &cache}}, nil
}

// parseMonthly parses the monthly price of a request, in currency when it is not "".
func parseMonthly(m Money, currency string) (int64, string, bool) {
	if currency != "" && m.Currency != currency {
		return 0, "", false
	}
	v, err := finance.ParseMoney(m.Amount, m.Currency)
	if err != nil || strings.HasPrefix(m.Amount, "-") {
		return 0, "", false
	}
	return v.Minor, string(v.Currency), true
}

// CreateSupporterPlan serves POST /api/v1/admin/supporter_plans.
func (s *Server) CreateSupporterPlan(ctx context.Context, req CreateSupporterPlanRequestObject) (CreateSupporterPlanResponseObject, error) {
	v, p, err := s.requireSupporters(ctx)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return CreateSupporterPlandefaultApplicationProblemPlusJSONResponse(*p), nil
	case req.Body == nil:
		return CreateSupporterPlandefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	b := req.Body
	monthly, currency, ok := parseMonthly(b.MonthlyPrice, "")
	if !ok {
		return CreateSupporterPlan422ApplicationProblemPlusJSONResponse{s.planProblem(ctx, SupporterPlanProblemReasonMonthlyPrice)}, nil
	}
	in := appsupporters.PlanInput{
		RoleKey: b.Role, Currency: currency, ByUserID: v.UserID,
		Terms: supporters.PlanTerms{
			Name: b.Name, MonthlyAmountMinor: monthly, AnnualDiscountPercent: b.AnnualDiscountPercent, EmojiLimit: b.EmojiLimit,
			Rank: b.Rank, Visibility: supporters.Visibility(b.Visibility),
		},
	}
	if b.Description != nil {
		in.Terms.Description = *b.Description
	}
	plan, err := s.o.Supporters.CreatePlan(ctx, in)
	if reason, ok := planReason(err); ok {
		return CreateSupporterPlan422ApplicationProblemPlusJSONResponse{s.planProblem(ctx, reason)}, nil
	}
	if pr, ok := s.supporterProblem(ctx, err); ok {
		return CreateSupporterPlandefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: create a supporter plan: %w", err)
	}
	return CreateSupporterPlan201JSONResponse{adminSupporterPlanResponse(plan)}, nil
}

// UpdateSupporterPlan serves PATCH /api/v1/admin/supporter_plans/{id}.
func (s *Server) UpdateSupporterPlan(ctx context.Context, req UpdateSupporterPlanRequestObject) (UpdateSupporterPlanResponseObject, error) {
	v, p, err := s.requireSupporters(ctx)
	switch {
	case err != nil:
		return nil, err
	case p != nil:
		return UpdateSupporterPlandefaultApplicationProblemPlusJSONResponse(*p), nil
	case req.Body == nil:
		return UpdateSupporterPlandefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "body")), nil
	}
	b := req.Body
	in := appsupporters.PlanUpdate{
		ID: req.Id, Name: b.Name, Description: b.Description, AnnualDiscountPercent: b.AnnualDiscountPercent,
		EmojiLimit: b.EmojiLimit, Rank: b.Rank, RolePosition: b.RolePosition, ByUserID: v.UserID,
	}
	if b.Visibility != nil {
		vis := supporters.Visibility(*b.Visibility)
		in.Visibility = &vis
	}
	if b.RolePermissions != nil {
		in.SetRolePermissions = true
		in.RolePermissions = make([]appidentity.Permission, len(*b.RolePermissions))
		for i, perm := range *b.RolePermissions {
			in.RolePermissions[i] = appidentity.Permission(perm)
		}
	}
	if b.MonthlyPrice != nil {
		cur, err := s.o.Supporters.Plan(ctx, req.Id)
		if pr, ok := s.supporterProblem(ctx, err); ok {
			return UpdateSupporterPlandefaultApplicationProblemPlusJSONResponse(pr), nil
		}
		if err != nil {
			return nil, fmt.Errorf("clientapi: supporter plan %s: %w", req.Id, err)
		}
		monthly, _, ok := parseMonthly(*b.MonthlyPrice, cur.Currency)
		if !ok {
			return UpdateSupporterPlan422ApplicationProblemPlusJSONResponse{s.planProblem(ctx, SupporterPlanProblemReasonMonthlyPrice)}, nil
		}
		in.MonthlyAmountMinor = &monthly
	}
	plan, err := s.o.Supporters.RevisePlan(ctx, in)
	if reason, ok := planReason(err); ok {
		return UpdateSupporterPlan422ApplicationProblemPlusJSONResponse{s.planProblem(ctx, reason)}, nil
	}
	if pr, ok := s.supporterProblem(ctx, err); ok {
		return UpdateSupporterPlandefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: update supporter plan %s: %w", req.Id, err)
	}
	return UpdateSupporterPlan200JSONResponse{adminSupporterPlanResponse(plan)}, nil
}

// RetireSupporterPlan serves POST /api/v1/admin/supporter_plans/{id}/retire.
func (s *Server) RetireSupporterPlan(ctx context.Context, req RetireSupporterPlanRequestObject) (RetireSupporterPlanResponseObject, error) {
	v, p, err := s.requireSupporters(ctx)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return RetireSupporterPlandefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.Supporters.RetirePlan(ctx, req.Id, v.UserID)
	if pr, ok := s.supporterProblem(ctx, err); ok {
		return RetireSupporterPlandefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: retire supporter plan %s: %w", req.Id, err)
	}
	plan, err := s.o.Supporters.Plan(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: the supporter plan %s: %w", req.Id, err)
	}
	cache := cacheNoStore
	return RetireSupporterPlan200JSONResponse{AdminSupporterPlanJSONResponse{Body: newAdminSupporterPlan(plan), Headers: AdminSupporterPlanResponseHeaders{CacheControl: &cache}}}, nil
}

func (s *Server) newAdminSupporterSubscription(ctx context.Context, sv appsupporters.SubscriptionView) (AdminSupporterSubscription, error) {
	cur := sv.Current
	out := AdminSupporterSubscription{
		Id: sv.ID, State: SupporterSubscriptionState(cur.State), PlanId: cur.PlanID, PlanName: sv.Plan.Revision.Name,
		Interval: SupporterInterval(sv.Price.Interval), Price: minorMoney(sv.Price.AmountMinor, sv.Price.Currency),
		StartedAt: sv.CreatedAt.UTC(),
	}
	at := func(t time.Time) *time.Time {
		u := t.UTC()
		return &u
	}
	if !cur.PeriodEndsAt.IsZero() {
		out.PeriodEndsAt = at(cur.PeriodEndsAt)
	}
	if cur.Grace != nil {
		out.GraceEndsAt = at(cur.Grace.EndsAt)
	}
	if sp := sv.ScheduledPrice; sp != nil {
		id, interval := sp.PlanID, SupporterInterval(sp.Interval)
		out.ScheduledPlanId, out.ScheduledInterval = &id, &interval
	}
	u, ok, err := s.o.Supporters.Subscriber(ctx, sv.UserID)
	if err != nil {
		return AdminSupporterSubscription{}, fmt.Errorf("clientapi: the subscriber of %s: %w", sv.ID, err)
	}
	if ok {
		out.AccountId, out.Username = &u.AccountID, new(strings.TrimPrefix(u.Handle, "@"))
	}
	return out, nil
}

// ListSupporterSubscriptions serves GET /api/v1/admin/supporter_subscriptions.
func (s *Server) ListSupporterSubscriptions(ctx context.Context, req ListSupporterSubscriptionsRequestObject) (ListSupporterSubscriptionsResponseObject, error) {
	_, p, err := s.requireSupporters(ctx)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return ListSupporterSubscriptionsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListSupporterSubscriptionsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	f := appsupporters.SubscriptionFilter{Before: q.before, Limit: q.limit + 1}
	if req.Params.PlanId != nil {
		f.PlanID = knownID(*req.Params.PlanId)
	}
	if req.Params.State != nil {
		if !req.Params.State.Valid() {
			return ListSupporterSubscriptionsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, "state")), nil
		}
		f.State = supporters.State(*req.Params.State)
	}
	subs, err := s.o.Supporters.Subscriptions(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the subscriptions: %w", err)
	}
	subs, last := trim(subs, q.limit)
	page := AdminSupporterSubscriptionPage{Items: make([]AdminSupporterSubscription, 0, len(subs))}
	for _, sv := range subs {
		item, err := s.newAdminSupporterSubscription(ctx, sv)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, item)
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(subs[last].ID)
	}
	cache := cacheNoStore
	return ListSupporterSubscriptions200JSONResponse{Body: page, Headers: ListSupporterSubscriptions200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetSupporterSubscription serves GET /api/v1/admin/supporter_subscriptions/{id}.
func (s *Server) GetSupporterSubscription(ctx context.Context, req GetSupporterSubscriptionRequestObject) (GetSupporterSubscriptionResponseObject, error) {
	_, p, err := s.requireSupporters(ctx)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return GetSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	sv, err := s.o.Supporters.Subscription(ctx, req.Id)
	if pr, ok := s.supporterProblem(ctx, err); ok {
		return GetSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: subscription %s: %w", req.Id, err)
	}
	out, err := s.newAdminSupporterSubscription(ctx, sv)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return GetSupporterSubscription200JSONResponse{Body: out, Headers: GetSupporterSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// EndSupporterSubscription serves POST /api/v1/admin/supporter_subscriptions/{id}/end.
func (s *Server) EndSupporterSubscription(ctx context.Context, req EndSupporterSubscriptionRequestObject) (EndSupporterSubscriptionResponseObject, error) {
	v, p, err := s.requireSupporters(ctx)
	if err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return EndSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	err = s.o.Supporters.CancelByStaff(ctx, req.Id, v.UserID)
	if pr, ok := s.supporterProblem(ctx, err); ok {
		return EndSupporterSubscriptiondefaultApplicationProblemPlusJSONResponse(pr), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: end subscription %s: %w", req.Id, err)
	}
	sv, err := s.o.Supporters.Subscription(ctx, req.Id)
	if err != nil {
		return nil, fmt.Errorf("clientapi: subscription %s: %w", req.Id, err)
	}
	out, err := s.newAdminSupporterSubscription(ctx, sv)
	if err != nil {
		return nil, err
	}
	cache := cacheNoStore
	return EndSupporterSubscription200JSONResponse{Body: out, Headers: EndSupporterSubscription200ResponseHeaders{CacheControl: &cache}}, nil
}

// ListSupporterPlans serves GET /api/v1/instance/supporter_plans: a 404 while the program is
// disabled (ADR-0072 §2).
func (s *Server) ListSupporterPlans(ctx context.Context, _ ListSupporterPlansRequestObject) (ListSupporterPlansResponseObject, error) {
	if s.o.Supporters == nil {
		return ListSupporterPlansdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	plans, err := s.o.Supporters.OfferedPlans(ctx)
	if errors.Is(err, appsupporters.ErrDisabled) {
		return ListSupporterPlansdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	if err != nil {
		return nil, fmt.Errorf("clientapi: list the offered supporter plans: %w", err)
	}
	list := SupporterPlanList{Items: make([]SupporterPlan, 0, len(plans))}
	for _, p := range plans {
		list.Items = append(list.Items, SupporterPlan{
			Id: p.ID, Name: p.Revision.Name, Description: p.Revision.Description, Rank: p.Revision.Rank,
			MonthlyPrice: minorMoney(p.Prices[supporters.IntervalMonth].AmountMinor, p.Currency),
			YearlyPrice:  minorMoney(p.Prices[supporters.IntervalYear].AmountMinor, p.Currency),
		})
	}
	cache := cachePublic
	if _, ok := ViewerFrom(ctx); ok {
		cache = cachePrivate
	}
	return ListSupporterPlans200JSONResponse{Body: list, Headers: ListSupporterPlans200ResponseHeaders{CacheControl: &cache}}, nil
}
