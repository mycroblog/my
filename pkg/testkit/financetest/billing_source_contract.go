// SPDX-License-Identifier: AGPL-3.0-only

package financetest

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// BillingScenario is the provider's data that BillingSourceContract gives a factory: the billing
// months with their raw rows, which may offset each other and include credits, whether each month
// is final, how many billed pages a fetch of everything takes, and a failure of the provider.
type BillingScenario struct {
	// Location is UTC. The dates of the scenario are calendar dates in the source's own Location
	// (UTC for aws and the plugin of the tests, America/Los_Angeles for gcp): the fake provider of a
	// source serves them there.
	Location *time.Location
	Months   []BillingScenarioMonth
	// Pages is the number of billed pages (Cost Explorer pages, BigQuery queries) that a fetch of
	// the months takes, at least 1.
	Pages int
	// Failure, when not "", is the failure the provider answers with on the first billed request
	// (an error the adapter must classify as this kind): permission, not_enabled, not_found,
	// throttled, bytes_cap, invalid_response, or unavailable. A factory whose provider cannot
	// answer a kind calls t.Skip.
	Failure appfinance.ImportErrorKind
}

// BillingScenarioMonth is a billing month of a BillingScenario: open with data through Through (the
// last day with data), or Final.
type BillingScenarioMonth struct {
	Month   finance.YearMonth
	Final   bool
	Through finance.Date
	Rows    []FakeBillingRow
}

// BillingSourceFactory returns a source named aws, gcp, cloudflare, or plugin whose provider (a fake HTTP server,
// an emulator, a fake plugin) serves the scenario. The factory registers the cleanup of what it creates with
// t.Cleanup.
type BillingSourceFactory func(t *testing.T, s BillingScenario) appfinance.BillingSource

// BillingSourceContract verifies a BillingSource adapter (ADR-0068 §1): it returns the months
// asked for and no other, one line per (month, service, charge type) with the offsetting lines and
// credits summed, the covered days inside the month (the whole month once final, up to the last day
// with data before), the amounts in unrounded decimal, the final mark, known charge types and
// categories; it never makes more billed requests than MaxRequests, makes none for 0, and keeps no
// line when it stops at the cap; it classifies the provider's failures; and it stops when its
// context is canceled.
func BillingSourceContract(t *testing.T, factory BillingSourceFactory) {
	t.Helper()
	billingContract(t, factory, billingKeys)
}

// BillingTotalsSourceContract verifies a BillingSource adapter that knows only the total of each
// month, such as the Google Cloud budget notifications (billing_gcp_method = budget, ADR-0068 §7):
// what BillingSourceContract verifies, except that a month with costs is one line of charge type
// usage and category other whose amount is the sum of all the month's rows, credits and taxes
// included.
func BillingTotalsSourceContract(t *testing.T, factory BillingSourceFactory) {
	t.Helper()
	billingContract(t, factory, billingTotals)
}

// BillingUsageSourceContract verifies a BillingSource adapter whose provider reports usage charges
// alone, without credits, refunds, or taxes, such as Cloudflare's billable usage (ADR-0068 §11):
// what BillingSourceContract verifies, over a scenario that keeps only the rows of charge type
// usage, so the factory's provider never has to serve another kind.
func BillingUsageSourceContract(t *testing.T, factory BillingSourceFactory) {
	t.Helper()
	billingContract(t, func(t *testing.T, s BillingScenario) appfinance.BillingSource {
		months := make([]BillingScenarioMonth, 0, len(s.Months))
		for _, m := range s.Months {
			var rows []FakeBillingRow
			for _, r := range m.Rows {
				if r.ChargeType == finance.ChargeUsage {
					rows = append(rows, r)
				}
			}
			m.Rows = rows
			months = append(months, m)
		}
		s.Months = months
		return factory(t, s)
	}, billingUsage)
}

// billingShape is what the lines of a source hold.
type billingShape int

const (
	// billingKeys: one line per month, service, and charge type.
	billingKeys billingShape = iota
	// billingTotals: one total line per month.
	billingTotals
	// billingUsage: one line per month and service, of charge type usage alone.
	billingUsage
)

