// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeCosts holds one cost item and records the changes and who made them.
type fakeCosts struct {
	item  finance.CostItem
	calls []string
}

func (f *fakeCosts) Create(_ context.Context, in appfinance.CreateInput, byUserID string) (finance.CostItem, error) {
	if err := in.Recurrence.Validate(); err != nil {
		return finance.CostItem{}, err
	}
	f.calls = append(f.calls, "create "+in.Label.Name+" by "+byUserID)
	return f.item, nil
}

func (f *fakeCosts) Relabel(_ context.Context, id string, label finance.Label, byUserID string) (finance.CostItem, error) {
	if id != f.item.ID {
		return finance.CostItem{}, appfinance.ErrNotFound
	}
	f.calls = append(f.calls, "relabel "+label.Name+" "+label.Provider+" by "+byUserID)
	return f.item, nil
}

func (f *fakeCosts) RevisePrice(_ context.Context, _ string, on finance.Date, price finance.Money, byUserID string) (finance.CostItem, error) {
	f.calls = append(f.calls, "revise "+on.String()+" "+price.Decimal()+" by "+byUserID)
	return f.item, nil
}

func (f *fakeCosts) SetEnd(_ context.Context, _ string, endsOn *finance.Date, byUserID string) (finance.CostItem, error) {
	end := "none"
	if endsOn != nil {
		end = endsOn.String()
	}
	f.calls = append(f.calls, "end "+end+" by "+byUserID)
	return f.item, nil
}

func (f *fakeCosts) Cancel(_ context.Context, id, byUserID string) error {
	if id != f.item.ID {
		return appfinance.ErrNotFound
	}
	f.calls = append(f.calls, "cancel by "+byUserID)
	return nil
}

func (f *fakeCosts) Get(_ context.Context, id string) (finance.CostItem, error) {
	if id != f.item.ID {
		return finance.CostItem{}, appfinance.ErrNotFound
	}
	return f.item, nil
}

func (f *fakeCosts) List(context.Context, string, int) ([]finance.CostItem, error) {
	return []finance.CostItem{f.item}, nil
}

func (f *fakeCosts) Totals(_ context.Context, q appfinance.TotalsQuery) ([]finance.PeriodTotal, error) {
	periods, err := q.Granularity.Periods(q.From, q.To)
	if err != nil {
		return nil, err
	}
	out := make([]finance.PeriodTotal, len(periods))
	for i, p := range periods {
		out[i] = finance.PeriodTotal{Period: p, Totals: []finance.Money{{Minor: 1234, Currency: "USD"}}, Estimated: q.Basis == finance.BasisCash}
	}
	return out, nil
}

func (f *fakeCosts) Rules() appfinance.Rules {
	pacific, _ := time.LoadLocation("America/Los_Angeles")
	return appfinance.Rules{WeekStart: time.Monday, Sources: []appfinance.SourceRule{{Source: "gcp", Provider: "Google Cloud", Location: pacific}}}
}

