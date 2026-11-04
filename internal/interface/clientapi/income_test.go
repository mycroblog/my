// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/identity"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// fakeIncome totals a yearly payment of 12,000 yen with a fee of 432 yen on 2026-04-10, which
// the domain spreads over its year, against costs of 1,000 yen a period, and lists that payment
// with its subscription, subscriber, and provider ID, which only the administrators may read,
// and its refund, whose subscriber's account is gone.
type fakeIncome struct {
	queries []appfinance.IncomeQuery
	filters []appfinance.IncomeFilter
}

const (
	incomeSubscription = "0199f0e0-0000-7000-8000-00000000a5b1"
	incomeAccount      = "0199f0e0-0000-7000-8000-00000000acc1"
	incomeUsername     = "patron_secret"
	incomeProviderID   = "txn_secret_1"
)

func incomeDate(s string) finance.Date {
	d, err := finance.ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (f *fakeIncome) IncomeTotals(_ context.Context, q appfinance.IncomeQuery) ([]appfinance.IncomePeriod, error) {
	f.queries = append(f.queries, q)
	periods, err := q.Granularity.Periods(q.From, q.To)
	if err != nil {
		return nil, err
	}
	covers := finance.DateRange{Start: incomeDate("2026-04-10"), End: incomeDate("2027-04-09")}
	jpy := func(n int64) finance.Money { return finance.Money{Minor: n, Currency: "JPY"} }
	ms := []finance.IncomeMovement{{
		Kind: finance.IncomePayment, OccurredOn: covers.Start, Covers: &covers, Gross: jpy(12000), Fee: jpy(432), Net: jpy(11568),
	}}
	var out []appfinance.IncomePeriod
	for _, t := range finance.IncomeTotals(ms, q.Basis, periods) {
		p := appfinance.IncomePeriod{IncomeTotal: t}
		if q.Balance {
			p.Balance = finance.Balance(t.Net, []finance.Money{jpy(1000)})
		}
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeIncome) PlanCounts(context.Context) ([]appfinance.PlanCount, error) {
	return []appfinance.PlanCount{{PlanID: "0199f0e0-0000-7000-8000-00000000b001", Name: "Gold", Members: 3}}, nil
}

func (f *fakeIncome) IncomeRules() []appfinance.ProviderRule {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		tokyo = time.UTC
	}
	return []appfinance.ProviderRule{{Provider: "stripe", Location: tokyo}}
}

func (f *fakeIncome) ListIncome(_ context.Context, filter appfinance.IncomeFilter, _ string, _ int) ([]appfinance.IncomeEntry, error) {
	f.filters = append(f.filters, filter)
	covers := finance.DateRange{Start: incomeDate("2026-04-10"), End: incomeDate("2027-04-09")}
	e := appfinance.IncomeEntry{IncomeRecord: appfinance.IncomeRecord{
		ID: "0199f0e0-0000-7000-8000-00000000f001", Provider: "stripe", ProviderID: incomeProviderID, Kind: finance.IncomePayment,
		SubscriptionID: incomeSubscription, OccurredAt: time.Date(2026, 4, 10, 3, 0, 0, 0, time.UTC), OccurredOn: covers.Start,
		Covers: &covers, Gross: finance.Money{Minor: 12000, Currency: "JPY"}, Fee: finance.Money{Minor: 432, Currency: "JPY"},
		Net: finance.Money{Minor: 11568, Currency: "JPY"}, RecordedAt: time.Date(2026, 4, 10, 3, 5, 0, 0, time.UTC),
	}, Subscriber: appfinance.Subscriber{AccountID: incomeAccount, Username: incomeUsername}}
	refund := appfinance.IncomeEntry{IncomeRecord: appfinance.IncomeRecord{
		ID: "0199f0e0-0000-7000-8000-00000000f002", Provider: "stripe", ProviderID: "txn_refund_1", Kind: finance.IncomeRefund,
		ParentProviderID: incomeProviderID, SubscriptionID: incomeSubscription, OccurredAt: time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC),
		OccurredOn: incomeDate("2026-06-01"), Gross: finance.Money{Minor: -12000, Currency: "JPY"}, Net: finance.Money{Minor: -12000, Currency: "JPY"},
		Fee: finance.Money{Currency: "JPY"}, RecordedAt: time.Date(2026, 6, 1, 3, 5, 0, 0, time.UTC),
	}}
	var out []appfinance.IncomeEntry
	for _, m := range []appfinance.IncomeEntry{refund, e} {
		if filter.Kind == "" || filter.Kind == m.Kind {
			out = append(out, m)
		}
	}
	return out, nil
}

