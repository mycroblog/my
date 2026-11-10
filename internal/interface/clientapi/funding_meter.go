// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"context"
	"fmt"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/instance"
)

// FundingMeters measures this month's net income against the monthly target (ADR-0067 §22).
// *appfinance.Service implements it.
type FundingMeters interface {
	FundingMeter(ctx context.Context, target appfinance.FundingTarget) (finance.FundingMeter, bool, error)
}

// instanceFundingMeter presents the funding meter that settings publish on GET /instance, or nil
// while it is off, the finance module is not configured, or the month has no target. Under
// percent it carries the percentage alone: the costs and the income are both private then, and
// the income or the target would give either away (ADR-0067 §22).
func (s *Server) instanceFundingMeter(ctx context.Context, settings instance.Settings) (*FundingMeter, error) {
	visibility := settings.FundingMeterVisibility()
	if visibility == instance.FundingMeterOff || s.o.FundingMeters == nil {
		return nil, nil
	}
	target := settings.FundingMeterTarget()
	meter, ok, err := s.o.FundingMeters.FundingMeter(ctx, appfinance.FundingTarget{Amount: target.Amount, Currency: target.Currency})
	if err != nil {
		return nil, fmt.Errorf("clientapi: the funding meter: %w", err)
	}
	if !ok {
		return nil, nil
	}
	out := &FundingMeter{
		Visibility: FundingMeterVisibility(visibility), Label: meter.Period.Label,
		StartsOn: meter.Period.Range.Start.String(), EndsOn: meter.Period.Range.End.String(),
		Percent: meter.Percent, Covered: meter.Covered, Estimated: meter.Estimated, Rail: settings.FundingMeterRail(),
	}
	if visibility == instance.FundingMeterAmounts {
		income, target := newMoney(meter.Net), newMoney(meter.Target)
		out.Income, out.Target, out.TargetIsCosts = &income, &target, &meter.TargetIsCosts
	}
	return out, nil
}
