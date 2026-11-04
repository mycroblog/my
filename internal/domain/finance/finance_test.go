// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"errors"
	"math"
	"slices"
	"testing"
	"time"
)

func date(t *testing.T, s string) Date {
	t.Helper()
	d, err := ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate(%q) = %v", s, err)
	}
	return d
}

func datePtr(t *testing.T, s string) *Date {
	t.Helper()
	d := date(t, s)
	return &d
}

func jpy(n int64) Money { return Money{Minor: n, Currency: "JPY"} }

func TestParseDate(t *testing.T) {
	for _, s := range []string{"2026-04-10", "2024-02-29", "0001-01-01", "9999-12-31", "1970-01-01", "1969-12-31"} {
		d, err := ParseDate(s)
		if err != nil || d.String() != s {
			t.Errorf("ParseDate(%q) = %s, %v", s, d, err)
		}
	}
	for _, s := range []string{
		"", "2026-4-10", "2026/04/10", "2026-02-29", "2026-04-31", "2026-13-01", "2026-00-10", "2026-04-00",
		"0000-01-01", "+026-04-10", "2026-04-10T00:00:00Z", "２０２６-04-10",
	} {
		if _, err := ParseDate(s); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseDate(%q) = %v, want ErrInvalidDate", s, err)
		}
	}
	if got := DateOf(time.Date(2026, 4, 10, 23, 30, 0, 0, time.FixedZone("JST", 9*3600))); got.String() != "2026-04-10" {
		t.Errorf("DateOf = %s", got)
	}
	if d := date(t, "2026-04-10"); d.AddDays(-10).String() != "2026-03-31" || date(t, "2027-04-10").Sub(d) != 365 {
		t.Errorf("AddDays / Sub of %s", d)
	}
}

func TestRecurrenceValidate(t *testing.T) {
	for _, tc := range []struct {
		r  Recurrence
		ok bool
	}{
		{Recurrence{ScheduleOnce, 1}, true},
		{Recurrence{ScheduleOnce, 2}, false},
		{Recurrence{ScheduleWeeks, 1}, true},
		{Recurrence{ScheduleMonths, 99}, true},
		{Recurrence{ScheduleYears, 100}, false},
		{Recurrence{ScheduleYears, 0}, false},
		{Recurrence{"days", 1}, false},
	} {
		if err := tc.r.Validate(); (err == nil) != tc.ok || (err != nil && !errors.Is(err, ErrInvalidRecurrence)) {
			t.Errorf("%+v.Validate() = %v, want ok %v", tc.r, err, tc.ok)
		}
	}
}

// The payment dates count from the start date (ADR-0067 §3).
func TestOccurrence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		r     Recurrence
		start string
		want  []string
	}{
		{"once", Recurrence{ScheduleOnce, 1}, "2026-04-10", []string{"2026-04-10"}},
		{"every week", Recurrence{ScheduleWeeks, 1}, "2026-12-28", []string{"2026-12-28", "2027-01-04", "2027-01-11"}},
		{"every 2 weeks", Recurrence{ScheduleWeeks, 2}, "2026-04-06", []string{"2026-04-06", "2026-04-20", "2026-05-04"}},
		{
			"every month from the 31st", Recurrence{ScheduleMonths, 1}, "2026-01-31",
			[]string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31"},
		},
		{"every month from the 31st in a leap year", Recurrence{ScheduleMonths, 1}, "2028-01-31", []string{"2028-01-31", "2028-02-29", "2028-03-31"}},
		{"every month from the 30th", Recurrence{ScheduleMonths, 1}, "2026-01-30", []string{"2026-01-30", "2026-02-28", "2026-03-30"}},
		{"every 3 months across a year", Recurrence{ScheduleMonths, 3}, "2026-11-30", []string{"2026-11-30", "2027-02-28", "2027-05-30", "2027-08-30"}},
		{"every 18 months", Recurrence{ScheduleMonths, 18}, "2026-08-31", []string{"2026-08-31", "2028-02-29", "2029-08-31"}},
		{
			"every year from February 29", Recurrence{ScheduleYears, 1}, "2024-02-29",
			[]string{"2024-02-29", "2025-02-28", "2026-02-28", "2027-02-28", "2028-02-29"},
		},
		{"every 2 years", Recurrence{ScheduleYears, 2}, "2026-04-10", []string{"2026-04-10", "2028-04-10", "2030-04-10"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for k := range len(tc.want) + 1 {
				d, ok := tc.r.Occurrence(date(t, tc.start), k)
				if !ok {
					break
				}
				got = append(got, d.String())
			}
			if tc.r.Schedule != ScheduleOnce {
				got = got[:len(tc.want)]
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("occurrences = %v, want %v", got, tc.want)
			}
		})
	}
	if _, ok := (Recurrence{ScheduleYears, 1}).Occurrence(date(t, "9999-01-01"), 1); ok {
		t.Error("an occurrence past 9999-12-31")
	}
}

