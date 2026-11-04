// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"errors"
	"testing"
)

func dateRange(t *testing.T, from, to string) *DateRange {
	t.Helper()
	return &DateRange{Start: date(t, from), End: date(t, to)}
}

// incomeNet returns the net of currency of each period of IncomeTotals, by label.
func incomeNet(t *testing.T, ms []IncomeMovement, basis Basis, g Granularity, from, to string, currency Currency) map[string]int64 {
	t.Helper()
	periods, err := g.Periods(date(t, from), date(t, to))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, p := range IncomeTotals(ms, basis, periods) {
		for _, m := range p.Net {
			if m.Currency == currency {
				out[p.Period.Label] = m.Minor
			}
		}
	}
	return out
}

func sum(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}

// The example of ADR-0067 §15: a yearly payment of 12,000 yen with a fee of 432 yen on
// 2026-04-10, refunded in full on 2026-06-01.
func adrExample(t *testing.T) (payment, refund IncomeMovement) {
	covers := dateRange(t, "2026-04-10", "2027-04-09")
	payment = IncomeMovement{Kind: IncomePayment, OccurredOn: date(t, "2026-04-10"), Covers: covers, Gross: jpy(12000), Fee: jpy(432), Net: jpy(11568)}
	refund = IncomeMovement{Kind: IncomeRefund, OccurredOn: date(t, "2026-06-01"), ParentCovers: covers, Gross: jpy(-12000), Fee: jpy(0), Net: jpy(-12000)}
	return payment, refund
}

func TestIncomeTotalsAmortizesAndCancelsRefunds(t *testing.T) {
	payment, refund := adrExample(t)
	alone := incomeNet(t, []IncomeMovement{payment}, BasisAmortized, GranularityMonth, "2026-01-01", "2027-12-31", "JPY")
	got := incomeNet(t, []IncomeMovement{payment, refund}, BasisAmortized, GranularityMonth, "2026-01-01", "2027-12-31", "JPY")
	if n := alone["2026-04"] + alone["2026-05"]; n != 1648 {
		t.Errorf("April and May of the payment = %d, want floor(11568·52/365) = 1648", n)
	}
	// The refund leaves the months before it as they were.
	for _, label := range []string{"2026-04", "2026-05"} {
		if got[label] != alone[label] {
			t.Errorf("%s with the refund = %d, want %d as before it", label, got[label], alone[label])
		}
	}
	// June carries the refund's part of April and May (floor(-12000·52/365) = -1710) with the
	// days of June of both.
	june := Spread(11568, *payment.Covers, *dateRange(t, "2026-06-01", "2026-06-30")) - 1710 +
		Spread(-12000, *payment.Covers, *dateRange(t, "2026-06-01", "2026-06-30"))
	if got["2026-06"] != june {
		t.Errorf("June = %d, want %d", got["2026-06"], june)
	}
	if got["2026-06"] >= -1710 {
		t.Errorf("June = %d, want the refund of the past days and the fee kept on the later ones", got["2026-06"])
	}
	// The fee that the refund does not give back is all that is left.
	if s := sum(got); s != -432 {
		t.Errorf("sum of the months = %d, want -432", s)
	}
	weeks := incomeNet(t, []IncomeMovement{payment, refund}, BasisAmortized, GranularityWeek, "2026-04-01", "2027-04-30", "JPY")
	if s := sum(weeks); s != -432 {
		t.Errorf("sum of the weeks = %d, want -432", s)
	}
	years := incomeNet(t, []IncomeMovement{payment, refund}, BasisAmortized, GranularityYear, "2026-01-01", "2027-12-31", "JPY")
	if s := sum(years); s != -432 {
		t.Errorf("sum of the years = %d, want -432", s)
	}
	// After the refund, each later month is the fee the payment keeps, never the payment.
	for _, label := range []string{"2026-08", "2027-01", "2027-04"} {
		if got[label] > 0 || got[label] < -50 {
			t.Errorf("%s = %d, want the small negative difference of the days", label, got[label])
		}
	}
	cash := incomeNet(t, []IncomeMovement{payment, refund}, BasisCash, GranularityMonth, "2026-01-01", "2027-12-31", "JPY")
	if cash["2026-04"] != 11568 || cash["2026-06"] != -12000 || len(cash) != 2 {
		t.Errorf("cash = %v, want 11568 in April and -12000 in June", cash)
	}
}

