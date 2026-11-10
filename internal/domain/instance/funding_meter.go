// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"fmt"
	"strings"
)

// FundingMeterVisibility is the value of SettingFundingMeterVisibility (ADR-0067 §22): what the
// funding meter publishes about this month's net income against the monthly target.
type FundingMeterVisibility string

// Funding meter visibilities.
const (
	// FundingMeterOff publishes nothing.
	FundingMeterOff FundingMeterVisibility = "off"
	// FundingMeterPercent publishes the percentage alone, only while the costs and the income are
	// both private: no public amount combines with it into a private one.
	FundingMeterPercent FundingMeterVisibility = "percent"
	// FundingMeterAmounts publishes the percentage, the income, and the target, only while the
	// costs and the income are both public.
	FundingMeterAmounts FundingMeterVisibility = "amounts"
)

// The bounds of a KindDecimal value: 13 digits before the point keep any amount of four minor
// digits within an int64, and four are the most minor digits of an ISO 4217 currency (CLF, UYW).
const (
	MaxDecimalIntegerDigits  = 13
	MaxDecimalFractionDigits = 4
)

// canonicalDecimal returns raw without the leading zeros of its integer part ("007.50" is
// "7.50"), "" for "", and false for anything else than a decimal that is not negative within the
// bounds of KindDecimal. The digits after the point stay as written, so that the currency's
// digits can refuse "100.00" yen.
func canonicalDecimal(raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	whole, frac, dotted := strings.Cut(raw, ".")
	if !allDigits(whole) || (dotted && (!allDigits(frac) || len(frac) > MaxDecimalFractionDigits)) {
		return "", false
	}
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	if len(whole) > MaxDecimalIntegerDigits {
		return "", false
	}
	if dotted {
		return whole + "." + frac, true
	}
	return whole, true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// wellFormedCurrency reports whether code has the shape of an ISO 4217 alphabetic code: three
// upper case ASCII letters. Whether the code exists is the finance module's to say.
func wellFormedCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// FundingMeterAllowed reports whether v may be chosen under the current finance_visibility and
// income_visibility (ADR-0067 §22): percent only while both are private, amounts only while both
// are not, and off always. While only one side is public, the percentage would combine with its
// amounts into the other side's, so neither is allowed.
func (s Settings) FundingMeterAllowed(v FundingMeterVisibility) bool {
	costs := s.FinanceVisibility() != FinancePrivate
	income := s.IncomeVisibility() != IncomePrivate
	switch v {
	case FundingMeterOff:
		return true
	case FundingMeterPercent:
		return !costs && !income
	case FundingMeterAmounts:
		return costs && income
	}
	return false
}

// storedFundingMeterVisibility returns the stored value of SettingFundingMeterVisibility, off for
// an unexpected value, whether or not the visibilities of the costs and the income allow it.
func (s Settings) storedFundingMeterVisibility() FundingMeterVisibility {
	switch v, _ := s.Value(SettingFundingMeterVisibility); FundingMeterVisibility(v) {
	case FundingMeterPercent:
		return FundingMeterPercent
	case FundingMeterAmounts:
		return FundingMeterAmounts
	}
	return FundingMeterOff
}

// FundingMeterVisibility returns what the funding meter publishes: the value of
// SettingFundingMeterVisibility while FundingMeterAllowed allows it, off otherwise, so that a
// value stored before the costs or the income changed publication never publishes.
func (s Settings) FundingMeterVisibility() FundingMeterVisibility {
	if v := s.storedFundingMeterVisibility(); s.FundingMeterAllowed(v) {
		return v
	}
	return FundingMeterOff
}

// FundingMeterTarget is the monthly target of the funding meter: an amount in major units of
// Currency, or of the month's costs while Currency is empty; no Amount measures against the
// month's costs, whatever Currency is.
type FundingMeterTarget struct {
	Amount, Currency string
}

// IsCosts reports whether t measures against the month's costs.
func (t FundingMeterTarget) IsCosts() bool { return t.Amount == "" }

// FundingMeterTarget returns the values of SettingFundingMeterTargetAmount and
// SettingFundingMeterTargetCurrency.
func (s Settings) FundingMeterTarget() FundingMeterTarget {
	amount, _ := s.Value(SettingFundingMeterTargetAmount)
	currency, _ := s.Value(SettingFundingMeterTargetCurrency)
	return FundingMeterTarget{Amount: amount, Currency: currency}
}

// FundingMeterRail returns the value of SettingFundingMeterRail.
func (s Settings) FundingMeterRail() bool { return s.boolValue(SettingFundingMeterRail) }

// With returns a copy of s in which key holds value, for checking a change before it is
// recorded. value is canonical (Definition.Parse).
func (s Settings) With(key SettingKey, value string) Settings {
	out := Settings{stored: make(map[SettingKey]string, len(s.stored)+1)}
	for k, v := range s.stored {
		out.stored[k] = v
	}
	out.stored[key] = value
	return out
}

// CheckFundingMeterTarget refuses a target amount of zero (ADR-0067 §22). The finance module
// checks the currency and the digits of the amount in it.
func (s Settings) CheckFundingMeterTarget() error {
	if t := s.FundingMeterTarget(); !t.IsCosts() && strings.Trim(t.Amount, "0.") == "" {
		return fmt.Errorf("setting %s: %q is not greater than zero: %w", SettingFundingMeterTargetAmount, t.Amount, ErrInvalidValue)
	}
	return nil
}

// FundingMeterTurnedOff reports whether s, the settings after a change, stores a funding meter
// visibility that its finance_visibility and income_visibility no longer allow, which the change
// then turns off (ADR-0067 §22).
func (s Settings) FundingMeterTurnedOff() bool {
	return s.storedFundingMeterVisibility() != FundingMeterOff && s.FundingMeterVisibility() == FundingMeterOff
}
