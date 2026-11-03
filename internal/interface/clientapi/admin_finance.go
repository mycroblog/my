// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/domain/instance"
)

// The instance's costs (ADR-0067 §9, FR-901, FR-902): the admin operations on
// /admin/cost_items and /admin/cost_totals (manage_finance, ADR-0058 server administration).

// Costs runs the use cases of the cost items and their totals. *appfinance.Service implements it.
type Costs interface {
	Create(ctx context.Context, in appfinance.CreateInput, byUserID string) (finance.CostItem, error)
	Relabel(ctx context.Context, id string, label finance.Label, byUserID string) (finance.CostItem, error)
	RevisePrice(ctx context.Context, id string, effectiveOn finance.Date, price finance.Money, byUserID string) (finance.CostItem, error)
	SetEnd(ctx context.Context, id string, endsOn *finance.Date, byUserID string) (finance.CostItem, error)
	Cancel(ctx context.Context, id, byUserID string) error
	Get(ctx context.Context, id string) (finance.CostItem, error)
	List(ctx context.Context, before string, limit int) ([]finance.CostItem, error)
	Totals(ctx context.Context, q appfinance.TotalsQuery) ([]finance.PeriodTotal, error)
	Rules() appfinance.Rules
}

// requireCosts checks the manage_finance permission and that the use cases are configured; it
// returns the problem to answer otherwise.
func (s *Server) requireCosts(ctx context.Context) (*problem, error) {
	if p, err := s.requirePermission(ctx, identity.PermManageFinance); err != nil || p != nil {
		return p, err
	}
	if s.o.Costs == nil {
		p := s.notFoundResponse(ctx)
		return &p, nil
	}
	return nil, nil
}

func newCostItem(c finance.CostItem) CostItem {
	out := CostItem{
		Id: c.ID, Schedule: CostSchedule(c.Recurrence.Schedule), Interval: c.Recurrence.Interval, StartsOn: c.StartsOn.String(),
		Name: c.Label.Name, Provider: c.Label.Provider, Category: CostCategory(c.Label.Category), Memo: c.Label.Memo,
		Prices: make([]CostPriceRevision, 0, len(c.Prices)), CreatedAt: c.CreatedAt.UTC(),
	}
	if c.EndsOn != nil {
		ends := c.EndsOn.String()
		out.EndsOn = &ends
	}
	for _, p := range c.Prices {
		out.Prices = append(out.Prices, CostPriceRevision{EffectiveOn: p.EffectiveOn.String(), Price: newMoney(p.Price), RevisedAt: p.RevisedAt.UTC()})
	}
	return out
}

func costItemResponse(c finance.CostItem) CostItemJSONResponse {
	cache := cacheNoStore
	return CostItemJSONResponse{Body: newCostItem(c), Headers: CostItemResponseHeaders{CacheControl: &cache}}
}

// costField names the property of the request that an error of the finance use cases rejects,
// and false for an error that is not about the request's values.
func costField(err error, dateField string) (string, bool) {
	switch {
	case errors.Is(err, finance.ErrInvalidRecurrence):
		return "interval", true
	case errors.Is(err, finance.ErrInvalidName):
		return "name", true
	case errors.Is(err, finance.ErrInvalidProvider):
		return "provider", true
	case errors.Is(err, finance.ErrInvalidCategory):
		return "category", true
	case errors.Is(err, finance.ErrInvalidMemo):
		return "memo", true
	case errors.Is(err, finance.ErrInvalidAmount), errors.Is(err, finance.ErrInvalidCurrency), errors.Is(err, finance.ErrInvalidDecimal),
		errors.Is(err, finance.ErrTooManyDigits):
		return "price", true
	case errors.Is(err, finance.ErrBeforeStart), errors.Is(err, finance.ErrInvalidDate):
		return dateField, true
	}
	return "", false
}

// costProblem returns the problem of an error of the finance use cases: a 404 for an unknown or
// canceled item, a 422 naming the rejected property, and false for any other error.
func (s *Server) costProblem(ctx context.Context, err error, dateField string) (problem, bool) {
	if errors.Is(err, appfinance.ErrNotFound) {
		return s.notFoundResponse(ctx), true
	}
	if name, ok := costField(err, dateField); ok {
		return s.unprocessable(ctx, name), true
	}
	return problem{}, false
}

// unprocessable returns the 422 problem of a property whose value the use cases reject.
func (s *Server) unprocessable(ctx context.Context, name string) problem {
	l := s.Localizer(ctx)
	return s.problem(ctx, http.StatusUnprocessableEntity, ProblemTypeBlank, l.ServerErrorsUnprocessable(), l.ServerErrorsInvalidValue(name))
}