func TestAmortizes(t *testing.T) {
	for _, tc := range []struct {
		r    Recurrence
		want bool
	}{
		{Recurrence{ScheduleOnce, 1}, false},
		{Recurrence{ScheduleWeeks, 1}, false},
		{Recurrence{ScheduleWeeks, 4}, false},
		{Recurrence{ScheduleWeeks, 5}, true},
		{Recurrence{ScheduleMonths, 1}, false},
		{Recurrence{ScheduleMonths, 2}, true},
		{Recurrence{ScheduleYears, 1}, true},
	} {
		if got := tc.r.Amortizes(); got != tc.want {
			t.Errorf("%+v.Amortizes() = %v, want %v", tc.r, got, tc.want)
		}
	}
}

func TestPayments(t *testing.T) {
	item := func(r Recurrence, start string, end *Date, prices ...PriceRevision) CostItem {
		return CostItem{ID: testID(1), Recurrence: r, StartsOn: date(t, start), EndsOn: end, Prices: prices}
	}
	price := func(on string, n int64) PriceRevision { return PriceRevision{EffectiveOn: date(t, on), Price: jpy(n)} }
	type pay struct {
		on     string
		amount int64
		covers string
	}
	for _, tc := range []struct {
		name   string
		item   CostItem
		window string
		want   []pay
	}{
		{
			"the end date is included", item(Recurrence{ScheduleMonths, 1}, "2026-01-31", datePtr(t, "2026-04-30"), price("2026-01-31", 1000)),
			"2026-01-01/2026-12-31",
			[]pay{
				{"2026-01-31", 1000, "2026-01-31/2026-02-27"}, {"2026-02-28", 1000, "2026-02-28/2026-03-30"},
				{"2026-03-31", 1000, "2026-03-31/2026-04-29"}, {"2026-04-30", 1000, "2026-04-30/2026-05-30"},
			},
		},
		{
			"the day before the end date", item(Recurrence{ScheduleMonths, 1}, "2026-01-31", datePtr(t, "2026-04-29"), price("2026-01-31", 1000)),
			"2026-01-01/2026-12-31",
			[]pay{
				{"2026-01-31", 1000, "2026-01-31/2026-02-27"}, {"2026-02-28", 1000, "2026-02-28/2026-03-30"},
				{"2026-03-31", 1000, "2026-03-31/2026-04-29"},
			},
		},
		{
			"a revision applies from its date on",
			item(Recurrence{ScheduleMonths, 1}, "2026-01-15", nil, price("2026-01-15", 1000), price("2026-03-15", 1200)),
			"2026-02-01/2026-04-14",
			[]pay{
				{"2026-01-15", 1000, "2026-01-15/2026-02-14"}, {"2026-02-15", 1000, "2026-02-15/2026-03-14"},
				{"2026-03-15", 1200, "2026-03-15/2026-04-14"},
			},
		},
		{
			"a revision after the payment date",
			item(Recurrence{ScheduleMonths, 1}, "2026-01-15", nil, price("2026-01-15", 1000), price("2026-03-16", 1200)),
			"2026-03-15/2026-03-31",
			[]pay{{"2026-03-15", 1000, "2026-03-15/2026-04-14"}},
		},
		{
			"the last of the revisions of a date wins",
			item(Recurrence{ScheduleYears, 1}, "2026-04-10", nil, price("2026-04-10", 1650), price("2027-04-10", 1800), price("2026-04-10", 1500)),
			"2026-01-01/2027-12-31",
			[]pay{{"2026-04-10", 1500, "2026-04-10/2027-04-09"}, {"2027-04-10", 1800, "2027-04-10/2028-04-09"}},
		},
		{
			"a payment before the window that covers it",
			item(Recurrence{ScheduleYears, 1}, "2020-04-10", nil, price("2020-04-10", 1650)),
			"2026-01-01/2026-01-31",
			[]pay{{"2025-04-10", 1650, "2025-04-10/2026-04-09"}},
		},
		{
			"once", item(Recurrence{ScheduleOnce, 1}, "2026-04-10", nil, price("2026-04-10", 5000)),
			"2026-01-01/2026-12-31", []pay{{"2026-04-10", 5000, "2026-04-10/2026-04-10"}},
		},
		{
			"once outside the window", item(Recurrence{ScheduleOnce, 1}, "2026-04-10", nil, price("2026-04-10", 5000)),
			"2026-05-01/2026-12-31", nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.window
			got := tc.item.Payments(DateRange{Start: date(t, w[:10]), End: date(t, w[11:])})
			var gotPays []pay
			for _, p := range got {
				gotPays = append(gotPays, pay{p.On.String(), p.Price.Minor, p.Covers.String()})
			}
			if !slices.Equal(gotPays, tc.want) {
				t.Errorf("Payments = %v, want %v", gotPays, tc.want)
			}
		})
	}
}