func billingContract(t *testing.T, factory BillingSourceFactory, shape billingShape) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, factory BillingSourceFactory, shape billingShape)
	}{
		{"ReturnsTheMonthsAskedFor", billingMonthsAskedFor},
		{"SumsEachKeyOnce", billingSumsKeys},
		{"CoversDaysInsideTheMonth", billingCovered},
		{"StaysWithinMaxRequests", billingMaxRequests},
		{"CallsNothingWithoutRequests", billingZeroRequests},
		{"ClassifiesFailures", billingFailures},
		{"StopsWhenCanceled", billingCanceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, factory, shape) })
	}
}

// billingLines is the number of lines of the months 2026-09 and 2026-10 of billingScenario: four
// keys, a total for each month, or the usage of one service in each month.
func billingLines(shape billingShape) int {
	if shape == billingKeys {
		return 4
	}
	return 2
}

// billingLocations are the time zones of the billing months of each source (ADR-0068 §1).
var billingLocations = map[string]string{"aws": "UTC", "gcp": "America/Los_Angeles", "cloudflare": "UTC", appfinance.SourcePlugin: "UTC"}

func billingMonth(t *testing.T, s string) finance.YearMonth {
	t.Helper()
	m, err := finance.ParseYearMonth(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// billingScenario has three months: an old final one that is never asked for, the final previous
// month, and the current month open through its 20th.
func billingScenario(t *testing.T) BillingScenario {
	t.Helper()
	return BillingScenario{
		Location: time.UTC,
		Pages:    1,
		Months: []BillingScenarioMonth{
			{Month: billingMonth(t, "2026-08"), Final: true, Rows: []FakeBillingRow{
				{Service: "Amazon Simple Storage Service", ChargeType: finance.ChargeUsage, Amount: "9.99", Currency: "USD"},
			}},
			{Month: billingMonth(t, "2026-09"), Final: true, Rows: []FakeBillingRow{
				{Service: "Amazon Elastic Compute Cloud - Compute", ChargeType: finance.ChargeUsage, Amount: "10.00", Currency: "USD"},
				{Service: "Amazon Elastic Compute Cloud - Compute", ChargeType: finance.ChargeUsage, Amount: "-10.00", Currency: "USD"},
				{Service: "Amazon Elastic Compute Cloud - Compute", ChargeType: finance.ChargeUsage, Amount: "3.1415926535", Currency: "USD"},
				{Service: "Amazon Elastic Compute Cloud - Compute", ChargeType: finance.ChargeCredit, Amount: "-1.5", Currency: "USD"},
				{Service: "Amazon Elastic Compute Cloud - Compute", ChargeType: finance.ChargeCredit, Amount: "-0.25", Currency: "USD"},
				{Service: "Tax", ChargeType: finance.ChargeTax, Amount: "0.31", Currency: "USD"},
			}},
			{Month: billingMonth(t, "2026-10"), Through: financeDate(t, "2026-10-20"), Rows: []FakeBillingRow{
				{Service: "Amazon Simple Storage Service", ChargeType: finance.ChargeUsage, Amount: "0.0049", Currency: "USD"},
			}},
		},
	}
}

func billingFetch(t *testing.T, src appfinance.BillingSource, maxRequests int, months ...string) (appfinance.Snapshot, error) {
	t.Helper()
	req := appfinance.FetchRequest{MaxRequests: maxRequests}
	for _, m := range months {
		req.Months = append(req.Months, billingMonth(t, m))
	}
	return src.Fetch(t.Context(), req)
}

func billingLinesByKey(t *testing.T, snap appfinance.Snapshot) map[string]appfinance.CostLine {
	t.Helper()
	out := map[string]appfinance.CostLine{}
	for _, l := range snap.Lines {
		k := l.Month.String() + "|" + l.Service + "|" + string(l.ChargeType)
		if _, ok := out[k]; ok {
			t.Errorf("the key %s comes twice", k)
		}
		out[k] = l
	}
	return out
}

func billingAmountIs(t *testing.T, what, got, want string) {
	t.Helper()
	g, err := finance.ParseDecimal(got)
	if err != nil {
		t.Errorf("%s: amount %q is not a decimal: %v", what, got, err)
		return
	}
	w, _ := new(big.Rat).SetString(want)
	if g.Cmp(w) != 0 {
		t.Errorf("%s: amount %s, want %s unrounded", what, got, want)
	}
}

func billingMonthsAskedFor(t *testing.T, factory BillingSourceFactory, shape billingShape) {
	s := billingScenario(t)
	src := factory(t, s)
	if _, ok := billingLocations[src.Name()]; !ok {
		t.Errorf("Name() = %q, want aws, gcp, cloudflare, or plugin", src.Name())
	}
	if want := billingLocations[src.Name()]; src.Location() == nil || src.Location().String() != want {
		t.Errorf("Location() = %v, want %s for %s", src.Location(), want, src.Name())
	}
	methods := []string{appfinance.MethodCostExplorer, appfinance.MethodBigQuery, appfinance.MethodBudget, appfinance.MethodBillableUsage, appfinance.MethodPlugin}
	if m := src.Method(); !slices.Contains(methods, m) {
		t.Errorf("Method() = %q, want one of %v", m, methods)
	}
	snap, err := billingFetch(t, src, 10, "2026-09", "2026-10")
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	for _, l := range snap.Lines {
		if m := l.Month.String(); m != "2026-09" && m != "2026-10" {
			t.Errorf("a line of %s, which was not asked for", m)
		}
	}
	if want := billingLines(shape); len(billingLinesByKey(t, snap)) != want {
		t.Errorf("Fetch = %d lines, want %d: %+v", len(snap.Lines), want, snap.Lines)
	}
	if snap.Requests < 1 || snap.Requests > 10 {
		t.Errorf("Requests = %d, want 1 to 10", snap.Requests)
	}
	only, err := billingFetch(t, src, 10, "2026-10")
	if err != nil {
		t.Fatalf("Fetch of one month = %v", err)
	}
	if len(only.Lines) != 1 || only.Lines[0].Month.String() != "2026-10" {
		t.Errorf("Fetch of 2026-10 = %+v, want its one line", only.Lines)
	}
	empty, err := billingFetch(t, src, 10, "2026-11")
	if err != nil || len(empty.Lines) != 0 {
		t.Errorf("Fetch of a month without costs = %+v, %v, want no line", empty.Lines, err)
	}
}

func billingSumsKeys(t *testing.T, factory BillingSourceFactory, shape billingShape) {
	snap, err := billingFetch(t, factory(t, billingScenario(t)), 10, "2026-09", "2026-10")
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	lines := billingLinesByKey(t, snap)
	if shape == billingTotals {
		billingSumsTotals(t, snap)
		return
	}
	want := map[string]string{
		"2026-09|Amazon Elastic Compute Cloud - Compute|usage":  "3.1415926535",
		"2026-09|Amazon Elastic Compute Cloud - Compute|credit": "-1.75",
		"2026-09|Tax|tax": "0.31",
		"2026-10|Amazon Simple Storage Service|usage": "0.0049",
	}
	if shape == billingUsage {
		delete(want, "2026-09|Amazon Elastic Compute Cloud - Compute|credit")
		delete(want, "2026-09|Tax|tax")
		for _, l := range snap.Lines {
			if l.ChargeType != finance.ChargeUsage {
				t.Errorf("a line of %s from a source of usage alone: %+v", l.ChargeType, l)
			}
		}
	}
	for k, want := range want {
		l, ok := lines[k]
		if !ok {
			t.Errorf("no line %s", k)
			continue
		}
		billingAmountIs(t, k, l.Amount, want)
		if l.Currency != "USD" {
			t.Errorf("%s: currency %q, want USD", k, l.Currency)
		}
		if !l.ChargeType.Valid() || !l.Category.Valid() {
			t.Errorf("%s: charge type %q, category %q", k, l.ChargeType, l.Category)
		}
	}
	if l, ok := lines["2026-09|Tax|tax"]; ok && l.Category != finance.CategoryOther {
		t.Errorf("the tax is in %q, want other", l.Category)
	}
}

// billingSumsTotals checks the lines of a source of totals: one usage line of category other a
// month, of the sum of the month's rows.
func billingSumsTotals(t *testing.T, snap appfinance.Snapshot) {
	t.Helper()
	byMonth := map[string]appfinance.CostLine{}
	for _, l := range snap.Lines {
		m := l.Month.String()
		if _, ok := byMonth[m]; ok {
			t.Errorf("%s has more than one line: %+v", m, snap.Lines)
		}
		byMonth[m] = l
	}
	for m, want := range map[string]string{"2026-09": "1.7015926535", "2026-10": "0.0049"} {
		l, ok := byMonth[m]
		if !ok {
			t.Errorf("no line of %s", m)
			continue
		}
		billingAmountIs(t, m, l.Amount, want)
		if l.Service == "" || l.ChargeType != finance.ChargeUsage || l.Category != finance.CategoryOther || l.Currency != "USD" {
			t.Errorf("%s: service %q, charge type %q, category %q, currency %q; want a service, usage, other, USD",
				m, l.Service, l.ChargeType, l.Category, l.Currency)
		}
	}
}

func billingCovered(t *testing.T, factory BillingSourceFactory, _ billingShape) {
	snap, err := billingFetch(t, factory(t, billingScenario(t)), 10, "2026-09", "2026-10")
	if err != nil {
		t.Fatalf("Fetch = %v", err)
	}
	for _, l := range snap.Lines {
		month := l.Month.Range()
		if !l.Covered.Valid() || !month.Contains(l.Covered.Start) || !month.Contains(l.Covered.End) {
			t.Errorf("%s %s covers %s, outside the month", l.Month, l.Service, l.Covered)
		}
		switch l.Month.String() {
		case "2026-09":
			if !l.Final || l.Covered != month {
				t.Errorf("the final month: %s %s final %v covers %s, want final over %s", l.Month, l.Service, l.Final, l.Covered, month)
			}
		case "2026-10":
			want := finance.DateRange{Start: month.Start, End: financeDate(t, "2026-10-20")}
			if l.Final || l.Covered != want {
				t.Errorf("the open month: final %v covers %s, want an estimate over %s", l.Final, l.Covered, want)
			}
		}
	}
}

func billingMaxRequests(t *testing.T, factory BillingSourceFactory, shape billingShape) {
	s := billingScenario(t)
	s.Pages = 3
	snap, err := billingFetch(t, factory(t, s), 3, "2026-09", "2026-10")
	if want := billingLines(shape); err != nil || snap.Requests < 1 || snap.Requests > 3 || len(snap.Lines) != want {
		t.Errorf("Fetch of 3 pages within 3 = %d lines, %d requests, %v; want %d lines", len(snap.Lines), snap.Requests, err, want)
	}
	snap, err = billingFetch(t, factory(t, s), 2, "2026-09", "2026-10")
	if !errors.Is(err, appfinance.ErrRequestCap) || appfinance.ImportErrorKindOf(err) != appfinance.ImportErrorRequestCap {
		t.Errorf("Fetch of 3 pages within 2 = %v, want ErrRequestCap", err)
	}
	if snap.Requests > 2 || len(snap.Lines) != 0 {
		t.Errorf("Fetch stopped at the cap: %d requests, %d lines; want at most 2 and none", snap.Requests, len(snap.Lines))
	}
}

func billingZeroRequests(t *testing.T, factory BillingSourceFactory, _ billingShape) {
	snap, err := billingFetch(t, factory(t, billingScenario(t)), 0, "2026-09", "2026-10")
	if !errors.Is(err, appfinance.ErrRequestCap) || snap.Requests != 0 || len(snap.Lines) != 0 {
		t.Errorf("Fetch with MaxRequests 0 = %d requests, %d lines, %v; want ErrRequestCap and nothing", snap.Requests, len(snap.Lines), err)
	}
}

func billingFailures(t *testing.T, factory BillingSourceFactory, _ billingShape) {
	for _, kind := range []appfinance.ImportErrorKind{
		appfinance.ImportErrorPermission, appfinance.ImportErrorNotEnabled, appfinance.ImportErrorNotFound,
		appfinance.ImportErrorThrottled, appfinance.ImportErrorBytesCap, appfinance.ImportErrorInvalidResponse,
		appfinance.ImportErrorUnavailable,
	} {
		t.Run(string(kind), func(t *testing.T) {
			s := billingScenario(t)
			s.Failure = kind
			snap, err := billingFetch(t, factory(t, s), 10, "2026-09", "2026-10")
			if got := appfinance.ImportErrorKindOf(err); got != kind {
				t.Errorf("Fetch = %v, classified %q, want %q", err, got, kind)
			}
			if len(snap.Lines) != 0 || snap.Requests > 10 {
				t.Errorf("a failed Fetch = %d lines, %d requests", len(snap.Lines), snap.Requests)
			}
		})
	}
}

func billingCanceled(t *testing.T, factory BillingSourceFactory, _ billingShape) {
	src := factory(t, billingScenario(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	snap, err := src.Fetch(ctx, appfinance.FetchRequest{Months: []finance.YearMonth{billingMonth(t, "2026-10")}, MaxRequests: 10})
	if !errors.Is(err, context.Canceled) || len(snap.Lines) != 0 {
		t.Errorf("Fetch with a canceled context = %d lines, %v; want context.Canceled", len(snap.Lines), err)
	}
}