func TestIncomeTotalsPlacement(t *testing.T) {
	monthly := dateRange(t, "2026-04-25", "2026-05-24")
	yearly := dateRange(t, "2026-04-01", "2027-03-31")
	for _, tc := range []struct {
		name  string
		ms    []IncomeMovement
		basis Basis
		want  map[string]int64
	}{
		{
			name:  "a monthly payment stays on its date",
			ms:    []IncomeMovement{{Kind: IncomePayment, OccurredOn: date(t, "2026-04-25"), Covers: monthly, Gross: jpy(500), Fee: jpy(18), Net: jpy(482)}},
			basis: BasisAmortized, want: map[string]int64{"2026-04": 482},
		},
		{
			name:  "a payment without a period stays on its date",
			ms:    []IncomeMovement{{Kind: IncomePayment, OccurredOn: date(t, "2026-05-02"), Gross: jpy(500), Fee: jpy(18), Net: jpy(482)}},
			basis: BasisAmortized, want: map[string]int64{"2026-05": 482},
		},
		{
			name: "a refund of a monthly payment falls in its own month",
			ms: []IncomeMovement{
				{Kind: IncomePayment, OccurredOn: date(t, "2026-04-25"), Covers: monthly, Gross: jpy(500), Fee: jpy(18), Net: jpy(482)},
				{Kind: IncomeRefund, OccurredOn: date(t, "2026-05-03"), ParentCovers: monthly, Gross: jpy(-500), Fee: jpy(0), Net: jpy(-500)},
			},
			basis: BasisAmortized, want: map[string]int64{"2026-04": 482, "2026-05": -500},
		},
		{
			name:  "a fee of no payment falls on its date",
			ms:    []IncomeMovement{{Kind: IncomeFee, OccurredOn: date(t, "2026-06-30"), Gross: jpy(0), Fee: jpy(84), Net: jpy(-84)}},
			basis: BasisAmortized, want: map[string]int64{"2026-06": -84},
		},
		{
			name:  "a yearly payment spreads over its year",
			ms:    []IncomeMovement{{Kind: IncomePayment, OccurredOn: date(t, "2026-04-01"), Covers: yearly, Gross: jpy(3650), Fee: jpy(0), Net: jpy(3650)}},
			basis: BasisAmortized, want: map[string]int64{"2026-04": 300, "2026-05": 310, "2026-06": 300},
		},
		{
			name: "a chargeback and its reversal of a yearly payment cancel each other from their dates on",
			ms: []IncomeMovement{
				{Kind: IncomePayment, OccurredOn: date(t, "2026-04-01"), Covers: yearly, Gross: jpy(3650), Fee: jpy(0), Net: jpy(3650)},
				{Kind: IncomeChargeback, OccurredOn: date(t, "2026-05-01"), ParentCovers: yearly, Gross: jpy(-3650), Fee: jpy(1500), Net: jpy(-5150)},
				{Kind: IncomeChargebackReversal, OccurredOn: date(t, "2026-05-01"), ParentCovers: yearly, Gross: jpy(3650), Fee: jpy(0), Net: jpy(3650)},
			},
			// May carries the payment's May, and the chargeback and the reversal of April and May
			// by the payment's days: 310 + floor(-5150·61/365) + 3650·61/365 = 310 - 861 + 610.
			basis: BasisAmortized, want: map[string]int64{"2026-04": 300, "2026-05": 59},
		},
		{
			name: "cash places a yearly payment and its partial refund on their dates",
			ms: []IncomeMovement{
				{Kind: IncomePayment, OccurredOn: date(t, "2026-04-01"), Covers: yearly, Gross: jpy(3650), Fee: jpy(0), Net: jpy(3650)},
				{Kind: IncomeRefund, OccurredOn: date(t, "2026-05-15"), ParentCovers: yearly, Gross: jpy(-1000), Fee: jpy(0), Net: jpy(-1000)},
			},
			basis: BasisCash, want: map[string]int64{"2026-04": 3650, "2026-05": -1000},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := incomeNet(t, tc.ms, tc.basis, GranularityMonth, "2026-04-01", "2026-06-30", "JPY")
			for label, want := range tc.want {
				if got[label] != want {
					t.Errorf("%s = %d, want %d (all: %v)", label, got[label], want, got)
				}
			}
		})
	}
}