func TestSpread(t *testing.T) {
	span := func(s string) DateRange { return DateRange{Start: date(t, s[:10]), End: date(t, s[11:])} }
	for _, tc := range []struct {
		name   string
		amount int64
		span   string
		part   string
		want   int64
	}{
		// ADR-0067 §4: 21 of 365 days.
		{"the example of the ADR", 1650, "2026-04-10/2027-04-09", "2026-04-01/2026-04-30", 94},
		{"the next month", 1650, "2026-04-10/2027-04-09", "2026-05-01/2026-05-31", 141},
		{"the last month", 1650, "2026-04-10/2027-04-09", "2027-04-01/2027-04-30", 1650 - 1609},
		{"the whole span", 1650, "2026-04-10/2027-04-09", "2026-01-01/2027-12-31", 1650},
		{"no common date", 1650, "2026-04-10/2027-04-09", "2027-04-10/2027-04-30", 0},
		{"one day", 300, "2026-04-10/2026-04-10", "2026-04-10/2026-04-10", 300},
		{"a credit", -1000, "2026-04-01/2026-04-30", "2026-04-01/2026-04-07", -234},
		{"a large amount", math.MaxInt64, "2026-01-01/2026-12-31", "2026-01-01/2026-12-31", math.MaxInt64},
		{"a large credit", math.MinInt64, "2026-01-01/2026-12-31", "2026-01-01/2026-12-31", math.MinInt64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Spread(tc.amount, span(tc.span), span(tc.part)); got != tc.want {
				t.Errorf("Spread(%d, %s, %s) = %d, want %d", tc.amount, tc.span, tc.part, got, tc.want)
			}
		})
	}
}

// However a span is cut into days, weeks, months, or years, the parts sum to the amount.
func TestSpreadSumsToTheAmount(t *testing.T) {
	spans := []DateRange{
		{Start: date(t, "2026-04-10"), End: date(t, "2027-04-09")},
		{Start: date(t, "2024-02-29"), End: date(t, "2025-02-27")},
		{Start: date(t, "2026-12-28"), End: date(t, "2027-01-03")},
		{Start: date(t, "2026-01-01"), End: date(t, "2026-03-31")},
	}
	for _, amount := range []int64{0, 1, 7, 1650, 99999, -12345, math.MaxInt64 / 3} {
		for _, span := range spans {
			var byDay int64
			for d := span.Start; !d.After(span.End); d = d.AddDays(1) {
				one := Spread(amount, span, DateRange{Start: d, End: d})
				if amount >= 0 && one < 0 {
					t.Fatalf("a negative day %s of %d", d, amount)
				}
				byDay += one
			}
			if byDay != amount {
				t.Errorf("days of %d over %s sum to %d", amount, span, byDay)
			}
			for _, g := range []Granularity{GranularityWeek, GranularityMonth, GranularityYear} {
				periods, err := g.Periods(span.Start, span.End)
				if err != nil {
					t.Fatal(err)
				}
				var sum int64
				for _, p := range periods {
					sum += Spread(amount, span, p.Range)
				}
				if sum != amount {
					t.Errorf("%ss of %d over %s sum to %d", g, amount, span, sum)
				}
			}
		}
	}
}