func TestCostItems(t *testing.T) {
	start, _ := finance.ParseDate("2026-04-10")
	costs := &fakeCosts{item: finance.CostItem{
		ID: "0199f0e0-0000-7000-8000-0000000000c1", Recurrence: finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 1},
		StartsOn: start, Label: finance.Label{Name: "example.com", Provider: "Registrar", Category: finance.CategoryDomain},
		Prices:    []finance.PriceRevision{{EffectiveOn: start, Price: finance.Money{Minor: 1650, Currency: "JPY"}}},
		CreatedAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{
			userOf(aliceAccountID): {identity.PermManageFinance},
			userOf(bobAccountID):   {identity.PermManageSettings},
		}
		o.Costs = costs
	})
	e.auth.tokens["admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	e.auth.tokens["admin-read"] = bearer("admin:read", aliceAccountID)
	id := costs.item.ID
	newItem := `{"schedule":"years","interval":1,"starts_on":"2026-04-10","name":"example.com","provider":"Registrar",` +
		`"category":"domain","price":{"amount":"1650","currency":"JPY"}}`

	for _, tc := range []struct {
		method, path, token, body string
		status                    int
	}{
		{http.MethodGet, "/admin/cost_items", "admin-read", "", http.StatusOK},
		{http.MethodGet, "/admin/cost_items/" + id, "admin-read", "", http.StatusOK},
		{http.MethodGet, "/admin/cost_items/0199f0e0-0000-7000-8000-0000000000ff", "admin-read", "", http.StatusNotFound},
		{http.MethodPost, "/admin/cost_items", "admin", newItem, http.StatusCreated},
		{http.MethodPatch, "/admin/cost_items/" + id, "admin", `{"provider":"Other registrar"}`, http.StatusOK},
		{http.MethodPost, "/admin/cost_items/" + id + "/price_revisions", "admin", `{"effective_on":"2027-04-10","price":{"amount":"1800","currency":"JPY"}}`, http.StatusOK},
		{http.MethodPut, "/admin/cost_items/" + id + "/end", "admin", `{"ends_on":"2028-04-09"}`, http.StatusOK},
		{http.MethodPut, "/admin/cost_items/" + id + "/end", "admin", `{}`, http.StatusOK},
		{http.MethodDelete, "/admin/cost_items/" + id, "admin", "", http.StatusNoContent},
		{http.MethodGet, "/admin/cost_totals?granularity=year&from=2026-01-01&to=2027-12-31", "admin-read", "", http.StatusOK},

		// The values the server rejects.
		{http.MethodPost, "/admin/cost_items", "admin", `{"schedule":"days","interval":1,"starts_on":"2026-04-10","name":"x","provider":"y","category":"other","price":{"amount":"1","currency":"JPY"}}`, http.StatusUnprocessableEntity},
		{http.MethodPost, "/admin/cost_items", "admin", `{"schedule":"once","interval":3,"starts_on":"2026-04-10","name":"x","provider":"y","category":"other","price":{"amount":"1","currency":"JPY"}}`, http.StatusUnprocessableEntity},
		{http.MethodPost, "/admin/cost_items", "admin", `{"schedule":"once","interval":1,"starts_on":"2026-04-10","name":"x","provider":"y","category":"other","price":{"amount":"1.234","currency":"USD"}}`, http.StatusUnprocessableEntity},
		{http.MethodPost, "/admin/cost_items", "admin", `{"schedule":"once","interval":1,"starts_on":"2026-04-10","name":"x","provider":"y","category":"other","price":{"amount":"1","currency":"XXX"}}`, http.StatusUnprocessableEntity},
		{http.MethodPost, "/admin/cost_items/" + id + "/price_revisions", "admin", `{"effective_on":"2027-13-01","price":{"amount":"1","currency":"JPY"}}`, http.StatusUnprocessableEntity},
		{http.MethodDelete, "/admin/cost_items/0199f0e0-0000-7000-8000-0000000000ff", "admin", "", http.StatusNotFound},
		{http.MethodGet, "/admin/cost_totals?granularity=week&from=2026-01-01&to=2028-01-01", "admin-read", "", http.StatusUnprocessableEntity},
		{http.MethodGet, "/admin/cost_totals?granularity=day&from=2026-01-01&to=2026-01-02", "admin-read", "", http.StatusBadRequest},
		{http.MethodGet, "/admin/cost_totals?granularity=month&from=2026-01-01&to=2026-01-02&basis=accrual", "admin-read", "", http.StatusBadRequest},

		// Without manage_finance, or without the write scope.
		{http.MethodGet, "/admin/cost_items", "bob-admin", "", http.StatusForbidden},
		{http.MethodPost, "/admin/cost_items", "bob-admin", newItem, http.StatusForbidden},
		{http.MethodPatch, "/admin/cost_items/" + id, "bob-admin", `{"name":"x"}`, http.StatusForbidden},
		{http.MethodDelete, "/admin/cost_items/" + id, "bob-admin", "", http.StatusForbidden},
		{http.MethodGet, "/admin/cost_totals?granularity=month&from=2026-01-01&to=2026-01-02", "bob-admin", "", http.StatusForbidden},
		{http.MethodPost, "/admin/cost_items", "admin-read", newItem, http.StatusForbidden},
	} {
		if res := e.adminBody(t, tc.method, tc.path, tc.token, tc.body); res.StatusCode != tc.status {
			t.Errorf("%s %s with %s = %d, want %d", tc.method, tc.path, tc.token, res.StatusCode, tc.status)
		}
	}
	by := " by " + userOf(aliceAccountID)
	want := []string{
		"create example.com" + by, "relabel example.com Other registrar" + by, "revise 2027-04-10 1800" + by,
		"end 2028-04-09" + by, "end none" + by, "cancel" + by,
	}
	if len(costs.calls) != len(want) {
		t.Fatalf("calls = %q, want %q", costs.calls, want)
	}
	for i := range want {
		if costs.calls[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, costs.calls[i], want[i])
		}
	}

	res := e.adminBody(t, http.MethodGet, "/admin/cost_totals?granularity=month&from=2026-04-01&to=2026-05-31&basis=cash", "admin-read", "")
	totals := decodeJSON[clientapi.CostTotals](t, res)
	if totals.Basis != clientapi.CostBasisCash || len(totals.Periods) != 2 || totals.Periods[0].Label != "2026-04" || !totals.Periods[0].Estimated ||
		totals.Periods[0].Totals[0] != (clientapi.Money{Amount: "12.34", Currency: "USD"}) || res.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("totals = %+v", totals)
	}
	if len(totals.Rules.Sources) != 1 || totals.Rules.Sources[0].TimeZone != "America/Los_Angeles" || totals.Rules.WeekStart != "monday" {
		t.Errorf("rules = %+v", totals.Rules)
	}
	item := decodeJSON[clientapi.CostItem](t, e.adminBody(t, http.MethodGet, "/admin/cost_items/"+id, "admin-read", ""))
	if item.Schedule != "years" || item.StartsOn != "2026-04-10" || item.EndsOn != nil || item.Prices[0].Price != (clientapi.Money{Amount: "1650", Currency: "JPY"}) {
		t.Errorf("item = %+v", item)
	}
}
