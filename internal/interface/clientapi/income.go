// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance's income (ADR-0073 §6, §7, §9, FR-916, FR-917): the public totals that the
// instance setting income_visibility publishes, the totals of the finance viewers, and the
// administrators' totals and money movements. Only the administrators' operations carry the
// gross, the fees, individual movements, and the subscribers.

// Income totals the income records and lists their money movements. *appfinance.Service
// implements it.
type Income interface {
	IncomeTotals(ctx context.Context, q appfinance.IncomeQuery) ([]appfinance.IncomePeriod, error)
	PlanCounts(ctx context.Context) ([]appfinance.PlanCount, error)
	IncomeRules() []appfinance.ProviderRule
	ListIncome(ctx context.Context, f appfinance.IncomeFilter, before string, limit int) ([]appfinance.IncomeEntry, error)
}

// incomeView is what an answer of the income totals shows besides the net: the balance against
// the costs and the members of each plan.
type incomeView struct {
	balance, plans bool
}

// readIncomeTotals validates q and reads its income totals, or returns the problem of an
// invalid query.
func (s *Server) readIncomeTotals(ctx context.Context, q costTotalsQuery, balance bool) ([]appfinance.IncomePeriod, CostBasis, *problem, error) {
	query, basis, p := s.parseTotalsQuery(ctx, q)
	if p != nil {
		return nil, "", p, nil
	}
	totals, err := s.o.Income.IncomeTotals(ctx, appfinance.IncomeQuery{TotalsQuery: query, Balance: balance})
	switch {
	case errors.Is(err, finance.ErrInvalidPeriod), errors.Is(err, finance.ErrTooManyPeriods):
		p := s.unprocessable(ctx, "to")
		return nil, "", &p, nil
	case err != nil:
		return nil, "", nil, fmt.Errorf("clientapi: total the income: %w", err)
	}
	return totals, basis, nil, nil
}

// incomeTotals validates q and presents its income totals as v shows them, or returns the
// problem of an invalid query.
func (s *Server) incomeTotals(ctx context.Context, q costTotalsQuery, v incomeView) (IncomeTotals, *problem, error) {
	totals, basis, p, err := s.readIncomeTotals(ctx, q, v.balance)
	if err != nil || p != nil {
		return IncomeTotals{}, p, err
	}
	out := IncomeTotals{Granularity: q.granularity, Basis: basis, Periods: make([]IncomePeriodTotal, 0, len(totals)), Rules: s.incomeRules()}
	for _, t := range totals {
		period := IncomePeriodTotal{
			Label: t.Period.Label, StartsOn: t.Period.Range.Start.String(), EndsOn: t.Period.Range.End.String(), Net: newMoneys(t.Net),
		}
		if v.balance {
			b := newMoneys(t.Balance)
			period.Balance = &b
		}
		out.Periods = append(out.Periods, period)
	}
	if v.plans {
		plans, err := s.planCounts(ctx)
		if err != nil {
			return IncomeTotals{}, nil, err
		}
		out.Plans = &plans
	}
	return out, nil, nil
}

func newMoneys(ms []finance.Money) []Money {
	out := make([]Money, 0, len(ms))
	for _, m := range ms {
		out = append(out, newMoney(m))
	}
	return out
}

// incomeRules presents the rules that cut the periods of the income totals (ADR-0073 §3, §5).
func (s *Server) incomeRules() IncomeTotalsRules {
	rules := s.o.Income.IncomeRules()
	out := IncomeTotalsRules{
		WeekStart: IncomeTotalsRulesWeekStartMonday, AmortizeAfterDays: finance.IncomeAmortizeAfterDays,
		Providers: make([]IncomeProviderRule, 0, len(rules)),
	}
	for _, r := range rules {
		out.Providers = append(out.Providers, IncomeProviderRule{Provider: r.Provider, TimeZone: r.Location.String()})
	}
	return out
}