func TestPeriods(t *testing.T) {
	labels := func(ps []Period) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Label+" "+p.Range.String())
		}
		return out
	}
	for _, tc := range []struct {
		g        Granularity
		from, to string
		want     []string
	}{
		{GranularityWeek, "2026-12-30", "2027-01-05", []string{"2026-W53 2026-12-28/2027-01-03", "2027-W01 2027-01-04/2027-01-10"}},
		{GranularityWeek, "2026-04-12", "2026-04-13", []string{"2026-W15 2026-04-06/2026-04-12", "2026-W16 2026-04-13/2026-04-19"}},
		{GranularityMonth, "2028-02-10", "2028-03-01", []string{"2028-02 2028-02-01/2028-02-29", "2028-03 2028-03-01/2028-03-31"}},
		{GranularityYear, "2026-12-31", "2027-01-01", []string{"2026 2026-01-01/2026-12-31", "2027 2027-01-01/2027-12-31"}},
	} {
		got, err := tc.g.Periods(date(t, tc.from), date(t, tc.to))
		if err != nil || !slices.Equal(labels(got), tc.want) {
			t.Errorf("%s Periods(%s, %s) = %v, %v; want %v", tc.g, tc.from, tc.to, labels(got), err, tc.want)
		}
	}
	for _, tc := range []struct {
		g        Granularity
		from, to string
		err      error
	}{
		{GranularityMonth, "2026-02-01", "2026-01-31", ErrInvalidPeriod},
		{GranularityWeek, "2026-01-05", "2028-01-02", nil}, // 104 weeks
		{GranularityWeek, "2026-01-05", "2028-01-03", ErrTooManyPeriods},
		{GranularityMonth, "2026-01-01", "2030-12-31", nil},
		{GranularityMonth, "2026-01-01", "2031-01-01", ErrTooManyPeriods},
		{GranularityYear, "2026-01-01", "2045-12-31", nil},
		{GranularityYear, "2026-01-01", "2046-01-01", ErrTooManyPeriods},
		{"day", "2026-01-01", "2026-01-01", ErrInvalidGranularity},
	} {
		if _, err := tc.g.Periods(date(t, tc.from), date(t, tc.to)); !errors.Is(err, tc.err) {
			t.Errorf("%s Periods(%s, %s) = %v, want %v", tc.g, tc.from, tc.to, err, tc.err)
		}
	}
}

