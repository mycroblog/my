// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/finance"
)

// FinanceHarness is what FinanceRepositoryContract needs from a database adapter: its unit of
// work and the finance repository.
type FinanceHarness struct {
	UnitOfWork uow.UnitOfWork
	Finance    appfinance.Repository
}

// FinanceFactory returns a harness on a new, empty database whose UnitOfWork uses opts. The
// factory registers the cleanup of everything it creates with t.Cleanup.
type FinanceFactory func(t *testing.T, opts uow.Options) FinanceHarness

// FinanceRepositoryContract verifies the finance repository of a database adapter through the
// finance use cases (ADR-0067, ADR-0068, FR-901, FR-902, FR-904): cost items paid once, every N weeks, months, or
// years are recorded with their label, prices, and end, keep their calendar dates as written, and
// list newest first; invalid input writes nothing; the totals of any week, month, or year follow
// the rules of ADR-0067 (payment dates from the start date, month ends, leap years, the boundaries
// of the periods, the dates of the price revisions, the end date included, amortization only for
// schedules longer than a month) and every split of the periods sums to what was paid; a new
// label regroups the breakdown; a cancellation removes the item from past periods as well; and
// the head projections rebuild to what the commands and the imports wrote. The billing imports
// of a FakeBillingSource count once however often they run, record estimates, finals, and
// corrections (0 for a key that the source no longer reports), refetch the months of the rules,
// stop at the request caps by UTC day and month without calling the source, and record every
// failed attempt alone. The income records follow finance_income_contract.go.
func FinanceRepositoryContract(t *testing.T, newHarness FinanceFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, e *financeEnv)
	}{
		{"RecordsAndListsCostItems", financeRecord},
		{"RejectsInvalidInput", financeInvalid},
		{"TotalsFollowTheRules", financeTotals},
		{"SplitsSumToThePayments", financeSplits},
		{"RelabelsRevisesAndEnds", financeChanges},
		{"ShowsTheProviderAsEntered", financeProviderSpelling},
		{"CancelsFromEveryPeriod", financeCancel},
		{"RebuildsItsProjection", financeRebuild},
		{"ImportsWithoutDuplicates", financeImportIdempotent},
		{"ImportsEstimatesFinalsAndCorrections", financeImportRevisions},
		{"ImportsWithinTheRequestCaps", financeImportCaps},
		{"ReservesRequestsBeforeAnImport", financeImportReservations},
		{"RecordsFailedImports", financeImportFailures},
		{"RecordsIncomeOnceAndTotalsIt", financeIncomeRecords},
		{"ListsIncomeMovements", financeIncomeList},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newFinanceEnv(t, newHarness)) })
	}
}

type financeEnv struct {
	h     FinanceHarness
	clock *Clock
	svc   *appfinance.Service
	// aws is the billing source of svc, and caps its request caps, which a test may set.
	aws  *FakeBillingSource
	caps map[string]appfinance.RequestCaps
	// dir is the supporter directory of svc, which a test may fill.
	dir *FakeSupporterDirectory
	// meter records the metrics of svc.
	meter *Meter
}

func newFinanceEnv(t *testing.T, newHarness FinanceFactory) *financeEnv {
	t.Helper()
	projectors := &uow.Projectors{}
	h := newHarness(t, uow.Options{Retry: uow.RetryPolicy{MaxAttempts: 20}, Projectors: projectors})
	clock := NewClock(time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC))
	appfinance.RegisterProjectors(projectors, h.Finance)
	aws := NewFakeBillingSource("aws", time.UTC)
	caps := map[string]appfinance.RequestCaps{}
	dir := &FakeSupporterDirectory{}
	meter := NewMeter()
	svc := appfinance.NewService(appfinance.Deps{
		UnitOfWork: h.UnitOfWork, Repository: h.Finance, Clock: clock, IDs: NewIDGenerator(clock),
		Sources: []appfinance.BillingSource{aws}, Caps: caps, Supporters: dir, Logger: discardLogger(), Meter: meter,
	})
	return &financeEnv{h: h, clock: clock, svc: svc, aws: aws, caps: caps, dir: dir, meter: meter}
}

