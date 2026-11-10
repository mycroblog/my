// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"context"
	"errors"
	"log/slog"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
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
// ID of its subscription ("" for none): the source subscription with one, unknown without.
func IncomeOf(provider string, m MoneyMovement, subscriptionID string) appfinance.IncomeMovement {
	return incomeOf(provider, m, subscriptionID, "")
}

// incomeOf is IncomeOf with tipID, our ID of its tip ("" for none), which makes the source
// one_time (ADR-0162 §16). The finance module gives a refund, a chargeback, or a reversal the
// source of its payment.
func incomeOf(provider string, m MoneyMovement, subscriptionID, tipID string) appfinance.IncomeMovement {
	source := string(finance.SourceUnknown)
	switch {
	case subscriptionID != "":
		source = string(finance.SourceSubscription)
	case tipID != "":
		source = string(finance.SourceOneTime)
	}
	out := appfinance.IncomeMovement{
		Provider: provider, ProviderID: m.ID, Kind: string(m.Kind), ParentID: m.ParentID, SubscriptionID: subscriptionID, TipID: tipID, Source: source,
		OccurredAt: m.OccurredAt, OccurredOn: m.OccurredOn.String(), Gross: m.Gross, Fee: m.Fee, Net: m.Net, Currency: m.Currency,
	}
	if !m.Covers.IsZero() {
		out.CoversStart, out.CoversEnd = m.Covers.Start.String(), m.Covers.End.String()
	}
	return out
}

// recordIncome hands the final movements of provider to the income records, with our ID of
// each one's subscription or tip (tips, by payment): "" for a fee of no subscription and for a
// subscription that is not ours, whose movement still counts as income of the provider's account.
func (s *Service) recordIncome(ctx context.Context, provider string, ms []MoneyMovement, tips map[PaymentRef]string) error {
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
		tip := ""
		if m.Subscription == "" {
			tip = tips[m.Payment]
		}
		out = append(out, incomeOf(provider, m, id, tip))
	}
	if len(out) == 0 {
		return nil
	}
	return s.d.Income.RecordIncome(ctx, out)
}
