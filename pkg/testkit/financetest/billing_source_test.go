// SPDX-License-Identifier: AGPL-3.0-only

package financetest_test

import (
	"fmt"
	"testing"

	appfinance "github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/pkg/testkit/financetest"
)

// fakeFailures are the provider errors of the fake, wrapped like an adapter wraps its provider's.
var fakeFailures = map[appfinance.ImportErrorKind]error{
	appfinance.ImportErrorPermission:      appfinance.ErrBillingPermission,
	appfinance.ImportErrorNotEnabled:      appfinance.ErrBillingNotEnabled,
	appfinance.ImportErrorNotFound:        appfinance.ErrBillingNotFound,
	appfinance.ImportErrorThrottled:       appfinance.ErrBillingThrottled,
	appfinance.ImportErrorBytesCap:        appfinance.ErrBytesCap,
	appfinance.ImportErrorInvalidResponse: appfinance.ErrInvalidBillingResponse,
	appfinance.ImportErrorUnavailable:     appfinance.ErrBillingUnavailable,
}

func TestFakeBillingSourceContract(t *testing.T) {
	financetest.BillingSourceContract(t, func(_ *testing.T, s financetest.BillingScenario) appfinance.BillingSource {
		src := financetest.NewFakeBillingSource("aws", s.Location)
		for _, m := range s.Months {
			src.SetMonth(m.Month, financetest.FakeBillingMonth{Final: m.Final, Through: m.Through, Rows: m.Rows})
		}
		src.SetPages(s.Pages)
		if s.Failure != "" {
			src.SetFailure(fmt.Errorf("fake provider: %w", fakeFailures[s.Failure]))
		}
		return src
	})
}

// TestFakeBillingUsageSourceContract runs the contract of a source of usage alone over the fake,
// which then never sees a row of another charge type.
func TestFakeBillingUsageSourceContract(t *testing.T) {
	financetest.BillingUsageSourceContract(t, func(t *testing.T, s financetest.BillingScenario) appfinance.BillingSource {
		src := financetest.NewFakeBillingSource("cloudflare", s.Location)
		for _, m := range s.Months {
			for _, r := range m.Rows {
				if r.ChargeType != finance.ChargeUsage {
					t.Errorf("the scenario of a source of usage alone has a row of %s", r.ChargeType)
				}
			}
			src.SetMonth(m.Month, financetest.FakeBillingMonth{Final: m.Final, Through: m.Through, Rows: m.Rows})
		}
		src.SetPages(s.Pages)
		if s.Failure != "" {
			src.SetFailure(fmt.Errorf("fake provider: %w", fakeFailures[s.Failure]))
		}
		return src
	})
}
