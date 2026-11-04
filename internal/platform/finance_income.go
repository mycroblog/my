// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/supporters"
)

// The two ports between the supporters and the finance modules (ADR-0067 §11): the supporters
// hand their money movements to the income records, and the finance module reads the plans,
// the subscribers, and the provider's time zone of the supporters. Both read the App when they
// are called, since the supporters module is set up before the finance module.

// financeIncome is the supporters.IncomeRecorder of the finance module's income records.
type financeIncome struct{ a *App }

var _ supporters.IncomeRecorder = financeIncome{}

// errNoFinance reports a money movement before the finance module is set up.
var errNoFinance = errors.New("platform: the finance module is not set up")

func (f financeIncome) RecordIncome(ctx context.Context, ms []finance.IncomeMovement) error {
	if f.a.Finance == nil {
		return errNoFinance
	}
	return f.a.Finance.RecordIncome(ctx, ms)
}

func (f financeIncome) KnownIncome(ctx context.Context, provider string, ids []string) (map[string]bool, error) {
	if f.a.Finance == nil {
		return nil, errNoFinance
	}
	return f.a.Finance.KnownIncome(ctx, provider, ids)
}

// supporterDirectory is the finance.SupporterDirectory of the supporters module; without it,
// it reads nothing.
type supporterDirectory struct{ a *App }

var _ finance.SupporterDirectory = supporterDirectory{}

func (d supporterDirectory) PlanCounts(ctx context.Context) ([]finance.PlanCount, error) {
	if d.a.Supporters == nil {
		return nil, nil
	}
	counts, err := d.a.Supporters.PlanCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]finance.PlanCount, len(counts))
	for i, c := range counts {
		out[i] = finance.PlanCount{PlanID: c.PlanID, Name: c.Name, Members: c.Members}
	}
	return out, nil
}

func (d supporterDirectory) SubscriptionAccounts(ctx context.Context, ids []string) (map[string]finance.Subscriber, error) {
	out := map[string]finance.Subscriber{}
	if d.a.Supporters == nil {
		return out, nil
	}
	users, err := d.a.Supporters.SubscriptionUsers(ctx, ids)
	if err != nil || len(users) == 0 {
		return out, err
	}
	accounts := map[string]finance.Subscriber{}
	for _, u := range users {
		acct, read := accounts[u.UserID]
		if !read {
			la, err := d.a.Accounts.LocalAccountOfUser(ctx, u.UserID)
			switch {
			case errors.Is(err, account.ErrNotFound):
			case err != nil:
				return nil, err
			default:
				acct = finance.Subscriber{AccountID: la.AccountID, Username: la.Username}
			}
			accounts[u.UserID] = acct
		}
		if acct.AccountID != "" {
			out[u.SubscriptionID] = acct
		}
	}
	return out, nil
}

func (d supporterDirectory) ProviderRules() []finance.ProviderRule {
	if d.a.Supporters == nil {
		return nil
	}
	name, loc, ok := d.a.Supporters.ProviderLocation()
	if !ok {
		return nil
	}
	return []finance.ProviderRule{{Provider: name, Location: loc}}
}