func TestIncomeTotalsFeeAndCurrencies(t *testing.T) {
	yearly := dateRange(t, "2026-04-10", "2027-04-09")
	ms := []IncomeMovement{
		{Kind: IncomePayment, OccurredOn: date(t, "2026-04-10"), Covers: yearly, Gross: jpy(12000), Fee: jpy(432), Net: jpy(11568)},
		{Kind: IncomePayment, OccurredOn: date(t, "2026-04-12"), Gross: Money{Minor: 500, Currency: "USD"}, Fee: Money{Minor: 45, Currency: "USD"}, Net: Money{Minor: 455, Currency: "USD"}},
	}
	periods, err := GranularityMonth.Periods(date(t, "2026-04-01"), date(t, "2027-04-30"))
	if err != nil {
		t.Fatal(err)
	}
	var fee int64
	for _, p := range IncomeTotals(ms, BasisAmortized, periods) {
		for i, g := range p.Gross {
			n, f := p.Net[i], p.Fee[i]
			if n.Currency != g.Currency || f.Currency != g.Currency || g.Minor-f.Minor != n.Minor {
				t.Errorf("%s: gross %v, fee %v, net %v do not agree", p.Period.Label, g, f, n)
			}
			if g.Currency == "JPY" {
				fee += f.Minor
			}
		}
		if p.Period.Label == "2026-04" && (len(p.Net) != 2 || p.Net[0].Currency != "JPY" || p.Net[1] != (Money{Minor: 455, Currency: "USD"})) {
			t.Errorf("April = %v, want JPY and USD by code, never converted", p.Net)
		}
	}
	if fee != 432 {
		t.Errorf("fee of the year = %d, want 432", fee)
	}
}

func TestIncomeMovementValidate(t *testing.T) {
	ok := IncomeMovement{Kind: IncomePayment, OccurredOn: date(t, "2026-04-10"), Covers: dateRange(t, "2026-04-10", "2026-05-09"), Gross: jpy(500), Fee: jpy(18), Net: jpy(482)}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate = %v", err)
	}
	for _, tc := range []struct {
		name string
		edit func(m *IncomeMovement)
		want error
	}{
		{"unknown kind", func(m *IncomeMovement) { m.Kind = "payout" }, ErrInvalidIncome},
		{"net is not gross less fee", func(m *IncomeMovement) { m.Net = jpy(500) }, ErrInvalidIncome},
		{"mixed currencies", func(m *IncomeMovement) { m.Fee = Money{Minor: 18, Currency: "USD"} }, ErrInvalidIncome},
		{"unknown currency", func(m *IncomeMovement) {
			m.Gross.Currency, m.Fee.Currency, m.Net.Currency = "ABC", "ABC", "ABC"
		}, ErrInvalidCurrency},
		{"a refund with a period", func(m *IncomeMovement) { m.Kind, m.Gross, m.Fee, m.Net = IncomeRefund, jpy(-500), jpy(0), jpy(-500) }, ErrInvalidIncome},
		{"a period that ends before it starts", func(m *IncomeMovement) { m.Covers = dateRange(t, "2026-05-09", "2026-04-10") }, ErrInvalidIncome},
	} {
		m := ok
		tc.edit(&m)
		if err := m.Validate(); !errors.Is(err, tc.want) {
			t.Errorf("%s: Validate = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestBalance(t *testing.T) {
	got := Balance([]Money{jpy(1000), {Minor: 500, Currency: "USD"}}, []Money{{Minor: 300, Currency: "EUR"}, jpy(1500)})
	want := []Money{{Minor: -300, Currency: "EUR"}, jpy(-500), {Minor: 500, Currency: "USD"}}
	if len(got) != len(want) {
		t.Fatalf("Balance = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Balance = %v, want %v", got, want)
		}
	}
}
