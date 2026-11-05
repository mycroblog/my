// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_billing_gcp

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { billingSourceAdapters["gcp"] = newGCPBillingSource }

// newGCPBillingSource reports that the build tag no_billing_gcp left the Google Cloud billing
// adapter out.
func newGCPBillingSource(context.Context, *config.Config, kernel.Meter) (finance.BillingSource, error) {
	return nil, config.ExcludedError("billing_sources", "gcp", "no_billing_gcp")
}