func financeDate(t *testing.T, s string) finance.Date {
	t.Helper()
	d, err := finance.ParseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func financeDatePtr(t *testing.T, s string) *finance.Date {
	t.Helper()
	d := financeDate(t, s)
	return &d
}

func financeJPY(n int64) finance.Money { return finance.Money{Minor: n, Currency: "JPY"} }

// create records a cost item from the CLI and advances the clock, so that the items list in
// the order they were created.
func (e *financeEnv) create(t *testing.T, name string, r finance.Recurrence, start string, price finance.Money) finance.CostItem {
	t.Helper()
	c, err := e.svc.Create(t.Context(), appfinance.CreateInput{
		Recurrence: r, StartsOn: financeDate(t, start), Price: price,
		Label: finance.Label{Name: name, Provider: "Provider " + name, Category: finance.CategoryOther},
	}, "")
	if err != nil {
		t.Fatalf("Create(%s) = %v", name, err)
	}
	e.clock.Advance(time.Second)
	return c
}

// totals returns the total of currency in each period of g from from to to under basis, by
// period label.
func (e *financeEnv) totals(t *testing.T, g finance.Granularity, from, to string, basis finance.Basis, currency finance.Currency) map[string]int64 {
	t.Helper()
	got, err := e.svc.Totals(t.Context(), appfinance.TotalsQuery{Granularity: g, From: financeDate(t, from), To: financeDate(t, to), Basis: basis})
	if err != nil {
		t.Fatalf("Totals(%s %s..%s %s) = %v", g, from, to, basis, err)
	}
	out := map[string]int64{}
	for _, p := range got {
		for _, m := range p.Totals {
			if m.Currency == currency {
				out[p.Period.Label] = m.Minor
			}
		}
	}
	return out
}

func checkTotals(t *testing.T, what string, got, want map[string]int64) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func financeRecord(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	once := e.create(t, "Logo", finance.Recurrence{Schedule: finance.ScheduleOnce, Interval: 1}, "2026-03-31", financeJPY(5000))
	weekly := e.create(t, "Backups", finance.Recurrence{Schedule: finance.ScheduleWeeks, Interval: 2}, "2026-04-06", financeJPY(300))
	monthly := e.create(t, "VPS", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, "2026-01-31",
		finance.Money{Minor: 1234, Currency: "USD"})
	c, err := e.svc.Create(ctx, appfinance.CreateInput{
		Recurrence: finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 1}, StartsOn: financeDate(t, "2024-02-29"),
		EndsOn: financeDatePtr(t, "9999-12-31"), Price: financeJPY(1650),
		Label: finance.Label{Name: " example.org ", Provider: "Ｒｅｇｉｓｔｒａｒ", Category: finance.CategoryDomain, Memo: "auto-renew"},
	}, NamedID("user-1"))
	if err != nil {
		t.Fatalf("Create(yearly) = %v", err)
	}
	got, err := e.svc.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if got.Recurrence != (finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 1}) || got.StartsOn.String() != "2024-02-29" ||
		got.EndsOn == nil || got.EndsOn.String() != "9999-12-31" || got.CreatedByUserID != NamedID("user-1") || !got.CreatedAt.Equal(e.clock.Now()) {
		t.Errorf("Get = %+v", got)
	}
	if got.Label != (finance.Label{Name: "example.org", Provider: "Ｒｅｇｉｓｔｒａｒ", Category: finance.CategoryDomain, Memo: "auto-renew"}) ||
		got.ProviderNormalized != "registrar" {
		t.Errorf("label = %+v, normalized %q", got.Label, got.ProviderNormalized)
	}
	if len(got.Prices) != 1 || got.Prices[0].EffectiveOn != got.StartsOn || got.Prices[0].Price != financeJPY(1650) {
		t.Errorf("prices = %+v", got.Prices)
	}
	if once.EndsOn != nil || once.Prices[0].EffectiveOn.String() != "2026-03-31" {
		t.Errorf("once = %+v", once)
	}

	// Newest first, in pages.
	page, err := e.svc.List(ctx, "", 3)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(items []finance.CostItem) []string {
		out := []string{}
		for _, c := range items {
			out = append(out, c.ID)
		}
		return out
	}
	if want := []string{c.ID, monthly.ID, weekly.ID}; !slices.Equal(ids(page), want) {
		t.Errorf("List = %v, want %v", ids(page), want)
	}
	rest, err := e.svc.List(ctx, page[len(page)-1].ID, 3)
	if err != nil || !slices.Equal(ids(rest), []string{once.ID}) {
		t.Errorf("List(after) = %v, %v; want %v", ids(rest), err, once.ID)
	}
	if _, err := e.svc.Get(ctx, NamedID("no-such-item")); !errors.Is(err, appfinance.ErrNotFound) {
		t.Errorf("Get(unknown) = %v, want ErrNotFound", err)
	}
}

