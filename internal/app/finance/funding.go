// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"context"

	"github.com/mycroblog/my/internal/domain/finance"
)

// CheckFundingTarget checks the monthly target of the funding meter that the instance settings
// funding_meter_target_amount and funding_meter_target_currency hold (ADR-0067 §22): a currency
// that is an ISO 4217 code with minor units, and an amount with no more digits than it has. An
// empty currency passes: the amount is then in the currency of the month's costs, which only the
// meter knows. It fails with finance.ErrInvalidCurrency and the errors of finance.ParseMoney; the
// instance module calls it, through its port FundingTargets, before it records either setting.
func CheckFundingTarget(amount, currency string) error {
	switch {
	case currency == "":
		return nil
	case amount == "":
		_, err := finance.ParseCurrency(currency)
		return err
	}
	_, err := finance.ParseMoney(amount, currency)
	return err
}

// FundingTarget is the monthly target of the funding meter as the instance settings hold it: an
// amount in major units of Currency, or of the currency of the month's costs while Currency is
// empty; no Amount measures against the month's costs.
type FundingTarget struct {
	Amount, Currency string
}

// FundingMeter measures the net income of the month that holds today (UTC) against target
// (ADR-0067 §22), both prorated (finance.BasisAmortized) as the totals are. It returns false when
// the month has no target: the month's costs are not in one currency or not above zero while
// target needs them, or the amount has more digits than the currency of the costs has.
func (s *Service) FundingMeter(ctx context.Context, target FundingTarget) (finance.FundingMeter, bool, error) {
	today := finance.DateOf(s.d.Clock.Now().UTC())
	month, err := finance.GranularityMonth.PeriodOf(today)
	if err != nil {
		return finance.FundingMeter{}, false, err
	}
	q := TotalsQuery{Granularity: finance.GranularityMonth, From: today, To: today, Basis: finance.BasisAmortized}
	var costs finance.PeriodTotal
	if target.Amount == "" || target.Currency == "" {
		totals, err := s.Totals(ctx, q)
		if err != nil {
			return finance.FundingMeter{}, false, err
		}
		if len(totals) > 0 {
			costs = totals[0]
		}
	}
	var set *finance.Money
	if target.Amount != "" {
		currency := target.Currency
		if currency == "" {
			if len(costs.Totals) != 1 {
				return finance.FundingMeter{}, false, nil
			}
			currency = string(costs.Totals[0].Currency)
		}
		m, err := finance.ParseMoney(target.Amount, currency)
		if err != nil {
			return finance.FundingMeter{}, false, nil // CheckFundingTarget could not see the currency of the costs
		}
		set = &m
	}
	income, err := s.IncomeTotals(ctx, IncomeQuery{TotalsQuery: q})
	if err != nil {
		return finance.FundingMeter{}, false, err
	}
	var net []finance.Money
	if len(income) > 0 {
		net = income[0].Net
	}
	meter, ok := finance.MeasureFunding(month, net, costs, set)
	return meter, ok, nil
}