// parseCostDate parses an optional date of a request.
func parseCostDate(s *CalendarDate) (*finance.Date, error) {
	if s == nil {
		return nil, nil
	}
	d, err := finance.ParseDate(*s)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ListCostItems serves GET /api/v1/admin/cost_items.
func (s *Server) ListCostItems(ctx context.Context, req ListCostItemsRequestObject) (ListCostItemsResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return ListCostItemsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	q := s.parsePage(ctx, req.Params.Cursor, req.Params.Limit)
	if q.problem != nil {
		return ListCostItemsdefaultApplicationProblemPlusJSONResponse(*q.problem), nil
	}
	list, err := s.o.Costs.List(ctx, q.before, q.limit+1)
	if err != nil {
		return nil, fmt.Errorf("clientapi: list cost items: %w", err)
	}
	list, last := trim(list, q.limit)
	page := CostItemPage{Items: make([]CostItem, 0, len(list))}
	for _, c := range list {
		page.Items = append(page.Items, newCostItem(c))
	}
	if last >= 0 {
		page.NextCursor = encodeCursor(list[last].ID)
	}
	cache := cacheNoStore
	return ListCostItems200JSONResponse{Body: page, Headers: ListCostItems200ResponseHeaders{CacheControl: &cache}}, nil
}

// CreateCostItem serves POST /api/v1/admin/cost_items.
func (s *Server) CreateCostItem(ctx context.Context, req CreateCostItemRequestObject) (CreateCostItemResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return CreateCostItemdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	if !b.Schedule.Valid() {
		return CreateCostItemdefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "schedule")), nil
	}
	starts, err := finance.ParseDate(b.StartsOn)
	if err != nil {
		return CreateCostItemdefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "starts_on")), nil
	}
	ends, err := parseCostDate(b.EndsOn)
	if err != nil {
		return CreateCostItemdefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "ends_on")), nil
	}
	price, err := finance.ParseMoney(b.Price.Amount, b.Price.Currency)
	if err != nil {
		return CreateCostItemdefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "price")), nil
	}
	label := finance.Label{Name: b.Name, Provider: b.Provider, Category: finance.Category(b.Category)}
	if b.Memo != nil {
		label.Memo = *b.Memo
	}
	item, err := s.o.Costs.Create(ctx, appfinance.CreateInput{
		Recurrence: finance.Recurrence{Schedule: finance.Schedule(b.Schedule), Interval: b.Interval},
		StartsOn:   starts, EndsOn: ends, Label: label, Price: price,
	}, v.UserID)
	if err != nil {
		if p, ok := s.costProblem(ctx, err, "ends_on"); ok {
			return CreateCostItemdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: create cost item: %w", err)
	}
	return CreateCostItem200JSONResponse{costItemResponse(item)}, nil
}

// GetCostItem serves GET /api/v1/admin/cost_items/{id}.
func (s *Server) GetCostItem(ctx context.Context, req GetCostItemRequestObject) (GetCostItemResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return GetCostItemdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	item, err := s.o.Costs.Get(ctx, req.Id)
	if errors.Is(err, appfinance.ErrNotFound) {
		return GetCostItemdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	} else if err != nil {
		return nil, fmt.Errorf("clientapi: get cost item %s: %w", req.Id, err)
	}
	return GetCostItem200JSONResponse{costItemResponse(item)}, nil
}

// UpdateCostItem serves PATCH /api/v1/admin/cost_items/{id}.
func (s *Server) UpdateCostItem(ctx context.Context, req UpdateCostItemRequestObject) (UpdateCostItemResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return UpdateCostItemdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	cur, err := s.o.Costs.Get(ctx, req.Id)
	if errors.Is(err, appfinance.ErrNotFound) {
		return UpdateCostItemdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	} else if err != nil {
		return nil, fmt.Errorf("clientapi: get cost item %s: %w", req.Id, err)
	}
	label, b := cur.Label, req.Body
	if b.Name != nil {
		label.Name = *b.Name
	}
	if b.Provider != nil {
		label.Provider = *b.Provider
	}
	if b.Category != nil {
		label.Category = finance.Category(*b.Category)
	}
	if b.Memo != nil {
		label.Memo = *b.Memo
	}
	item, err := s.o.Costs.Relabel(ctx, req.Id, label, v.UserID)
	if err != nil {
		if p, ok := s.costProblem(ctx, err, ""); ok {
			return UpdateCostItemdefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: relabel cost item %s: %w", req.Id, err)
	}
	return UpdateCostItem200JSONResponse{costItemResponse(item)}, nil
}

// ReviseCostItemPrice serves POST /api/v1/admin/cost_items/{id}/price_revisions.
func (s *Server) ReviseCostItemPrice(ctx context.Context, req ReviseCostItemPriceRequestObject) (ReviseCostItemPriceResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return ReviseCostItemPricedefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	effective, err := finance.ParseDate(req.Body.EffectiveOn)
	if err != nil {
		return ReviseCostItemPricedefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "effective_on")), nil
	}
	price, err := finance.ParseMoney(req.Body.Price.Amount, req.Body.Price.Currency)
	if err != nil {
		return ReviseCostItemPricedefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "price")), nil
	}
	item, err := s.o.Costs.RevisePrice(ctx, req.Id, effective, price, v.UserID)
	if err != nil {
		if p, ok := s.costProblem(ctx, err, "effective_on"); ok {
			return ReviseCostItemPricedefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: revise the price of cost item %s: %w", req.Id, err)
	}
	return ReviseCostItemPrice200JSONResponse{costItemResponse(item)}, nil
}

