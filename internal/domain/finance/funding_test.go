// SPDX-License-Identifier: AGPL-3.0-only

package finance_test

import (
	"testing"

	"github.com/mycroblog/my/internal/domain/finance"
)

func TestProgress(t *testing.T) {
	yen := func(n int64) finance.Money { return finance.Money{Minor: n, Currency: "JPY"} }
	usd := func(n int64) finance.Money { return finance.Money{Minor: n, Currency: "USD"} }
	for _, tc := range []struct {
		name    string
		net     []finance.Money
		target  finance.Money
		want    int64
		covered bool
		netWant int64
	}{
		{name: "none", net: nil, target: yen(10000), want: 0, netWant: 0},
		{name: "rounded down", net: []finance.Money{yen(7299)}, target: yen(10000), want: 72, netWant: 7299},
		{name: "just short of the target", net: []finance.Money{yen(9999)}, target: yen(10000), want: 99, netWant: 9999},
		{name: "covered", net: []finance.Money{yen(10000)}, target: yen(10000), want: 100, covered: true, netWant: 10000},
		{name: "above the target", net: []finance.Money{yen(13050)}, target: yen(10000), want: 130, covered: true, netWant: 13050},
		{name: "negative after refunds", net: []finance.Money{yen(-500)}, target: yen(10000), want: 0, netWant: -500},
		{name: "other currencies do not count", net: []finance.Money{yen(5000), usd(100000)}, target: yen(10000), want: 50, netWant: 5000},
		{name: "only another currency", net: []finance.Money{usd(100000)}, target: yen(10000), want: 0, netWant: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := finance.Progress(tc.net, tc.target)
			if got.Percent != tc.want || got.Covered != tc.covered || got.Net.Minor != tc.netWant || got.Net.Currency != tc.target.Currency {
				t.Errorf("Progress(%v, %v) = %+v, want %d%% covered %v net %d", tc.net, tc.target, got, tc.want, tc.covered, tc.netWant)
			}
		})
	}
}

func TestMeasureFunding(t *testing.T) {
	today, err := finance.ParseDate("2026-11-10")
	if err != nil {
		t.Fatal(err)
	}
	month, err := finance.GranularityMonth.PeriodOf(today)
	if err != nil {
		t.Fatal(err)
	}
	yen := func(n int64) finance.Money { return finance.Money{Minor: n, Currency: "JPY"} }
	usd := func(n int64) finance.Money { return finance.Money{Minor: n, Currency: "USD"} }
	net := []finance.Money{yen(7250)}
	costs := finance.PeriodTotal{Period: month, Totals: []finance.Money{yen(10000)}, Estimated: true}

	got, ok := finance.MeasureFunding(month, net, costs, nil)
	if !ok || !got.TargetIsCosts || !got.Estimated || got.Target != yen(10000) || got.Percent != 72 || got.Covered {
		t.Errorf("against the costs = %+v, %v; want 72%% of the estimated 10,000 yen of costs", got, ok)
	}
	target := yen(5000)
	got, ok = finance.MeasureFunding(month, net, costs, &target)
	if !ok || got.TargetIsCosts || got.Estimated || got.Percent != 145 || !got.Covered {
		t.Errorf("against a target = %+v, %v; want 145%%, covered, without the estimate of the costs", got, ok)
	}
	for name, c := range map[string]finance.PeriodTotal{
		"no costs":         {Period: month},
		"two currencies":   {Period: month, Totals: []finance.Money{yen(10000), usd(500)}},
		"credits only":     {Period: month, Totals: []finance.Money{yen(-300)}},
		"costs of nothing": {Period: month, Totals: []finance.Money{yen(0)}},
	} {
		if got, ok := finance.MeasureFunding(month, net, c, nil); ok {
			t.Errorf("against %s = %+v, want no meter", name, got)
		}
	}
}
