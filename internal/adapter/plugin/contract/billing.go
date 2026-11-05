// SPDX-License-Identifier: AGPL-3.0-only

package contract

import (
	"context"
	"errors"
	"fmt"
	"time"

	billingplugin "github.com/mycroblog/my/internal/adapter/billing/plugin"
	billingv1 "github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/billing/v1"
	"github.com/mycroblog/my/internal/adapter/plugin/gen/my/plugin/billing/v1/billingv1connect"
	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
)

// billedMaxRequests is what the billed case of the billing kind allows the plugin.
const billedMaxRequests = 10

// billingCases check a billing plugin. Only the case that forbids any billed request runs by default, since the
// providers behind a real plugin may charge for the others (ADR-0146 §16).
var billingCases = []Case{
	{Name: "fetch_without_requests", Run: billingWithoutRequests},
	{
		Name: "fetch_billed",
		Skip: func(env Env) string {
			if !env.Billed {
				return "the providers may charge for it; --billed runs it"
			}
			return ""
		},
		Run: billingBilled,
	},
}

func thisAndLastMonth(now time.Time) (last, this finance.YearMonth) {
	this = finance.YearMonthOf(finance.DateOf(now.UTC()))
	return this.AddMonths(-1), this
}

// billingWithoutRequests sends max_requests = 0 itself: the adapter would not make the call.
func billingWithoutRequests(ctx context.Context, env Env) error {
	last, this := thisAndLastMonth(time.Now())
	res, err := billingv1connect.NewBillingServiceClient(env.Conn.RPC).Fetch(ctx, &billingv1.FetchRequest{
		Months: []string{last.String(), this.String()}, MaxRequests: 0,
	})
	if err != nil {
		return describeCallError(env, err)
	}
	switch {
	case res.GetOutcome() != billingv1.Outcome_OUTCOME_REQUEST_CAP:
		return fmt.Errorf("max_requests 0: the outcome is %s, not OUTCOME_REQUEST_CAP", res.GetOutcome())
	case res.GetRequests() != 0:
		return fmt.Errorf("max_requests 0: %d billed requests, not 0", res.GetRequests())
	case len(res.GetLines()) != 0:
		return fmt.Errorf("max_requests 0: %d lines, not none", len(res.GetLines()))
	}
	return nil
}

// billingBilled makes the server's call through the adapter, which applies the checks of the imports to the answer. The
// plugin's time zone is unknown here, so the months are UTC's; they only have to be months.
func billingBilled(ctx context.Context, env Env) error {
	source, err := billingplugin.New(env.Conn, time.UTC)
	if err != nil {
		return err
	}
	last, this := thisAndLastMonth(time.Now())
	_, err = source.Fetch(ctx, appfinance.FetchRequest{Months: []finance.YearMonth{last, this}, MaxRequests: billedMaxRequests})
	if errors.Is(err, appfinance.ErrBillingUnavailable) {
		return describeCallError(env, err)
	}
	return err
}