func TestTotals(t *testing.T) {
	seq := 0
	item := func(name string, r Recurrence, start string, end *Date, price Money, category Category) CostItem {
		seq++
		return CostItem{
			ID: testID(seq), Recurrence: r, StartsOn: date(t, start), EndsOn: end, ProviderNormalized: "provider " + name,
			Label:  Label{Name: name, Provider: "Provider " + name, Category: category},
			Prices: []PriceRevision{{EffectiveOn: date(t, start), Price: price}},
		}
	}
	domain := item("domain", Recurrence{ScheduleYears, 1}, "2026-04-10", nil, jpy(1650), CategoryDomain)
	vps := item("vps", Recurrence{ScheduleMonths, 1}, "2026-01-31", datePtr(t, "2026-04-30"), Money{Minor: 1234, Currency: "USD"}, CategoryCompute)
	weekly := item("weekly", Recurrence{ScheduleWeeks, 1}, "2026-04-06", nil, jpy(500), CategoryOther)
	fiveWeeks := item("five", Recurrence{ScheduleWeeks, 5}, "2026-04-06", nil, jpy(3500), CategoryStorage)
	fourWeeks := item("four", Recurrence{ScheduleWeeks, 4}, "2026-04-06", nil, jpy(2800), CategoryStorage)
	quarterly := item("quarterly", Recurrence{ScheduleMonths, 3}, "2026-01-01", nil, jpy(9000), CategoryDatabase)
	once := item("once", Recurrence{ScheduleOnce, 1}, "2026-03-31", nil, jpy(5000), CategorySoftware)

	type want struct {
		label  string
		totals []Money
	}
	for _, tc := range []struct {
		name     string
		items    []CostItem
		basis    Basis
		g        Granularity
		from, to string
		want     []want
	}{
		{
			"a yearly domain, amortized by month", []CostItem{domain}, BasisAmortized, GranularityMonth, "2026-03-01", "2026-05-31",
			[]want{{"2026-03", nil}, {"2026-04", []Money{jpy(94)}}, {"2026-05", []Money{jpy(141)}}},
		},
		{
			"a yearly domain, by cash", []CostItem{domain}, BasisCash, GranularityMonth, "2026-03-01", "2026-05-31",
			[]want{{"2026-03", nil}, {"2026-04", []Money{jpy(1650)}}, {"2026-05", nil}},
		},
		{
			"a yearly domain, amortized by year", []CostItem{domain}, BasisAmortized, GranularityYear, "2026-01-01", "2027-12-31",
			// The second year has 366 days (2028-02-29).
			[]want{{"2026", []Money{jpy(1202)}}, {"2027", []Money{jpy(448 + 1199)}}},
		},
		{
			"a yearly domain, by cash by year", []CostItem{domain}, BasisCash, GranularityYear, "2026-01-01", "2027-12-31",
			[]want{{"2026", []Money{jpy(1650)}}, {"2027", []Money{jpy(1650)}}},
		},
		{
			"monthly from the 31st with an end, the same under both bases", []CostItem{vps}, BasisAmortized, GranularityMonth,
			"2026-01-01", "2026-05-31",
			[]want{
				{"2026-01", []Money{{1234, "USD"}}}, {"2026-02", []Money{{1234, "USD"}}}, {"2026-03", []Money{{1234, "USD"}}},
				{"2026-04", []Money{{1234, "USD"}}}, {"2026-05", nil},
			},
		},
		{
			"weekly by month and by week", []CostItem{weekly}, BasisAmortized, GranularityMonth, "2026-04-01", "2026-04-30",
			[]want{{"2026-04", []Money{jpy(2000)}}},
		},
		{
			"weekly across a new year", []CostItem{weekly}, BasisCash, GranularityWeek, "2026-04-01", "2026-04-12",
			[]want{{"2026-W14", nil}, {"2026-W15", []Money{jpy(500)}}},
		},
		{
			"every 5 weeks amortizes", []CostItem{fiveWeeks}, BasisAmortized, GranularityMonth, "2026-04-01", "2026-05-31",
			[]want{{"2026-04", []Money{jpy(2500)}}, {"2026-05", []Money{jpy(1000 + 2100)}}},
		},
		{
			"every 5 weeks amortized by week", []CostItem{fiveWeeks}, BasisAmortized, GranularityWeek, "2026-04-06", "2026-04-19",
			[]want{{"2026-W15", []Money{jpy(700)}}, {"2026-W16", []Money{jpy(700)}}},
		},
		{
			"every 4 weeks stays on its dates", []CostItem{fourWeeks}, BasisAmortized, GranularityMonth, "2026-04-01", "2026-05-31",
			[]want{{"2026-04", []Money{jpy(2800)}}, {"2026-05", []Money{jpy(2800)}}},
		},
		{
			"quarterly amortized", []CostItem{quarterly}, BasisAmortized, GranularityMonth, "2026-01-01", "2026-03-31",
			[]want{{"2026-01", []Money{jpy(3100)}}, {"2026-02", []Money{jpy(2800)}}, {"2026-03", []Money{jpy(3100)}}},
		},
		{
			"once on the last day of a month", []CostItem{once}, BasisAmortized, GranularityMonth, "2026-03-01", "2026-04-30",
			[]want{{"2026-03", []Money{jpy(5000)}}, {"2026-04", nil}},
		},
		{
			"currencies stay apart", []CostItem{domain, vps, once}, BasisCash, GranularityYear, "2026-01-01", "2026-12-31",
			[]want{{"2026", []Money{jpy(1650 + 5000), {4 * 1234, "USD"}}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			periods, err := tc.g.Periods(date(t, tc.from), date(t, tc.to))
			if err != nil {
				t.Fatal(err)
			}
			got := Totals(tc.items, nil, tc.basis, periods)
			if len(got) != len(tc.want) {
				t.Fatalf("%d periods, want %d", len(got), len(tc.want))
			}
			for i, w := range tc.want {
				if got[i].Period.Label != w.label || !slices.Equal(got[i].Totals, w.totals) && (len(got[i].Totals) != 0 || len(w.totals) != 0) {
					t.Errorf("period %d = %s %v, want %s %v", i, got[i].Period.Label, got[i].Totals, w.label, w.totals)
				}
			}
		})
	}

	// The breakdown groups by provider, category, and currency.
	periods, _ := GranularityYear.Periods(date(t, "2026-01-01"), date(t, "2026-12-31"))
	got := Totals([]CostItem{domain, vps}, nil, BasisCash, periods)
	wantLines := []Line{
		{Key: BreakdownKey{Provider: "provider domain", Category: CategoryDomain, Currency: "JPY"}, ProviderName: "Provider domain", Amount: 1650},
		{Key: BreakdownKey{Provider: "provider vps", Category: CategoryCompute, Currency: "USD"}, ProviderName: "Provider vps", Amount: 4 * 1234},
	}
	if !slices.Equal(got[0].Breakdown, wantLines) {
		t.Errorf("Breakdown = %v, want %v", got[0].Breakdown, wantLines)
	}
}

// The weeks, months, and years of an amortized payment each sum to what was paid.
func TestTotalsSumToThePayments(t *testing.T) {
	for _, tc := range []struct {
		r     Recurrence
		start string
		end   string
	}{
		{Recurrence{ScheduleYears, 1}, "2024-02-29", "2027-02-28"},
		{Recurrence{ScheduleMonths, 3}, "2026-01-31", "2027-12-31"},
		{Recurrence{ScheduleWeeks, 6}, "2026-01-07", "2027-12-31"},
		{Recurrence{ScheduleMonths, 1}, "2026-01-31", "2027-12-31"},
	} {
		end := date(t, tc.end)
		c := CostItem{
			Recurrence: tc.r, StartsOn: date(t, tc.start), EndsOn: &end, Label: Label{Category: CategoryOther},
			Prices: []PriceRevision{{EffectiveOn: date(t, tc.start), Price: jpy(1999)}},
		}
		all := DateRange{Start: MinDate, End: MaxDate}
		var paid int64
		last := c.StartsOn
		for _, p := range c.Payments(all) {
			paid += p.Price.Minor
			last = p.Covers.End
		}
		for _, g := range []Granularity{GranularityWeek, GranularityMonth, GranularityYear} {
			// Every period from the start to the end of the last covered date, past MaxPeriods.
			var periods []Period
			for d := c.StartsOn; !d.After(last); {
				p, err := g.PeriodOf(d)
				if err != nil {
					t.Fatal(err)
				}
				periods = append(periods, p)
				d = p.Range.End.AddDays(1)
			}
			var sum int64
			for _, pt := range Totals([]CostItem{c}, nil, BasisAmortized, periods) {
				for _, m := range pt.Totals {
					sum += m.Minor
				}
			}
			if sum != paid {
				t.Errorf("%+v from %s: %ss sum to %d, paid %d", tc.r, tc.start, g, sum, paid)
			}
		}
	}
}

// The item lines of a period are the amount of each item and each imported service in it, and
// sum to the period's totals; the totals do not change (ADR-0070 §5).
func TestItemizedTotals(t *testing.T) {
	vps := CostItem{
		ID: testID(2), Recurrence: Recurrence{ScheduleMonths, 1}, StartsOn: date(t, "2026-04-15"), ProviderNormalized: "hetzner",
		Label:  Label{Name: "VPS", Provider: "Hetzner", Category: CategoryCompute, Memo: "fsn1"},
		Prices: []PriceRevision{{EffectiveOn: date(t, "2026-04-15"), Price: jpy(1000)}},
	}
	backup := vps
	backup.ID, backup.Label.Name, backup.Label.Memo = testID(1), "Backup", ""
	backup.Prices = []PriceRevision{{EffectiveOn: date(t, "2026-04-15"), Price: jpy(300)}}
	imported := []Entry{
		{
			Range: DateRange{Start: date(t, "2026-05-01"), End: date(t, "2026-05-31")}, Amount: Money{Minor: 250, Currency: "USD"},
			Provider: "google cloud", Category: CategoryStorage, Item: ItemRef{Key: "import:gcp:Cloud Storage", Name: "Cloud Storage"},
		},
		{
			Range: DateRange{Start: date(t, "2026-05-01"), End: date(t, "2026-05-31")}, Amount: Money{Minor: -50, Currency: "USD"},
			Provider: "google cloud", Category: CategoryStorage, Item: ItemRef{Key: "import:gcp:Cloud Storage", Name: "Cloud Storage"},
		},
	}
	periods, err := GranularityMonth.Periods(date(t, "2026-04-01"), date(t, "2026-05-31"))
	if err != nil {
		t.Fatal(err)
	}
	items := []CostItem{vps, backup}
	got := ItemizedTotals(items, imported, BasisCash, periods)
	plain := Totals(items, imported, BasisCash, periods)
	for i := range got {
		if !slices.Equal(got[i].Totals, plain[i].Totals) || !slices.Equal(got[i].Breakdown, plain[i].Breakdown) || plain[i].Items != nil {
			t.Errorf("period %s: itemized %+v, totals %+v", got[i].Period.Label, got[i], plain[i])
		}
	}
	want := [][]ItemLine{
		{
			{Item: ItemRef{Key: "item:" + testID(1), Name: "Backup"}, Provider: "hetzner", ProviderName: "Hetzner", Category: CategoryCompute, Amount: jpy(300)},
			{Item: ItemRef{Key: "item:" + testID(2), Name: "VPS", Memo: "fsn1"}, Provider: "hetzner", ProviderName: "Hetzner", Category: CategoryCompute, Amount: jpy(1000)},
		},
		{
			{Item: ItemRef{Key: "import:gcp:Cloud Storage", Name: "Cloud Storage"}, Provider: "google cloud", ProviderName: "google cloud", Category: CategoryStorage, Amount: Money{Minor: 200, Currency: "USD"}},
			{Item: ItemRef{Key: "item:" + testID(1), Name: "Backup"}, Provider: "hetzner", ProviderName: "Hetzner", Category: CategoryCompute, Amount: jpy(300)},
			{Item: ItemRef{Key: "item:" + testID(2), Name: "VPS", Memo: "fsn1"}, Provider: "hetzner", ProviderName: "Hetzner", Category: CategoryCompute, Amount: jpy(1000)},
		},
	}
	for i := range want {
		if !slices.Equal(got[i].Items, want[i]) {
			t.Errorf("period %s items = %+v, want %+v", got[i].Period.Label, got[i].Items, want[i])
		}
	}
}

// A breakdown line shows its provider as entered: the provider of the newest label revision
// among the cost items of the line's normalized provider that meet the period, then the larger
// revision ID, and an imported cost's provider name only without such an item (ADR-0067).
func TestBreakdownSpelling(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	item := func(id int, provider string, start string, end *Date, revisedAt time.Time, category Category) CostItem {
		return CostItem{
			ID: testID(id), Recurrence: Recurrence{ScheduleMonths, 1}, StartsOn: date(t, start), EndsOn: end, ProviderNormalized: "conoha",
			LabelRevisionID: testID(100 + id), LabelRevisedAt: revisedAt,
			Label:  Label{Name: "VPS", Provider: provider, Category: category},
			Prices: []PriceRevision{{EffectiveOn: date(t, start), Price: jpy(1000)}},
		}
	}
	// ＣｏｎｏＨａ has the newest label but stops in January; conoha and ConoHa share a time, and
	// ConoHa's revision ID is the larger.
	upper := item(1, "ConoHa", "2026-01-05", nil, at, CategoryCompute)
	lower := item(2, "conoha", "2026-01-10", nil, at, CategoryStorage)
	upper.LabelRevisionID, lower.LabelRevisionID = testID(200), testID(150)
	wide := item(3, "ＣｏｎｏＨａ", "2026-01-15", datePtr(t, "2026-01-31"), at.Add(time.Hour), CategoryCompute)
	imported := []Entry{{
		Range: DateRange{Start: date(t, "2026-01-01"), End: date(t, "2026-03-31")}, Amount: Money{Minor: 500, Currency: "USD"},
		Provider: "google cloud", Category: CategoryStorage, Spelling: Spelling{Name: "Google Cloud"},
		Item: ItemRef{Key: "import:gcp:Cloud Storage", Name: "Cloud Storage"},
	}, {
		Range: DateRange{Start: date(t, "2026-01-01"), End: date(t, "2026-01-31")}, Amount: Money{Minor: 100, Currency: "USD"},
		Provider: "conoha", Category: CategoryOther, Spelling: Spelling{Name: "CONOHA"},
		Item: ItemRef{Key: "import:x:VPS", Name: "VPS"},
	}}
	periods, err := GranularityMonth.Periods(date(t, "2026-01-01"), date(t, "2026-02-28"))
	if err != nil {
		t.Fatal(err)
	}
	got := ItemizedTotals([]CostItem{upper, lower, wide}, imported, BasisCash, periods)
	want := [][]Line{
		{
			{Key: BreakdownKey{Provider: "conoha", Category: CategoryCompute, Currency: "JPY"}, ProviderName: "ＣｏｎｏＨａ", Amount: 2000},
			{Key: BreakdownKey{Provider: "conoha", Category: CategoryOther, Currency: "USD"}, ProviderName: "ＣｏｎｏＨａ", Amount: 100},
			{Key: BreakdownKey{Provider: "conoha", Category: CategoryStorage, Currency: "JPY"}, ProviderName: "ＣｏｎｏＨａ", Amount: 1000},
			{Key: BreakdownKey{Provider: "google cloud", Category: CategoryStorage, Currency: "USD"}, ProviderName: "Google Cloud", Amount: 500 * 31 / 90},
		},
		{
			{Key: BreakdownKey{Provider: "conoha", Category: CategoryCompute, Currency: "JPY"}, ProviderName: "ConoHa", Amount: 1000},
			{Key: BreakdownKey{Provider: "conoha", Category: CategoryStorage, Currency: "JPY"}, ProviderName: "ConoHa", Amount: 1000},
			{Key: BreakdownKey{Provider: "google cloud", Category: CategoryStorage, Currency: "USD"}, ProviderName: "Google Cloud", Amount: 500*59/90 - 500*31/90},
		},
	}
	for i := range want {
		if !slices.Equal(got[i].Breakdown, want[i]) {
			t.Errorf("period %s breakdown = %+v, want %+v", got[i].Period.Label, got[i].Breakdown, want[i])
		}
		for _, line := range got[i].Items {
			if line.Provider == "conoha" && line.ProviderName != want[i][0].ProviderName {
				t.Errorf("period %s item %+v, want the spelling %q", got[i].Period.Label, line, want[i][0].ProviderName)
			}
		}
	}
	// Without a cost item, an imported cost shows its provider name.
	got = Totals(nil, imported[1:], BasisCash, periods[:1])
	if len(got[0].Breakdown) != 1 || got[0].Breakdown[0].ProviderName != "CONOHA" {
		t.Errorf("imported only = %+v", got[0].Breakdown)
	}
}

func TestLabelValidate(t *testing.T) {
	ok := Label{Name: "example.org", Provider: "Registrar", Category: CategoryDomain}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	for _, tc := range []struct {
		edit func(*Label)
		err  error
	}{
		{func(l *Label) { l.Name = "" }, ErrInvalidName},
		{func(l *Label) { l.Name = string(make([]rune, MaxName+1)) }, ErrInvalidName},
		{func(l *Label) { l.Name = "a\nb" }, ErrInvalidName},
		{func(l *Label) { l.Provider = "" }, ErrInvalidProvider},
		{func(l *Label) { l.Category = "food" }, ErrInvalidCategory},
		{func(l *Label) { l.Memo = "a\x00b" }, ErrInvalidMemo},
		{func(l *Label) { l.Memo = "\xff" }, ErrInvalidMemo},
	} {
		l := ok
		tc.edit(&l)
		if err := l.Validate(); !errors.Is(err, tc.err) {
			t.Errorf("Validate(%+v) = %v, want %v", l, err, tc.err)
		}
	}
	if l := (Label{Name: " a ", Provider: " b ", Memo: " c\n"}).Normalize(); l.Name != "a" || l.Provider != "b" || l.Memo != "c" {
		t.Errorf("Normalize = %+v", l)
	}
}

func TestCurrency(t *testing.T) {
	for _, tc := range []struct {
		code  string
		units int
	}{{"JPY", 0}, {"USD", 2}, {"EUR", 2}, {"KWD", 3}, {"CLF", 4}} {
		c, err := ParseCurrency(tc.code)
		if n, ok := c.MinorUnits(); err != nil || !ok || n != tc.units {
			t.Errorf("ParseCurrency(%s) = %s, %v; minor units %d", tc.code, c, err, n)
		}
	}
	for _, code := range []string{"", "jpy", "XAU", "XXX", "ABC", "US"} {
		if _, err := ParseCurrency(code); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("ParseCurrency(%q) = %v", code, err)
		}
	}
	if err := ValidatePrice(jpy(-1)); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("ValidatePrice(-1) = %v", err)
	}
	if !slices.IsSorted(Currencies()) {
		t.Error("Currencies is not sorted")
	}
}