// incomeSecrets are what neither the public totals nor the finance viewers may read: who paid
// what, and the gross and the fees (ADR-0067 §16, §17).
var incomeSecrets = []string{incomeSubscription, incomeAccount, incomeUsername, incomeProviderID, `"gross"`, `"fee"`, "12000", `"432"`}

func newIncomeEnv(t *testing.T) (*authEnv, *fakeIncome) {
	t.Helper()
	start := incomeDate("2026-04-01")
	costs := &fakeCosts{item: finance.CostItem{
		ID: "0199f0e0-0000-7000-8000-0000000000c1", Recurrence: finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1},
		StartsOn: start, Label: finance.Label{Name: "VPS", Provider: "Hetzner", Category: finance.CategoryCompute},
		Prices: []finance.PriceRevision{{EffectiveOn: start, Price: finance.Money{Minor: 1000, Currency: "JPY"}}},
	}}
	income := &fakeIncome{}
	settings := settingsInstance{&fakeSettings{}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Permissions = fakePermissions{
			userOf(aliceAccountID): {identity.PermManageSettings},
			userOf(bobAccountID):   {identity.PermManageFinance},
			userOf(carolAccountID): {identity.PermViewFinance},
		}
		o.Costs, o.Income, o.Settings, o.Instance = costs, income, settings.fakeSettings, settings
	})
	e.auth.tokens["alice-admin"] = bearer("admin", aliceAccountID)
	e.auth.tokens["bob-admin"] = bearer("admin", bobAccountID)
	e.auth.tokens["bob-finance"] = bearer("read:finance", bobAccountID)
	e.auth.tokens["carol-finance"] = bearer("read:finance", carolAccountID)
	e.auth.tokens["carol-admin"] = bearer("admin", carolAccountID)
	e.auth.tokens["dave-finance"] = bearer("read:finance", daveAccountID)
	return e, income
}

