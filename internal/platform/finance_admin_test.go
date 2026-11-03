// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestCostItemsAdmin drives the admin API of the instance's costs on SQLite (ADR-0067 §9): an
// administrator records a cost paid once, every week, every month, and every year, changes the
// label, revises the price, ends and cancels items, reads the totals amortized and in cash, and
// finds every change in the audit log.
func TestCostItemsAdmin(t *testing.T) {
	t.Parallel()
	e := newAccountsEnv(t)
	admin := e.user("alice")
	create := func(body map[string]any) clientapi.CostItem {
		t.Helper()
		var item clientapi.CostItem
		e.want(e.do(http.MethodPost, "/admin/cost_items", admin.token, body, &item), http.StatusCreated, "POST /admin/cost_items")
		return item
	}
	domain := create(map[string]any{
		"schedule": "years", "interval": 1, "starts_on": "2026-04-10", "name": "example.com", "provider": "Registrar",
		"category": "domain", "price": map[string]string{"amount": "1650", "currency": "JPY"},
	})
	vm := create(map[string]any{
		"schedule": "months", "interval": 1, "starts_on": "2026-01-31", "name": "VM", "provider": "Ｃｌｏｕｄ",
		"category": "compute", "memo": "the main server", "price": map[string]string{"amount": "12.34", "currency": "USD"},
	})
	weekly := create(map[string]any{
		"schedule": "weeks", "interval": 2, "starts_on": "2026-04-06", "name": "Backups", "provider": "Storage",
		"category": "storage", "price": map[string]string{"amount": "100", "currency": "JPY"},
	})
	once := create(map[string]any{
		"schedule": "once", "interval": 1, "starts_on": "2026-04-15", "name": "Mistake", "provider": "Shop",
		"category": "other", "price": map[string]string{"amount": "500", "currency": "JPY"},
	})
	if domain.Schedule != clientapi.CostScheduleYears || domain.StartsOn != "2026-04-10" || len(domain.Prices) != 1 ||
		domain.Prices[0].Price != (clientapi.Money{Amount: "1650", Currency: "JPY"}) || vm.Memo != "the main server" {
		t.Errorf("created = %+v, %+v", domain, vm)
	}

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"too many digits", map[string]any{
			"schedule": "once", "interval": 1, "starts_on": "2026-04-15", "name": "x", "provider": "y", "category": "other",
			"price": map[string]string{"amount": "1.5", "currency": "JPY"},
		}},
		{"interval of once", map[string]any{
			"schedule": "once", "interval": 2, "starts_on": "2026-04-15", "name": "x", "provider": "y", "category": "other",
			"price": map[string]string{"amount": "1", "currency": "JPY"},
		}},
		{"end before start", map[string]any{
			"schedule": "months", "interval": 1, "starts_on": "2026-04-15", "ends_on": "2026-04-14", "name": "x", "provider": "y",
			"category": "other", "price": map[string]string{"amount": "1", "currency": "JPY"},
		}},
		{"February 30", map[string]any{
			"schedule": "once", "interval": 1, "starts_on": "2026-02-30", "name": "x", "provider": "y", "category": "other",
			"price": map[string]string{"amount": "1", "currency": "JPY"},
		}},
	} {
		e.want(e.do(http.MethodPost, "/admin/cost_items", admin.token, tc.body, nil), http.StatusUnprocessableEntity, tc.name)
	}

	var item clientapi.CostItem
	e.want(e.do(http.MethodPatch, "/admin/cost_items/"+vm.Id, admin.token, map[string]any{"name": "Main VM"}, &item), http.StatusOK, "PATCH")
	if item.Name != "Main VM" || item.Provider != "Ｃｌｏｕｄ" || item.Memo != "the main server" {
		t.Errorf("relabeled = %+v", item)
	}
	e.want(e.do(http.MethodPost, "/admin/cost_items/"+vm.Id+"/price_revisions", admin.token, map[string]any{
		"effective_on": "2026-05-01", "price": map[string]string{"amount": "15.00", "currency": "USD"},
	}, &item), http.StatusOK, "POST price_revisions")
	if len(item.Prices) != 2 || item.Prices[1].EffectiveOn != "2026-05-01" || item.Prices[1].Price.Amount != "15.00" {
		t.Errorf("revised = %+v", item.Prices)
	}
	e.want(e.do(http.MethodPost, "/admin/cost_items/"+vm.Id+"/price_revisions", admin.token, map[string]any{
		"effective_on": "2025-12-31", "price": map[string]string{"amount": "1", "currency": "USD"},
	}, nil), http.StatusUnprocessableEntity, "a revision before the start")
	e.want(e.do(http.MethodPut, "/admin/cost_items/"+weekly.Id+"/end", admin.token, map[string]any{"ends_on": "2026-04-30"}, &item), http.StatusOK, "PUT end")
	if item.EndsOn == nil || *item.EndsOn != "2026-04-30" {
		t.Errorf("ended = %+v", item)
	}
	e.want(e.do(http.MethodPost, "/admin/cost_items/"+once.Id+"/cancel", admin.token, nil, nil), http.StatusNoContent, "POST cancel")
	e.want(e.do(http.MethodPost, "/admin/cost_items/"+once.Id+"/cancel", admin.token, nil, nil), http.StatusNotFound, "cancel twice")
	e.want(e.do(http.MethodGet, "/admin/cost_items/"+once.Id, admin.token, nil, nil), http.StatusNotFound, "GET a canceled item")

	var page clientapi.CostItemPage
	e.want(e.do(http.MethodGet, "/admin/cost_items?limit=2", admin.token, nil, &page), http.StatusOK, "GET /admin/cost_items")
	if len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].Id != weekly.Id || page.Items[1].Id != vm.Id {
		t.Errorf("first page = %+v", page)
	}

	var totals clientapi.CostTotals
	e.want(e.do(http.MethodGet, "/admin/cost_totals?granularity=month&from=2026-04-01&to=2026-05-31", admin.token, nil, &totals),
		http.StatusOK, "GET /admin/cost_totals")
	if totals.Basis != clientapi.CostBasisAmortized || len(totals.Periods) != 2 || totals.Rules.WeekStart != "monday" {
		t.Fatalf("totals = %+v", totals)
	}
	april := totals.Periods[0]
	// 21 of the 365 days of the domain (floor(1650·21/365) = 94), two backups, and the VM.
	if april.Label != "2026-04" || april.StartsOn != "2026-04-01" || april.EndsOn != "2026-04-30" || april.Estimated ||
		!slices.Equal(april.Totals, []clientapi.Money{{Amount: "294", Currency: "JPY"}, {Amount: "12.34", Currency: "USD"}}) {
		t.Errorf("April amortized = %+v", april)
	}
	if !slices.ContainsFunc(april.Breakdown, func(l clientapi.CostBreakdownLine) bool {
		return l.Provider == "Ｃｌｏｕｄ" && l.Category == clientapi.CostCategoryCompute && l.Amount.Amount == "12.34"
	}) {
		t.Errorf("April breakdown = %+v", april.Breakdown)
	}
	e.want(e.do(http.MethodGet, "/admin/cost_totals?granularity=month&from=2026-04-01&to=2026-04-30&basis=cash", admin.token, nil, &totals),
		http.StatusOK, "GET /admin/cost_totals in cash")
	if !slices.Equal(totals.Periods[0].Totals, []clientapi.Money{{Amount: "1850", Currency: "JPY"}, {Amount: "12.34", Currency: "USD"}}) {
		t.Errorf("April in cash = %+v", totals.Periods[0].Totals)
	}
	e.want(e.do(http.MethodGet, "/admin/cost_totals?granularity=month&from=2026-01-01&to=2031-01-01", admin.token, nil, nil),
		http.StatusUnprocessableEntity, "61 months")
	e.want(e.do(http.MethodGet, "/admin/cost_totals?granularity=day&from=2026-01-01&to=2026-01-02", admin.token, nil, nil),
		http.StatusBadRequest, "an unknown granularity")

	var audit clientapi.AdminAuditEntryPage
	e.want(e.do(http.MethodGet, "/admin/audit_log?target_kind=cost_item&limit=40", admin.token, nil, &audit), http.StatusOK, "GET /admin/audit_log")
	var got []string
	for _, entry := range audit.Items {
		got = append(got, string(entry.Action)+" "+entry.TargetLabel)
	}
	want := []string{
		"cost_item_canceled Mistake", "cost_item_end_changed Backups", "cost_item_price_revised Main VM", "cost_item_relabeled Main VM",
		"cost_item_created Mistake", "cost_item_created Backups", "cost_item_created VM", "cost_item_created example.com",
	}
	if !slices.Equal(got, want) {
		t.Errorf("audit log = %q, want %q", got, want)
	}
}