// SetCostItemEnd serves PUT /api/v1/admin/cost_items/{id}/end.
func (s *Server) SetCostItemEnd(ctx context.Context, req SetCostItemEndRequestObject) (SetCostItemEndResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return SetCostItemEnddefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	ends, err := parseCostDate(req.Body.EndsOn)
	if err != nil {
		return SetCostItemEnddefaultApplicationProblemPlusJSONResponse(s.unprocessable(ctx, "ends_on")), nil
	}
	item, err := s.o.Costs.SetEnd(ctx, req.Id, ends, v.UserID)
	if err != nil {
		if p, ok := s.costProblem(ctx, err, "ends_on"); ok {
			return SetCostItemEnddefaultApplicationProblemPlusJSONResponse(p), nil
		}
		return nil, fmt.Errorf("clientapi: set the end of cost item %s: %w", req.Id, err)
	}
	return SetCostItemEnd200JSONResponse{costItemResponse(item)}, nil
}

// CancelCostItem serves POST /api/v1/admin/cost_items/{id}/cancel.
func (s *Server) CancelCostItem(ctx context.Context, req CancelCostItemRequestObject) (CancelCostItemResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return CancelCostItemdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	v, err := requireViewer(ctx)
	if err != nil {
		return nil, err
	}
	err = s.o.Costs.Cancel(ctx, req.Id, v.UserID)
	if errors.Is(err, appfinance.ErrNotFound) {
		return CancelCostItemdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	} else if err != nil {
		return nil, fmt.Errorf("clientapi: cancel cost item %s: %w", req.Id, err)
	}
	return CancelCostItem204Response{}, nil
}

