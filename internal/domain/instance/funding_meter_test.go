// SPDX-License-Identifier: AGPL-3.0-only

package instance_test

import (
	"errors"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
)

// The funding meter's choices follow the publication of the costs and the income (ADR-0067 §22):
// percent only while both are private, amounts only while both are public, neither while only one
// is, and a stored choice that the publication no longer allows reads as off.
func TestFundingMeterAllowed(t *testing.T) {
	settings := func(finance, income string, meter instance.FundingMeterVisibility) instance.Settings {
		return instance.Settings{}.
			With(instance.SettingFinanceVisibility, finance).
			With(instance.SettingIncomeVisibility, income).
			With(instance.SettingFundingMeterVisibility, string(meter))
	}
	for _, tc := range []struct {
		finance, income  string
		percent, amounts bool
	}{
		{"private", "private", true, false},
		{"totals", "private", false, false},
		{"private", "plans", false, false},
		{"totals", "totals", false, true},
		{"breakdown", "plans", false, true},
	} {
		for meter, allowed := range map[instance.FundingMeterVisibility]bool{
			instance.FundingMeterOff: true, instance.FundingMeterPercent: tc.percent, instance.FundingMeterAmounts: tc.amounts,
		} {
			s := settings(tc.finance, tc.income, meter)
			if got := s.FundingMeterAllowed(meter); got != allowed {
				t.Errorf("FundingMeterAllowed(%s) under %s / %s = %v, want %v", meter, tc.finance, tc.income, got, allowed)
			}
			want := meter
			if !allowed {
				want = instance.FundingMeterOff
			}
			if got := s.FundingMeterVisibility(); got != want {
				t.Errorf("FundingMeterVisibility() of %s under %s / %s = %q, want %q", meter, tc.finance, tc.income, got, want)
			}
			if got := s.FundingMeterTurnedOff(); got != !allowed {
				t.Errorf("FundingMeterTurnedOff() of %s under %s / %s = %v, want %v", meter, tc.finance, tc.income, got, !allowed)
			}
		}
	}
	var zero instance.Settings
	if zero.FundingMeterVisibility() != instance.FundingMeterOff || zero.FundingMeterRail() || !zero.FundingMeterTarget().IsCosts() {
		t.Error("the funding meter is not off, without the rail, against the costs by default")
	}
}

func TestFundingMeterTargetSettings(t *testing.T) {
	amount, err := instance.Lookup(instance.SettingFundingMeterTargetAmount)
	if err != nil {
		t.Fatal(err)
	}
	for raw, want := range map[string]string{"": "", "1650": "1650", "12.34": "12.34", "007.50": "7.50", "0": "0", "0.5": "0.5"} {
		if got, err := amount.Parse(raw); err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"-5", "1e3", "1.", ".5", "1.23456", "12,34", " 5", "12345678901234"} {
		if _, err := amount.Parse(raw); !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("Parse(%q) = %v, want ErrInvalidValue", raw, err)
		}
	}
	currency, err := instance.Lookup(instance.SettingFundingMeterTargetCurrency)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := currency.Parse("jpy"); err != nil || got != "JPY" {
		t.Errorf("Parse(jpy) = %q, %v; want JPY", got, err)
	}
	for _, raw := range []string{"JP", "YENS", "J1Y"} {
		if _, err := currency.Parse(raw); !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("Parse(%q) = %v, want ErrInvalidValue", raw, err)
		}
	}
	target := func(amount, currency string) instance.Settings {
		return instance.Settings{}.With(instance.SettingFundingMeterTargetAmount, amount).With(instance.SettingFundingMeterTargetCurrency, currency)
	}
	for _, ok := range []instance.Settings{target("", ""), target("5", "USD"), target("0.01", "USD"), target("5", ""), target("", "USD")} {
		if err := ok.CheckFundingMeterTarget(); err != nil {
			t.Errorf("CheckFundingMeterTarget(%+v) = %v", ok.FundingMeterTarget(), err)
		}
	}
	for _, bad := range []instance.Settings{target("0", "USD"), target("0.00", ""), target("0.00", "USD")} {
		if err := bad.CheckFundingMeterTarget(); !errors.Is(err, instance.ErrInvalidValue) {
			t.Errorf("CheckFundingMeterTarget(%+v) = %v, want ErrInvalidValue", bad.FundingMeterTarget(), err)
		}
	}
}