func financeInvalid(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	ok := appfinance.CreateInput{
		Recurrence: finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, StartsOn: financeDate(t, "2026-04-01"),
		Price: financeJPY(100), Label: finance.Label{Name: "VPS", Provider: "Cloud", Category: finance.CategoryCompute},
	}
	for _, tc := range []struct {
		name string
		edit func(*appfinance.CreateInput)
		err  error
	}{
		{"once every 2", func(in *appfinance.CreateInput) {
			in.Recurrence = finance.Recurrence{Schedule: finance.ScheduleOnce, Interval: 2}
		}, finance.ErrInvalidRecurrence},
		{"every 100 months", func(in *appfinance.CreateInput) { in.Recurrence.Interval = 100 }, finance.ErrInvalidRecurrence},
		{"every day", func(in *appfinance.CreateInput) { in.Recurrence.Schedule = "days" }, finance.ErrInvalidRecurrence},
		{"a negative amount", func(in *appfinance.CreateInput) { in.Price = financeJPY(-1) }, finance.ErrInvalidAmount},
		{"an unknown currency", func(in *appfinance.CreateInput) { in.Price.Currency = "XAU" }, finance.ErrInvalidCurrency},
		{"no name", func(in *appfinance.CreateInput) { in.Label.Name = " " }, finance.ErrInvalidName},
		{"an unknown category", func(in *appfinance.CreateInput) { in.Label.Category = "food" }, finance.ErrInvalidCategory},
		{"an end before the start", func(in *appfinance.CreateInput) { in.EndsOn = financeDatePtr(t, "2026-03-31") }, finance.ErrBeforeStart},
	} {
		in := ok
		tc.edit(&in)
		if _, err := e.svc.Create(ctx, in, ""); !errors.Is(err, tc.err) {
			t.Errorf("Create(%s) = %v, want %v", tc.name, err, tc.err)
		}
	}
	if items, err := e.svc.List(ctx, "", 10); err != nil || len(items) != 0 {
		t.Fatalf("List after invalid input = %v, %v", items, err)
	}
	c, err := e.svc.Create(ctx, ok, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RevisePrice(ctx, c.ID, financeDate(t, "2026-03-31"), financeJPY(200), ""); !errors.Is(err, finance.ErrBeforeStart) {
		t.Errorf("RevisePrice(before the start) = %v", err)
	}
	if _, err := e.svc.SetEnd(ctx, c.ID, financeDatePtr(t, "2026-03-31"), ""); !errors.Is(err, finance.ErrBeforeStart) {
		t.Errorf("SetEnd(before the start) = %v", err)
	}
	if _, err := e.svc.Relabel(ctx, c.ID, finance.Label{Name: "VPS", Category: finance.CategoryCompute}, ""); !errors.Is(err, finance.ErrInvalidProvider) {
		t.Errorf("Relabel(no provider) = %v", err)
	}
	if _, err := e.svc.RevisePrice(ctx, NamedID("no-such-item"), financeDate(t, "2026-05-01"), financeJPY(200), ""); !errors.Is(err, appfinance.ErrNotFound) {
		t.Errorf("RevisePrice(unknown) = %v", err)
	}
	got, err := e.svc.Get(ctx, c.ID)
	if err != nil || len(got.Prices) != 1 || got.EndsOn != nil || got.Label.Provider != "Cloud" {
		t.Errorf("Get after rejected changes = %+v, %v", got, err)
	}
	for _, tc := range []struct {
		q   appfinance.TotalsQuery
		err error
	}{
		{appfinance.TotalsQuery{Granularity: "day", From: c.StartsOn, To: c.StartsOn, Basis: finance.BasisCash}, finance.ErrInvalidGranularity},
		{appfinance.TotalsQuery{Granularity: finance.GranularityMonth, From: c.StartsOn, To: c.StartsOn, Basis: "accrual"}, finance.ErrInvalidBasis},
		{appfinance.TotalsQuery{Granularity: finance.GranularityMonth, From: c.StartsOn, To: c.StartsOn.AddDays(-1), Basis: finance.BasisCash}, finance.ErrInvalidPeriod},
		{appfinance.TotalsQuery{Granularity: finance.GranularityYear, From: c.StartsOn, To: c.StartsOn.AddDays(365 * 21), Basis: finance.BasisCash}, finance.ErrTooManyPeriods},
	} {
		if _, err := e.svc.Totals(ctx, tc.q); !errors.Is(err, tc.err) {
			t.Errorf("Totals(%+v) = %v, want %v", tc.q, err, tc.err)
		}
	}
}

func financeTotals(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	cash, amortized := finance.BasisCash, finance.BasisAmortized
	month, week, year := finance.GranularityMonth, finance.GranularityWeek, finance.GranularityYear

	// A domain renewed every year: 1,650 yen on 2026-04-10 covers 2026-04-10 to 2027-04-09
	// (ADR-0067 §4), and 1,800 yen from the 2027 renewal on.
	domain := e.create(t, "example.org", finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 1}, "2026-04-10", financeJPY(1650))
	if _, err := e.svc.RevisePrice(ctx, domain.ID, financeDate(t, "2027-04-10"), financeJPY(1800), ""); err != nil {
		t.Fatal(err)
	}
	checkTotals(t, "domain by month, amortized", e.totals(t, month, "2026-03-01", "2026-06-30", amortized, "JPY"),
		map[string]int64{"2026-04": 94, "2026-05": 141, "2026-06": 135})
	checkTotals(t, "domain by month, cash", e.totals(t, month, "2026-03-01", "2026-06-30", cash, "JPY"),
		map[string]int64{"2026-04": 1650})
	// 2027 takes the 99 days of the first payment after 2026 and 266 of the 366 days of the second.
	checkTotals(t, "domain by year, amortized", e.totals(t, year, "2026-01-01", "2027-12-31", amortized, "JPY"),
		map[string]int64{"2026": 1202, "2027": 448 + 1308})
	checkTotals(t, "domain by year, cash", e.totals(t, year, "2026-01-01", "2027-12-31", cash, "JPY"),
		map[string]int64{"2026": 1650, "2027": 1800})
	if err := e.svc.Cancel(ctx, domain.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Every month from January 31: the last day of the shorter months, up to the end date
	// included; a raise from 2026-03-31 on. A month or shorter never amortizes.
	vps := e.create(t, "VPS", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, "2026-01-31", financeJPY(1000))
	if _, err := e.svc.RevisePrice(ctx, vps.ID, financeDate(t, "2026-03-31"), financeJPY(1200), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetEnd(ctx, vps.ID, financeDatePtr(t, "2026-04-30"), ""); err != nil {
		t.Fatal(err)
	}
	for _, basis := range []finance.Basis{cash, amortized} {
		checkTotals(t, "monthly by month, "+string(basis), e.totals(t, month, "2026-01-01", "2026-06-30", basis, "JPY"),
			map[string]int64{"2026-01": 1000, "2026-02": 1000, "2026-03": 1200, "2026-04": 1200})
	}
	// 2026-02-28 is a Saturday, in 2026-W09; 2026-03-31 a Tuesday, in 2026-W14.
	checkTotals(t, "monthly by week", e.totals(t, week, "2026-02-23", "2026-04-05", cash, "JPY"),
		map[string]int64{"2026-W09": 1000, "2026-W14": 1200})
	if err := e.svc.Cancel(ctx, vps.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Every 3 months amortizes over the days of its quarter (90, then 91); once stays on its day.
	quarterly := e.create(t, "Database", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 3}, "2026-01-01", financeJPY(9000))
	once := e.create(t, "Logo", finance.Recurrence{Schedule: finance.ScheduleOnce, Interval: 1}, "2026-03-31", financeJPY(5000))
	checkTotals(t, "quarterly and once by month, amortized", e.totals(t, month, "2026-01-01", "2026-04-30", amortized, "JPY"),
		map[string]int64{"2026-01": 3100, "2026-02": 2800, "2026-03": 3100 + 5000, "2026-04": 2967})
	checkTotals(t, "quarterly and once by month, cash", e.totals(t, month, "2026-01-01", "2026-04-30", cash, "JPY"),
		map[string]int64{"2026-01": 9000, "2026-03": 5000, "2026-04": 9000})
	for _, id := range []string{quarterly.ID, once.ID} {
		if err := e.svc.Cancel(ctx, id, ""); err != nil {
			t.Fatal(err)
		}
	}

	// Every 5 weeks amortizes (longer than a month), every 4 weeks does not.
	five := e.create(t, "Five", finance.Recurrence{Schedule: finance.ScheduleWeeks, Interval: 5}, "2026-04-06", financeJPY(3500))
	checkTotals(t, "every 5 weeks by week, amortized", e.totals(t, week, "2026-04-06", "2026-04-19", amortized, "JPY"),
		map[string]int64{"2026-W15": 700, "2026-W16": 700})
	checkTotals(t, "every 5 weeks by month, amortized", e.totals(t, month, "2026-04-01", "2026-05-31", amortized, "JPY"),
		map[string]int64{"2026-04": 2500, "2026-05": 1000 + 2100})
	if err := e.svc.Cancel(ctx, five.ID, ""); err != nil {
		t.Fatal(err)
	}
	four := e.create(t, "Four", finance.Recurrence{Schedule: finance.ScheduleWeeks, Interval: 4}, "2026-04-06", financeJPY(2800))
	checkTotals(t, "every 4 weeks by month, amortized", e.totals(t, month, "2026-04-01", "2026-05-31", amortized, "JPY"),
		map[string]int64{"2026-04": 2800, "2026-05": 2800})
	if err := e.svc.Cancel(ctx, four.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Every year from February 29: February 28 in the common years.
	leap := e.create(t, "Leap", finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 1}, "2024-02-29", financeJPY(100))
	checkTotals(t, "leap day by month, cash", e.totals(t, month, "2025-02-01", "2025-03-31", cash, "JPY"), map[string]int64{"2025-02": 100})
	checkTotals(t, "leap day by week, cash", e.totals(t, week, "2028-02-28", "2028-02-29", cash, "JPY"), map[string]int64{"2028-W09": 100})
	if err := e.svc.Cancel(ctx, leap.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Currencies stay apart.
	e.create(t, "Mail", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, "2026-01-15", finance.Money{Minor: 499, Currency: "USD"})
	e.create(t, "Name", finance.Recurrence{Schedule: finance.ScheduleOnce, Interval: 1}, "2026-01-20", financeJPY(300))
	got, err := e.svc.Totals(ctx, appfinance.TotalsQuery{Granularity: month, From: financeDate(t, "2026-01-01"), To: financeDate(t, "2026-01-31"), Basis: cash})
	if err != nil {
		t.Fatal(err)
	}
	if want := []finance.Money{financeJPY(300), {Minor: 499, Currency: "USD"}}; len(got) != 1 || !slices.Equal(got[0].Totals, want) {
		t.Errorf("Totals in two currencies = %+v, want %v", got, want)
	}
}

func financeSplits(t *testing.T, e *financeEnv) {
	start, end := "2025-11-30", "2028-03-31"
	items := []finance.CostItem{
		e.create(t, "Yearly", finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 1}, "2026-02-28", financeJPY(1999)),
		e.create(t, "Quarterly", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 3}, "2025-11-30", financeJPY(3001)),
		e.create(t, "Six weeks", finance.Recurrence{Schedule: finance.ScheduleWeeks, Interval: 6}, "2026-01-07", financeJPY(777)),
	}
	for _, c := range items {
		if _, err := e.svc.SetEnd(t.Context(), c.ID, financeDatePtr(t, "2027-01-31"), ""); err != nil {
			t.Fatal(err)
		}
	}
	// What was paid: every payment up to the end; the last ones cover dates up to 2028.
	var paid int64
	for _, c := range items {
		c.EndsOn = financeDatePtr(t, "2027-01-31")
		for _, p := range c.Payments(finance.DateRange{Start: finance.MinDate, End: finance.MaxDate}) {
			paid += p.Price.Minor
			if p.Covers.End.After(financeDate(t, end)) {
				t.Fatalf("a payment covers %s, past %s", p.Covers, end)
			}
		}
	}
	for _, g := range []finance.Granularity{finance.GranularityWeek, finance.GranularityMonth, finance.GranularityYear} {
		var sum int64
		for from := financeDate(t, start); !from.After(financeDate(t, end)); {
			p, err := g.PeriodOf(from)
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range e.totals(t, g, from.String(), from.String(), finance.BasisAmortized, "JPY") {
				sum += n
			}
			from = p.Range.End.AddDays(1)
		}
		if sum != paid {
			t.Errorf("the %ss sum to %d, paid %d", g, sum, paid)
		}
	}
}

func financeChanges(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	c := e.create(t, "Server", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, "2026-01-10", financeJPY(1000))
	// Two revisions on one date: the last recorded applies.
	for _, n := range []int64{1500, 1300} {
		if _, err := e.svc.RevisePrice(ctx, c.ID, financeDate(t, "2026-03-10"), financeJPY(n), ""); err != nil {
			t.Fatal(err)
		}
		e.clock.Advance(time.Second)
	}
	// A correction from the start date fixes every payment before the raise.
	if _, err := e.svc.RevisePrice(ctx, c.ID, c.StartsOn, financeJPY(900), ""); err != nil {
		t.Fatal(err)
	}
	checkTotals(t, "after the revisions", e.totals(t, finance.GranularityMonth, "2026-01-01", "2026-04-30", finance.BasisCash, "JPY"),
		map[string]int64{"2026-01": 900, "2026-02": 900, "2026-03": 1300, "2026-04": 1300})

	// The end date drops the payments after it, and no end brings them back.
	if _, err := e.svc.SetEnd(ctx, c.ID, financeDatePtr(t, "2026-03-09"), ""); err != nil {
		t.Fatal(err)
	}
	checkTotals(t, "after the end", e.totals(t, finance.GranularityMonth, "2026-01-01", "2026-04-30", finance.BasisCash, "JPY"),
		map[string]int64{"2026-01": 900, "2026-02": 900})
	got, err := e.svc.SetEnd(ctx, c.ID, nil, "")
	if err != nil || got.EndsOn != nil {
		t.Fatalf("SetEnd(nil) = %+v, %v", got.EndsOn, err)
	}
	checkTotals(t, "without an end", e.totals(t, finance.GranularityMonth, "2026-01-01", "2026-04-30", finance.BasisCash, "JPY"),
		map[string]int64{"2026-01": 900, "2026-02": 900, "2026-03": 1300, "2026-04": 1300})

	// A new label regroups every period under its provider and category.
	got, err = e.svc.Relabel(ctx, c.ID, finance.Label{Name: "Server", Provider: "Other Cloud", Category: finance.CategoryCompute, Memo: "moved"}, NamedID("user-1"))
	if err != nil || got.Label.Provider != "Other Cloud" || got.ProviderNormalized != "other cloud" || got.Label.Memo != "moved" {
		t.Fatalf("Relabel = %+v, %v", got, err)
	}
	totals, err := e.svc.Totals(ctx, appfinance.TotalsQuery{
		Granularity: finance.GranularityYear, From: financeDate(t, "2026-01-01"), To: financeDate(t, "2026-01-01"), Basis: finance.BasisCash,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []finance.Line{{
		Key: finance.BreakdownKey{Provider: "other cloud", Category: finance.CategoryCompute, Currency: "JPY"}, ProviderName: "Other Cloud",
		Amount: 900*2 + 1300*10,
	}}
	if len(totals) != 1 || !slices.Equal(totals[0].Breakdown, want) {
		t.Errorf("Breakdown = %+v, want %v", totals, want)
	}
	if len(got.Prices) != 4 {
		t.Errorf("prices = %+v, want the 4 revisions", got.Prices)
	}
}

// financeProviderSpelling checks that the spellings of a provider share one breakdown line, which
// shows the provider of the newest label revision among the items of the period (ADR-0098).
func financeProviderSpelling(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	monthly := finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}
	var ids []string
	for _, provider := range []string{"ConoHa", "conoha", "ＣｏｎｏＨａ"} {
		c, err := e.svc.Create(ctx, appfinance.CreateInput{
			Recurrence: monthly, StartsOn: financeDate(t, "2026-01-05"), Price: financeJPY(1000),
			Label: finance.Label{Name: "VPS " + provider, Provider: provider, Category: finance.CategoryCompute},
		}, "")
		if err != nil {
			t.Fatalf("Create(%s) = %v", provider, err)
		}
		ids = append(ids, c.ID)
		e.clock.Advance(time.Second)
	}
	breakdown := func() []finance.Line {
		t.Helper()
		got, err := e.svc.Totals(ctx, appfinance.TotalsQuery{
			Granularity: finance.GranularityMonth, From: financeDate(t, "2026-01-01"), To: financeDate(t, "2026-01-31"), Basis: finance.BasisCash, Items: true,
		})
		if err != nil || len(got) != 1 {
			t.Fatalf("Totals = %+v, %v", got, err)
		}
		for _, line := range got[0].Items {
			if line.ProviderName != got[0].Breakdown[0].ProviderName {
				t.Errorf("item line %+v, breakdown %+v", line, got[0].Breakdown)
			}
		}
		return got[0].Breakdown
	}
	line := func(name string) []finance.Line {
		return []finance.Line{{
			Key: finance.BreakdownKey{Provider: "conoha", Category: finance.CategoryCompute, Currency: "JPY"}, ProviderName: name, Amount: 3000,
		}}
	}
	if got := breakdown(); !slices.Equal(got, line("ＣｏｎｏＨａ")) {
		t.Errorf("breakdown = %+v, want the newest label's spelling", got)
	}
	// Relabeling an item makes its label the newest.
	if _, err := e.svc.Relabel(ctx, ids[0], finance.Label{Name: "VPS", Provider: "ConoHa", Category: finance.CategoryCompute}, ""); err != nil {
		t.Fatal(err)
	}
	if got := breakdown(); !slices.Equal(got, line("ConoHa")) {
		t.Errorf("breakdown after the relabel = %+v", got)
	}
}

func financeCancel(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	keep := e.create(t, "Keep", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, "2026-01-01", financeJPY(100))
	gone := e.create(t, "Gone", finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 1}, "2025-06-01", financeJPY(1200))
	checkTotals(t, "before the cancellation", e.totals(t, finance.GranularityYear, "2025-01-01", "2026-12-31", finance.BasisCash, "JPY"),
		map[string]int64{"2025": 1200, "2026": 1200 + 1200})
	if err := e.svc.Cancel(ctx, gone.ID, NamedID("user-1")); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	checkTotals(t, "after the cancellation", e.totals(t, finance.GranularityYear, "2025-01-01", "2026-12-31", finance.BasisCash, "JPY"),
		map[string]int64{"2026": 1200})
	if _, err := e.svc.Get(ctx, gone.ID); !errors.Is(err, appfinance.ErrNotFound) {
		t.Errorf("Get(canceled) = %v, want ErrNotFound", err)
	}
	if err := e.svc.Cancel(ctx, gone.ID, ""); !errors.Is(err, appfinance.ErrNotFound) {
		t.Errorf("Cancel(canceled) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.Relabel(ctx, gone.ID, finance.Label{Name: "Gone", Provider: "P", Category: finance.CategoryOther}, ""); !errors.Is(err, appfinance.ErrNotFound) {
		t.Errorf("Relabel(canceled) = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.SetEnd(ctx, gone.ID, nil, ""); !errors.Is(err, appfinance.ErrNotFound) {
		t.Errorf("SetEnd(canceled) = %v, want ErrNotFound", err)
	}
	items, err := e.svc.List(ctx, "", 10)
	if err != nil || len(items) != 1 || items[0].ID != keep.ID {
		t.Errorf("List = %v, %v; want only %s", items, err, keep.ID)
	}
}

func financeRebuild(t *testing.T, e *financeEnv) {
	ctx := t.Context()
	relabeled := e.create(t, "Relabeled", finance.Recurrence{Schedule: finance.ScheduleMonths, Interval: 1}, "2026-01-01", financeJPY(100))
	if _, err := e.svc.Relabel(ctx, relabeled.ID, finance.Label{Name: "Renamed", Provider: "P", Category: finance.CategoryEmail}, ""); err != nil {
		t.Fatal(err)
	}
	ended := e.create(t, "Ended", finance.Recurrence{Schedule: finance.ScheduleWeeks, Interval: 1}, "2026-01-05", financeJPY(100))
	if _, err := e.svc.SetEnd(ctx, ended.ID, financeDatePtr(t, "2026-06-30"), ""); err != nil {
		t.Fatal(err)
	}
	reopened := e.create(t, "Reopened", finance.Recurrence{Schedule: finance.ScheduleYears, Interval: 2}, "2026-01-05", financeJPY(100))
	if _, err := e.svc.SetEnd(ctx, reopened.ID, financeDatePtr(t, "2026-06-30"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetEnd(ctx, reopened.ID, nil, ""); err != nil {
		t.Fatal(err)
	}
	canceled := e.create(t, "Canceled", finance.Recurrence{Schedule: finance.ScheduleOnce, Interval: 1}, "2026-01-05", financeJPY(100))
	if err := e.svc.Cancel(ctx, canceled.ID, ""); err != nil {
		t.Fatal(err)
	}
	before, err := e.h.Finance.AllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 3 {
		t.Fatalf("AllItems = %d items, want 3", len(before))
	}
	if err := appfinance.RebuildProjections(ctx, e.h.UnitOfWork, e.h.Finance); err != nil {
		t.Fatalf("RebuildProjections = %v", err)
	}
	after, err := e.h.Finance.AllItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("after the rebuild:\n%+v\nwant\n%+v", after, before)
	}
}