// GetCostTotals serves GET /api/v1/admin/cost_totals.
func (s *Server) GetCostTotals(ctx context.Context, req GetCostTotalsRequestObject) (GetCostTotalsResponseObject, error) {
	if p, err := s.requireCosts(ctx); err != nil || p != nil {
		if err != nil {
			return nil, err
		}
		return GetCostTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	params := req.Params
	out, p, err := s.costTotals(ctx, costTotalsQuery{
		granularity: params.Granularity, from: params.From, to: params.To, basis: params.Basis, breakdown: true,
	})
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetCostTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := cacheNoStore
	return GetCostTotals200JSONResponse{Body: out, Headers: GetCostTotals200ResponseHeaders{CacheControl: &cache}}, nil
}

// costTotalsQuery is the query of the admin and the public totals, and what the answer may show.
type costTotalsQuery struct {
	granularity CostGranularity
	from, to    CalendarDate
	basis       *CostBasis
	// breakdown is whether the answer shows the breakdown by provider and category and the
	// providers' rules; untilToday stops the periods at the one that holds today (ADR-0067 §8);
	// items adds the item lines to the periods (ADR-0070 §5).
	breakdown, untilToday, items bool
}

// parseTotalsQuery validates the query of the cost and the income totals, or returns the
// problem of an invalid one; the basis is amortized unless the query names one.
func (s *Server) parseTotalsQuery(ctx context.Context, q costTotalsQuery) (appfinance.TotalsQuery, CostBasis, *problem) {
	reject := func(p problem) (appfinance.TotalsQuery, CostBasis, *problem) { return appfinance.TotalsQuery{}, "", &p }
	if !q.granularity.Valid() {
		return reject(s.invalidParameterResponse(ctx, "granularity"))
	}
	basis := CostBasisAmortized
	if q.basis != nil {
		basis = *q.basis
	}
	if !basis.Valid() {
		return reject(s.invalidParameterResponse(ctx, "basis"))
	}
	from, err := finance.ParseDate(q.from)
	if err != nil {
		return reject(s.invalidParameterResponse(ctx, "from"))
	}
	to, err := finance.ParseDate(q.to)
	if err != nil {
		return reject(s.invalidParameterResponse(ctx, "to"))
	}
	return appfinance.TotalsQuery{
		Granularity: finance.Granularity(q.granularity), From: from, To: to, Basis: finance.Basis(basis), UntilToday: q.untilToday,
		Items: q.items,
	}, basis, nil
}

// readCostTotals validates q and reads its totals and the basis they use, or returns the
// problem of an invalid query.
func (s *Server) readCostTotals(ctx context.Context, q costTotalsQuery) ([]finance.PeriodTotal, CostBasis, *problem, error) {
	query, basis, p := s.parseTotalsQuery(ctx, q)
	if p != nil {
		return nil, "", p, nil
	}
	totals, err := s.o.Costs.Totals(ctx, query)
	switch {
	case errors.Is(err, finance.ErrInvalidPeriod), errors.Is(err, finance.ErrTooManyPeriods):
		p := s.unprocessable(ctx, "to")
		return nil, "", &p, nil
	case err != nil:
		return nil, "", nil, fmt.Errorf("clientapi: total the costs: %w", err)
	}
	return totals, basis, nil, nil
}

// costTotals validates q and presents its totals, or returns the problem of an invalid query.
func (s *Server) costTotals(ctx context.Context, q costTotalsQuery) (CostTotals, *problem, error) {
	totals, basis, p, err := s.readCostTotals(ctx, q)
	if err != nil || p != nil {
		return CostTotals{}, p, err
	}
	rules := CostTotalsRules{WeekStart: CostTotalsRulesWeekStartMonday, Sources: []CostSourceRule{}}
	if q.breakdown {
		rules = s.costRules()
	}
	out := CostTotals{Granularity: q.granularity, Basis: basis, Periods: make([]CostPeriodTotal, 0, len(totals)), Rules: rules}
	for _, t := range totals {
		out.Periods = append(out.Periods, newCostPeriodTotal(t, q.breakdown))
	}
	return out, nil, nil
}

// newCostPeriodTotal presents the totals of a period, with their breakdown by provider and
// category when breakdown is true.
func newCostPeriodTotal(t finance.PeriodTotal, breakdown bool) CostPeriodTotal {
	p := CostPeriodTotal{
		Label: t.Period.Label, StartsOn: t.Period.Range.Start.String(), EndsOn: t.Period.Range.End.String(), Estimated: t.Estimated,
		Totals: make([]Money, 0, len(t.Totals)), Breakdown: []CostBreakdownLine{},
	}
	for _, m := range t.Totals {
		p.Totals = append(p.Totals, newMoney(m))
	}
	if breakdown {
		for _, line := range t.Breakdown {
			p.Breakdown = append(p.Breakdown, CostBreakdownLine{
				Provider: line.ProviderName, Category: CostCategory(line.Key.Category),
				Amount: newMoney(finance.Money{Minor: line.Amount, Currency: line.Key.Currency}),
			})
		}
	}
	return p
}

// GetInstanceCostTotals serves GET /api/v1/instance/cost_totals: the totals that the instance
// setting finance_visibility publishes to everyone (FR-903, ADR-0067 §8). private is a 404,
// totals leaves the breakdown and the providers' rules out, and the periods stop at the one that
// holds today. The answer is a projection of the totals: no level carries the names, memos, or
// individual payments of the cost items.
func (s *Server) GetInstanceCostTotals(ctx context.Context, req GetInstanceCostTotalsRequestObject) (GetInstanceCostTotalsResponseObject, error) {
	var settings instance.Settings
	if s.o.Instance != nil {
		var err error
		if settings, err = s.o.Instance.Settings(ctx); err != nil {
			return nil, fmt.Errorf("clientapi: instance settings: %w", err)
		}
	}
	visibility := settings.FinanceVisibility()
	if visibility == instance.FinancePrivate || s.o.Costs == nil {
		return GetInstanceCostTotalsdefaultApplicationProblemPlusJSONResponse(s.notFoundResponse(ctx)), nil
	}
	params := req.Params
	out, p, err := s.costTotals(ctx, costTotalsQuery{
		granularity: params.Granularity, from: params.From, to: params.To, basis: params.Basis,
		breakdown: visibility == instance.FinanceBreakdown, untilToday: true,
	})
	if err != nil {
		return nil, err
	}
	if p != nil {
		return GetInstanceCostTotalsdefaultApplicationProblemPlusJSONResponse(*p), nil
	}
	cache := publicCache(ctx)
	return GetInstanceCostTotals200JSONResponse{Body: out, Headers: GetInstanceCostTotals200ResponseHeaders{CacheControl: &cache}}, nil
}

// costRules presents the rules that cut the periods of the totals (ADR-0067 §5).
func (s *Server) costRules() CostTotalsRules {
	rules := s.o.Costs.Rules()
	out := CostTotalsRules{WeekStart: CostTotalsRulesWeekStartMonday, Sources: make([]CostSourceRule, 0, len(rules.Sources))}
	for _, src := range rules.Sources {
		out.Sources = append(out.Sources, CostSourceRule{Source: BillingSourceName(src.Source), Provider: src.Provider, TimeZone: src.Location.String()})
	}
	return out
}
