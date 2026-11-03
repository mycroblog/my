// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// viewerCosts is fakeCosts whose totals are the domain's totals of its item and of an imported
// cost, itemized when the query asks for the item lines.
type viewerCosts struct {
	*fakeCosts
	imported []finance.Entry
	queries  []appfinance.TotalsQuery
}

func (f *viewerCosts) Totals(_ context.Context, q appfinance.TotalsQuery) ([]finance.PeriodTotal, error) {
	f.queries = append(f.queries, q)
	periods, err := q.Granularity.Periods(q.From, q.To)
	if err != nil {
		return nil, err
	}
	items := []finance.CostItem{f.item}
	if q.Items {
		return finance.ItemizedTotals(items, f.imported, q.Basis, periods), nil
	}
	return finance.Totals(items, f.imported, q.Basis, periods), nil
}

// TestFinanceCostTotals checks GET /finance/cost_totals (ADR-0070 §5): view_finance alone reads
// the totals and their breakdown with a read scope and no admin scope, but records and changes
// nothing; the item lines, their names and their memos follow the instance settings, hidden by
// default; manage_finance reads every line.
func TestFinanceCostTotals(t *testing.T) {
	start, _ := finance.ParseDate("2026-04-01")
	end, _ := finance.ParseDate("2026-04-30")
	month := finance.DateRange{Start: start, End: end}
	costs := &viewerCosts{
		fakeCosts: &fakeCosts{item: finance.CostItem{
			ID: "0199f0e0-0000-7000-8000-0000000000c1", Recurrence: finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1},
			StartsOn: start, Label: finance.Label{Name: "Main VPS", Provider: "Hetzner", Category: finance.CategoryCompute, Memo: "fsn1-a"},
			ProviderNormalized: "hetzner",
			Prices:             []finance.PriceRevision{{EffectiveOn: start, Price: finance.Money{Minor: 500, Currency: "EUR"}}},
		}},
		imported: []finance.Entry{{
			Range: month, Amount: finance.Money{Minor: 1200, Currency: "USD"}, Provider: "google cloud", Spelling: finance.Spelling{Name: "Google Cloud"}, Category: finance.CategoryStorage,
			Item: finance.ItemRef{Key: "import:gcp:Cloud Storage", Name: "Cloud Storage"},
		}},
	}
	settings := settingsInstance{&fakeSettings{}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{
			userOf(aliceAccountID): {identity.PermManageSettings},
			userOf(bobAccountID):   {identity.PermManageFinance},
			userOf(carolAccountID): {identity.PermViewFinance},
		}
		o.Costs, o.Settings, o.Instance = costs, settings.fakeSettings, settings
	})
	e.auth.tokens["alice-admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-read"] = bearer("read", bobAccountID)
	e.auth.tokens["carol-read"] = bearer("read", carolAccountID)
	e.auth.tokens["carol-finance"] = bearer("read:finance", carolAccountID)
	e.auth.tokens["carol-admin"] = bearer("admin", carolAccountID)
	e.auth.tokens["dave-read"] = bearer("read", daveAccountID)
	const path = "/finance/cost_totals?granularity=month&from=2026-04-01&to=2026-04-30"

	// read fetches the totals and checks that no answer carries what no setting shows.
	read := func(token string) (clientapi.FinanceCostTotals, string) {
		t.Helper()
		res := e.send(t, http.MethodGet, path, token, "")
		if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "private, no-store" {
			t.Fatalf("GET %s as %s = %d, Cache-Control %q", path, token, res.StatusCode, res.Header.Get("Cache-Control"))
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{costs.item.ID, `"prices"`, `"created_by`, "revised"} {
			if strings.Contains(string(body), secret) {
				t.Errorf("finance totals contain %s: %s", secret, body)
			}
		}
		return decode[clientapi.FinanceCostTotals](t, &http.Response{Body: io.NopCloser(strings.NewReader(string(body)))}), string(body)
	}
	set := func(values string) {
		t.Helper()
		if res := e.send(t, http.MethodPatch, "/admin/settings", "alice-admin", `{"values":{`+values+`}}`); res.StatusCode != http.StatusOK {
			t.Fatalf("PATCH /admin/settings %s = %d", values, res.StatusCode)
		}
	}
	lines := func(out clientapi.FinanceCostTotals) []clientapi.FinanceCostItemLine {
		t.Helper()
		if len(out.Periods) != 1 {
			t.Fatalf("periods = %+v, want 1", out.Periods)
		}
		return out.Periods[0].Items
	}

	// view_finance alone, with read:finance and no admin scope: the totals, the breakdown, and
	// the rules always; no item line by default.
	for _, token := range []string{"carol-read", "carol-finance"} {
		out, body := read(token)
		if len(out.Periods) != 1 || len(out.Periods[0].Totals) != 2 || len(out.Periods[0].Breakdown) != 2 || len(out.Rules.Sources) != 1 {
			t.Errorf("totals as %s = %+v, want 2 totals, 2 breakdown lines, and the rules", token, out)
		}
		if len(lines(out)) != 0 || strings.Contains(body, "Main VPS") || strings.Contains(body, "fsn1-a") || strings.Contains(body, "Cloud Storage") {
			t.Errorf("default answer shows the items: %s", body)
		}
		if out.Viewer != (clientapi.FinanceViewerSettings{ItemNames: false, ItemMemos: false, Income: clientapi.FinanceViewerIncomeTotals}) {
			t.Errorf("viewer = %+v, want the defaults", out.Viewer)
		}
	}
	if q := costs.queries[0]; q.Items || q.UntilToday {
		t.Errorf("default query = %+v, want no item lines and periods beyond today", q)
	}

	// Neither an admin scope alone nor a user without the permission reads them.
	if res := e.send(t, http.MethodGet, path, "carol-admin", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("GET %s with an admin scope only = %d, want 403", path, res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, path, "dave-read", ""); res.StatusCode != http.StatusForbidden ||
		res.Header.Get("Content-Type") != "application/problem+json" {
		t.Errorf("GET %s without view_finance = %d, want 403", path, res.StatusCode)
	} else if p := decode[clientapi.Problem](t, res); p.Type != clientapi.ProblemTypeMissingPermission {
		t.Errorf("problem type = %q, want missing-permission", p.Type)
	}

	// view_finance records and changes nothing, and reads no admin operation.
	costs.calls = nil
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/admin/cost_totals?granularity=month&from=2026-04-01&to=2026-04-30", ""},
		{http.MethodGet, "/admin/cost_items", ""},
		{http.MethodPost, "/admin/cost_items", `{"name":"x","provider":"y","category":"other","schedule":"once","interval":1,"starts_on":"2026-04-01","price":{"amount":"1","currency":"USD"}}`},
		{http.MethodDelete, "/admin/cost_items/" + costs.item.ID, ""},
		{http.MethodPatch, "/admin/settings", `{"values":{"finance_viewer_item_names":"true"}}`},
	} {
		if res := e.send(t, c.method, c.path, "carol-admin", c.body); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as view_finance = %d, want 403", c.method, c.path, res.StatusCode)
		}
	}
	if len(costs.calls) != 0 {
		t.Errorf("view_finance changed the costs: %v", costs.calls)
	}

	// Names only: every line with its name, no memo.
	set(`"finance_viewer_item_names":"true"`)
	out, body := read("carol-read")
	got := lines(out)
	if len(got) != 2 || got[0].Name == nil || *got[0].Name != "Cloud Storage" || got[0].Provider != "Google Cloud" || got[0].Amount.Amount != "12.00" ||
		got[1].Name == nil || *got[1].Name != "Main VPS" || got[1].Category != clientapi.CostCategoryCompute || got[1].Amount.Amount != "5.00" {
		t.Errorf("lines with names = %s", body)
	}
	if strings.Contains(body, "fsn1-a") || strings.Contains(body, `"memo"`) || !out.Viewer.ItemNames || out.Viewer.ItemMemos {
		t.Errorf("names only shows the memos or reports other settings: %s", body)
	}

	// Memos only: the lines without names, the item's memo on its line.
	set(`"finance_viewer_item_names":"false","finance_viewer_item_memos":"true","finance_viewer_income":"plans"`)
	out, body = read("carol-read")
	got = lines(out)
	if len(got) != 2 || strings.Contains(body, `"name"`) || strings.Contains(body, "Main VPS") || strings.Contains(body, "Cloud Storage") {
		t.Errorf("memos only shows the names: %s", body)
	}
	memos := 0
	for _, line := range got {
		if line.Memo != nil {
			memos++
			if *line.Memo != "fsn1-a" || line.Provider != "Hetzner" {
				t.Errorf("memo line = %+v", line)
			}
		}
	}
	if memos != 1 || out.Viewer.Income != clientapi.FinanceViewerIncomePlans {
		t.Errorf("memos only = %s, want the item's memo on one line and income plans", body)
	}

	// manage_finance reads every name and memo whatever the settings.
	set(`"finance_viewer_item_memos":"false"`)
	out, body = read("bob-read")
	got = lines(out)
	if len(got) != 2 || got[1].Name == nil || *got[1].Name != "Main VPS" || got[1].Memo == nil || *got[1].Memo != "fsn1-a" || got[0].Memo != nil {
		t.Errorf("lines as manage_finance = %s", body)
	}
	if out.Viewer.ItemNames || out.Viewer.ItemMemos {
		t.Errorf("viewer as manage_finance = %+v, want the instance's settings", out.Viewer)
	}
	if _, body := read("carol-read"); strings.Contains(body, "Main VPS") || strings.Contains(body, "fsn1-a") {
		t.Errorf("view_finance reads the items after both settings are off: %s", body)
	}

	// The query is validated as on the admin totals.
	if res := e.send(t, http.MethodGet, "/finance/cost_totals?granularity=month&from=2026-05-01&to=2026-04-01", "carol-read", ""); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("to before from = %d, want 422", res.StatusCode)
	}
}