// readIncome fetches path with token, checks its status and Cache-Control, and checks that the
// answer carries none of incomeSecrets.
func readIncome(t *testing.T, e *authEnv, path, token, cache string) clientapi.IncomeTotals {
	t.Helper()
	res := e.send(t, http.MethodGet, path, token, "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != cache {
		t.Fatalf("GET %s = %d, Cache-Control %q, want 200 and %q", path, res.StatusCode, res.Header.Get("Cache-Control"), cache)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range incomeSecrets {
		if strings.Contains(string(body), secret) {
			t.Errorf("GET %s carries %s: %s", path, secret, body)
		}
	}
	var out clientapi.IncomeTotals
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func setSetting(t *testing.T, e *authEnv, key, value string) {
	t.Helper()
	if res := e.send(t, http.MethodPatch, "/admin/settings", "alice-admin", `{"values":{"`+key+`":"`+value+`"}}`); res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH %s %s = %d", key, value, res.StatusCode)
	}
}

// TestInstanceIncomeTotals checks what GET /instance/income_totals publishes at each level of
// income_visibility, apart from finance_visibility (FR-917, ADR-0067 §17): never a payment of
// anyone, the gross, or the fees; the plans only at plans; and the balance only while both
// settings publish.
func TestInstanceIncomeTotals(t *testing.T) {
	e, income := newIncomeEnv(t)
	const path = "/instance/income_totals?granularity=month&from=2026-04-01&to=2026-05-31"
	const costs = "/instance/cost_totals?granularity=month&from=2026-04-01&to=2026-05-31"
	status := func(p string) int {
		t.Helper()
		return e.send(t, http.MethodGet, p, "", "").StatusCode
	}
	visibility := func() clientapi.IncomeVisibility {
		t.Helper()
		return decode[clientapi.Instance](t, e.send(t, http.MethodGet, "/instance", "", "")).IncomeVisibility
	}

	// private by default: a 404, and nothing computed.
	if got := visibility(); got != clientapi.IncomeVisibilityPrivate {
		t.Errorf("income_visibility = %q, want private", got)
	}
	if got := status(path); got != http.StatusNotFound {
		t.Errorf("GET %s while private = %d, want 404", path, got)
	}
	if len(income.queries) != 0 {
		t.Errorf("private income was computed: %+v", income.queries)
	}
	if res := e.send(t, http.MethodPatch, "/admin/settings", "bob-admin", `{"values":{"income_visibility":"totals"}}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("PATCH income_visibility without manage_settings = %d, want 403", res.StatusCode)
	}

	// totals with private costs: the net by period, without the balance, the plans, or the costs.
	setSetting(t, e, "income_visibility", "totals")
	if got := visibility(); got != clientapi.IncomeVisibilityTotals {
		t.Errorf("income_visibility = %q, want totals", got)
	}
	out := readIncome(t, e, path, "", "public, max-age=60")
	if len(out.Periods) != 2 || out.Periods[0].Net[0] != (clientapi.Money{Amount: "665", Currency: "JPY"}) {
		t.Errorf("periods = %+v, want the net of April spread over the year", out.Periods)
	}
	for _, p := range out.Periods {
		if p.Balance != nil {
			t.Errorf("%s has a balance while the costs are private: %+v", p.Label, p.Balance)
		}
	}
	if out.Plans != nil {
		t.Errorf("plans with totals = %+v", out.Plans)
	}
	if len(out.Rules.Providers) != 1 || out.Rules.Providers[0].TimeZone != "Asia/Tokyo" || out.Rules.AmortizeAfterDays != 31 {
		t.Errorf("rules = %+v", out.Rules)
	}
	if q := income.queries[len(income.queries)-1]; q.Balance || !q.UntilToday {
		t.Errorf("public query = %+v, want no balance and UntilToday", q)
	}
	if got := status(costs); got != http.StatusNotFound {
		t.Errorf("GET %s with only the income public = %d, want 404", costs, got)
	}

	// Both public: the balance appears; plans: the members of each plan.
	setSetting(t, e, "finance_visibility", "totals")
	setSetting(t, e, "income_visibility", "plans")
	out = readIncome(t, e, path, "", "public, max-age=60")
	if b := out.Periods[0].Balance; b == nil || len(*b) != 1 || (*b)[0] != (clientapi.Money{Amount: "-335", Currency: "JPY"}) {
		t.Errorf("balance of April = %+v, want 665 - 1000", b)
	}
	if out.Plans == nil || len(*out.Plans) != 1 || (*out.Plans)[0].Members != 3 || (*out.Plans)[0].Name != "Gold" {
		t.Errorf("plans = %+v", out.Plans)
	}

	// The costs public and the income private: the costs only.
	setSetting(t, e, "income_visibility", "private")
	if got := status(path); got != http.StatusNotFound {
		t.Errorf("GET %s while private = %d, want 404", path, got)
	}
	if got := status(costs); got != http.StatusOK {
		t.Errorf("GET %s with the costs public = %d, want 200", costs, got)
	}

	setSetting(t, e, "income_visibility", "totals")
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/instance/income_totals?granularity=day&from=2026-01-01&to=2026-01-02", http.StatusBadRequest},
		{"/instance/income_totals?granularity=month&from=2026-01-01&to=2031-01-01", http.StatusUnprocessableEntity},
		{"/instance/income_totals?granularity=month&from=2026-02-30&to=2026-03-01", http.StatusBadRequest},
	} {
		if got := status(tc.path); got != tc.status {
			t.Errorf("GET %s = %d, want %d", tc.path, got, tc.status)
		}
	}
}

// TestFinanceIncomeTotals checks GET /finance/income_totals (ADR-0067 §16): the holders of
// view_finance read the net and the balance whatever is published, the plans only while
// finance_viewer_income or income_visibility shows them, and never who paid what, the gross, or
// the fees.
func TestFinanceIncomeTotals(t *testing.T) {
	e, _ := newIncomeEnv(t)
	const path = "/finance/income_totals?granularity=month&from=2026-04-01&to=2026-05-31"
	out := readIncome(t, e, path, "carol-finance", "private, no-store")
	if len(out.Periods) != 2 || out.Periods[0].Balance == nil {
		t.Errorf("periods = %+v, want the net and the balance", out.Periods)
	}
	if out.Plans != nil {
		t.Errorf("plans with finance_viewer_income = totals: %+v", out.Plans)
	}
	setSetting(t, e, "finance_viewer_income", "plans")
	if out := readIncome(t, e, path, "carol-finance", "private, no-store"); out.Plans == nil || len(*out.Plans) != 1 {
		t.Errorf("plans with finance_viewer_income = plans: %+v", out.Plans)
	}
	setSetting(t, e, "finance_viewer_income", "totals")
	setSetting(t, e, "income_visibility", "plans")
	if out := readIncome(t, e, path, "carol-finance", "private, no-store"); out.Plans == nil {
		t.Error("no plans while income_visibility publishes them")
	}
	setSetting(t, e, "income_visibility", "private")
	if out := readIncome(t, e, path, "bob-finance", "private, no-store"); out.Plans == nil {
		t.Error("no plans for manage_finance")
	}
	if res := e.send(t, http.MethodGet, path, "dave-finance", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("GET %s without view_finance = %d, want 403", path, res.StatusCode)
	}
	if res := e.send(t, http.MethodGet, path, "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET %s anonymously = %d, want 401", path, res.StatusCode)
	}
}

// TestAdminIncome checks the administrators' income operations (ADR-0067 §16): manage_finance
// reads the gross, the fees, and each movement with its subscriber; view_finance reads neither.
func TestAdminIncome(t *testing.T) {
	e, income := newIncomeEnv(t)
	const totals = "/admin/income_totals?granularity=month&from=2026-04-01&to=2026-05-31"
	const movements = "/admin/income_movements?kind=payment&from=2026-04-01&to=2026-04-30"
	for _, p := range []string{totals, movements} {
		if res := e.send(t, http.MethodGet, p, "carol-admin", ""); res.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s with view_finance = %d, want 403", p, res.StatusCode)
		}
	}
	out := decode[clientapi.AdminIncomeTotals](t, e.send(t, http.MethodGet, totals, "bob-admin", ""))
	if len(out.Periods) != 2 || len(out.Plans) != 1 {
		t.Fatalf("admin totals = %+v", out)
	}
	april := out.Periods[0]
	if april.Net[0].Amount != "665" || april.Gross[0].Amount != "690" || april.Fee[0].Amount != "25" || april.Balance[0].Amount != "-335" {
		t.Errorf("April = %+v, want gross 690, fee 25, net 665, balance -335", april)
	}
	page := decode[clientapi.IncomeMovementPage](t, e.send(t, http.MethodGet, movements, "bob-admin", ""))
	if len(page.Items) != 1 {
		t.Fatalf("movements = %+v", page)
	}
	m := page.Items[0]
	if f := income.filters[len(income.filters)-1]; f.Kind != finance.IncomePayment || f.From != incomeDate("2026-04-01") || f.To != incomeDate("2026-04-30") {
		t.Errorf("filter = %+v, want the payments of April", f)
	}
	if m.ProviderId != incomeProviderID || m.AccountId == nil || *m.AccountId != incomeAccount || m.Username == nil || *m.Username != incomeUsername || m.SubscriptionId == nil ||
		m.Fee.Amount != "432" || m.CoversStartsOn == nil || *m.CoversStartsOn != "2026-04-10" || m.OccurredOn != "2026-04-10" {
		t.Errorf("movement = %+v", m)
	}
	// A movement whose subscriber's account is gone keeps its subscription and shows no account.
	all := decode[clientapi.IncomeMovementPage](t, e.send(t, http.MethodGet, "/admin/income_movements", "bob-admin", ""))
	if f := income.filters[len(income.filters)-1]; f.Kind != "" || f.From != finance.MinDate || f.To != finance.MaxDate {
		t.Errorf("filter = %+v, want every movement", f)
	}
	if len(all.Items) != 2 || all.Items[0].SubscriptionId == nil || all.Items[0].AccountId != nil || all.Items[0].Username != nil {
		t.Errorf("movements = %+v, want the refund without its subscriber's account first", all.Items)
	}
	for _, p := range []string{"/admin/income_movements?kind=payout", "/admin/income_movements?from=2026-02-30"} {
		if res := e.send(t, http.MethodGet, p, "bob-admin", ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", p, res.StatusCode)
		}
	}
}
