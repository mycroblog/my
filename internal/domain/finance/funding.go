// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"math"
	"math/big"
)

// FundingProgress is how far a month's net income goes toward its target (ADR-0067 §22).
type FundingProgress struct {
	// Net is the month's net income in the target's currency; the income of other currencies
	// does not count, since no amount is converted (ADR-0067 §6).
	Net Money
	// Percent is Net over the target in whole percent, rounded down, so that 100 is reached only
	// when the month is covered; 0 while the net is not positive. It goes past 100.
	Percent int64
	// Covered reports that Net reaches the target.
	Covered bool
}

// Progress measures the net income of each currency against target, whose amount is positive.
func Progress(net []Money, target Money) FundingProgress {
	out := FundingProgress{Net: Money{Currency: target.Currency}}
	for _, m := range net {
		if m.Currency == target.Currency {
			out.Net.Minor += m.Minor
		}
	}
	out.Covered = out.Net.Minor >= target.Minor
	if out.Net.Minor > 0 && target.Minor > 0 {
		p := new(big.Int).Mul(big.NewInt(out.Net.Minor), big.NewInt(100))
		p.Quo(p, big.NewInt(target.Minor))
		out.Percent = math.MaxInt64
		if p.IsInt64() {
			out.Percent = p.Int64()
		}
	}
	return out
}

// FundingMeter is the funding meter of a month (ADR-0067 §22): its net income against its target.
type FundingMeter struct {
	Period Period
	// Target is the monthly target: the one the operator set, or the month's costs.
	Target Money
	// TargetIsCosts reports that Target is the month's costs.
	TargetIsCosts bool
	// Estimated reports that Target is the month's costs and an estimated imported cost meets the
	// month (ADR-0068 §3).
	Estimated bool
	FundingProgress
}

// MeasureFunding measures the month's net income against target, or against the month's costs
// when target is nil, which needs them in one currency and above zero. It returns false when the
// month has no such target.
func MeasureFunding(period Period, net []Money, costs PeriodTotal, target *Money) (FundingMeter, bool) {
	out := FundingMeter{Period: period}
	switch {
	case target != nil:
		out.Target = *target
	case len(costs.Totals) == 1:
		out.Target, out.TargetIsCosts, out.Estimated = costs.Totals[0], true, costs.Estimated
	default:
		return FundingMeter{}, false
	}
	if out.Target.Minor <= 0 {
		return FundingMeter{}, false
	}
	out.FundingProgress = Progress(net, out.Target)
	return out, true
}
