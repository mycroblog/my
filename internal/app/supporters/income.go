// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"log/slog"

	appfinance "github.com/mycroblog/my/internal/app/finance"
)

// The hand-over of the provider's money movements to the finance module's income records
// (ADR-0067 §11): the only call from this module into finance, through IncomeRecorder.

// IncomeRecorder takes the provider's final money movements to the finance module's income
// records (finance.RecordIncome, ADR-0067 §11): *appfinance.Service implements it.
type IncomeRecorder interface {
	RecordIncome(ctx context.Context, ms []appfinance.IncomeMovement) error
	KnownIncome(ctx context.Context, provider string, ids []string) (map[string]bool, error)
}

// IncomeOf returns m of provider in the form of the income records, with subscriptionID, our
// ID of its subscription ("" for none).
func IncomeOf(provider string, m MoneyMovement, subscriptionID string) appfinance.IncomeMovement {
	out := appfinance.IncomeMovement{
		Provider: provider, ProviderID: m.ID, Kind: string(m.Kind), ParentID: m.ParentID, SubscriptionID: subscriptionID,
		OccurredAt: m.OccurredAt, OccurredOn: m.OccurredOn.String(), Gross: m.Gross, Fee: m.Fee, Net: m.Net, Currency: m.Currency,
	}
	if !m.Covers.IsZero() {
		out.CoversStart, out.CoversEnd = m.Covers.Start.String(), m.Covers.End.String()
	}
	return out
}

// recordIncome hands the final movements of provider to the income records, with our ID of
// each one's subscription: "" for a fee of no subscription and for a subscription that is not
// ours, whose movement still counts as income of the provider's account.
func (s *Service) recordIncome(ctx context.Context, provider string, ms []MoneyMovement) error {
	ids := map[SubscriptionRef]string{}
	out := make([]appfinance.IncomeMovement, 0, len(ms))
	for _, m := range ms {
		if !m.Final {
			continue
		}
		id, ok := ids[m.Subscription]
		if !ok && m.Subscription != "" {
			var err error
			id, err = s.d.Repository.FindSubscriptionIDByRef(ctx, nil, provider, m.Subscription)
			switch {
			case errors.Is(err, ErrNotFound):
				s.d.Logger.WarnContext(ctx, "a money movement of a subscription that is not ours", slog.String("movement", m.ID))
				id = ""
			case err != nil:
				return err
			}
			ids[m.Subscription] = id
		}
		out = append(out, IncomeOf(provider, m, id))
	}
	if len(out) == 0 {
		return nil
	}
	return s.d.Income.RecordIncome(ctx, out)
}