// planCounts presents the members of each supporter plan now (ADR-0073 §7).
func (s *Server) planCounts(ctx context.Context) ([]IncomePlanCount, error) {
	counts, err := s.o.Income.PlanCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("clientapi: count the members of the plans: %w", err)
	}
	out := make([]IncomePlanCount, 0, len(counts))
	for _, c := range counts {
		out = append(out, IncomePlanCount{PlanId: c.PlanID, Name: c.Name, Members: c.Members})
	}
	return out, nil
}

// instanceSettings returns the instance settings, the defaults without the instance service.
func (s *Server) instanceSettings(ctx context.Context) (instance.Settings, error) {
	if s.o.Instance == nil {
		return instance.Settings{}, nil
	}
	settings, err := s.o.Instance.Settings(ctx)
	if err != nil {
		return instance.Settings{}, fmt.Errorf("clientapi: instance settings: %w", err)
	}
	return settings, nil
}

// GetInstanceIncomeTotals serves GET /api/v1/instance/income_totals: the net income that the
// instance setting income_visibility publishes to everyone (FR-917, ADR-0073 §7). private is a
// 404, plans adds the members of each plan, the balance is shown only while finance_visibility
// publishes the costs too, and the periods stop at the one that holds today.
func (s *Server) GetInstanceIncomeTotals(ctx context.Context, req GetInstanceIncomeTotalsRequestObject) (GetInstanceIncomeTotalsResponseObject, error) {
	settings, err := s.instanceSettings(ctx)
	if err != nil {
		return nil, err
	}
	visibility := settings.IncomeVisibility()
	if visibility == instance.IncomePrivate || s.o.Income == nil {
		return GetInstanceIncomeTotalsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	params := req.Params
	out, p, err := s.incomeTotals(ctx, costTotalsQuery{
		granularity: params.Granularity, from: params.From, to: params.To, basis: params.Basis, untilToday: true,
	}, incomeView{balance: settings.FinanceVisibility() != instance.FinancePrivate, plans: visibility == instance.IncomePlans})
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetInstanceIncomeTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := publicCache(ctx)
	return GetInstanceIncomeTotals200JSONResponse{Body: out, Headers: GetInstanceIncomeTotals200ResponseHeaders{CacheControl: &cache}}, nil
}

// GetFinanceIncomeTotals serves GET /api/v1/finance/income_totals: the net income with its
// balance for the holders of view_finance (ADR-0073 §6), and the members of each plan while
// finance_viewer_income or income_visibility shows them; a holder of manage_finance reads them
// always.
func (s *Server) GetFinanceIncomeTotals(ctx context.Context, req GetFinanceIncomeTotalsRequestObject) (GetFinanceIncomeTotalsResponseObject, error) {
	manage, p, err := s.requireFinanceViewer(ctx)
	if err != nil {
		return nil, err
	}
	if p == nil && s.o.Income == nil {
		nf := s.notFoundResponse(ctx)
		p = &nf
	}
	if p != nil {
		return GetFinanceIncomeTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	settings, err := s.instanceSettings(ctx)
	if err != nil {
		return nil, err
	}
	plans := manage || settings.FinanceViewer().Income == instance.FinanceViewerIncomePlans || settings.IncomeVisibility() == instance.IncomePlans
	params := req.Params
	out, p, err := s.incomeTotals(ctx, costTotalsQuery{
		granularity: params.Granularity, from: params.From, to: params.To, basis: params.Basis,
	}, incomeView{balance: true, plans: plans})
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetFinanceIncomeTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cachePrivateNoStore
	return GetFinanceIncomeTotals200JSONResponse{Body: out, Headers: GetFinanceIncomeTotals200ResponseHeaders{CacheControl: &cache}}, nil
}

// requireIncome checks the manage_finance permission and that the income use cases are
// configured; it returns the problem to answer otherwise.
func (s *Server) requireIncome(ctx context.Context) (*problem, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		return p, err
	}
	if s.o.Income == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

// GetIncomeTotals serves GET /api/v1/admin/income_totals: the gross, the fees, the net, and the
// balance of each period, with the members of each plan (ADR-0073 §6).
func (s *Server) GetIncomeTotals(ctx context.Context, req GetIncomeTotalsRequestObject) (GetIncomeTotalsResponseObject, error) {
	if p, err := s.requireIncome(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return GetIncomeTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	params := req.Params
	q := costTotalsQuery{granularity: params.Granularity, from: params.From, to: params.To, basis: params.Basis}
	totals, basis, p, err := s.readIncomeTotals(ctx, q, true)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetIncomeTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	plans, err := s.planCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := AdminIncomeTotals{
		Granularity: params.Granularity, Basis: basis, Periods: make([]AdminIncomePeriodTotal, 0, len(totals)), Plans: plans,
		Rules: s.incomeRules(),
	}
	for _, t := range totals {
		out.Periods = append(out.Periods, AdminIncomePeriodTotal{
			Label: t.Period.Label, StartsOn: t.Period.Range.Start.String(), EndsOn: t.Period.Range.End.String(),
			Gross: newMoneys(t.Gross), Fee: newMoneys(t.Fee), Net: newMoneys(t.Net), Balance: newMoneys(t.Balance),
		})
	}
	cache := cacheNoStore
	return GetIncomeTotals200JSONResponse{Body: out, Headers: GetIncomeTotals200ResponseHeaders{CacheControl: &cache}}, nil
}

// ListIncomeMovements serves GET /api/v1/admin/income_movements.
func (s *Server) ListIncomeMovements(ctx context.Context, req ListIncomeMovementsRequestObject) (ListIncomeMovementsResponseObject, error) {
	if p, err := s.requireIncome(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return ListIncomeMovementsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	params := req.Params
	reject := func(name string) (ListIncomeMovementsResponseObject, error) {
		return ListIncomeMovementsdefaultApplicationProblemPlusJSONResponse(s.invalidParameterResponse(ctx, name)), nil
	}
	f := appfinance.IncomeFilter{From: finance.MinDate, To: finance.MaxDate}
	var err error
	if params.From != nil {
		if f.From, err = finance.ParseDate(*params.From); err != nil {
			return reject("from")
		}
	}
	if params.To != nil {
		if f.To, err = finance.ParseDate(*params.To); err != nil {
			return reject("to")
		}
	}
	if params.Kind != nil {
		if !params.Kind.Valid() {
			return reject("kind")
		}
		f.Kind = finance.IncomeKind(*params.Kind)
	}
	q := s.parsePage(ctx, params.Cursor, params.Limit)
	if q.problem != nil {
		return ListIncomeMovementsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	list, err := s.o.Income.ListIncome(ctx, f, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list income movements: %w", err)
	}
	list, last := trim(list, q.limit)
	page := IncomeMovementPage{Items: make([]IncomeMovement, 0, len(list))}
	for _, e := range list {
		page.Items = append(page.Items, newIncomeMovement(e))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(list[last].ID)
	}
	cache := cacheNoStore
	return ListIncomeMovements200JSONResponse{Body: page, Headers: ListIncomeMovements200ResponseHeaders{CacheControl: &cache}}, nil
}

func newIncomeMovement(e appfinance.IncomeEntry) IncomeMovement {
	out := IncomeMovement{
		Id: e.ID, Provider: e.Provider, ProviderId: e.ProviderID, Kind: IncomeKind(e.Kind), OccurredAt: e.OccurredAt.UTC(),
		OccurredOn: e.OccurredOn.String(), Gross: newMoney(e.Gross), Fee: newMoney(e.Fee), Net: newMoney(e.Net),
		RecordedAt: e.RecordedAt.UTC(),
	}
	if e.ParentProviderID != "" {
		out.ParentProviderId = &e.ParentProviderID
	}
	if e.SubscriptionID != "" {
		out.SubscriptionId = &e.SubscriptionID
	}
	if e.Subscriber.AccountID != "" {
		out.AccountId, out.Username = &e.Subscriber.AccountID, &e.Subscriber.Username
	}
	if e.Covers != nil {
		start, end := e.Covers.Start.String(), e.Covers.End.String()
		out.CoversStartsOn, out.CoversEndsOn = &start, &end
	}
	return out
}
