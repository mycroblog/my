// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_billing_cloudflare

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { billingSourceAdapters["cloudflare"] = newCloudflareBillingSource }

// newCloudflareBillingSource reports that the build tag no_billing_cloudflare left the Cloudflare
// billing adapter out.
func newCloudflareBillingSource(context.Context, *config.Config, kernel.Meter) (finance.BillingSource, error) {
	return nil, config.ExcludedError("billing_sources", "cloudflare", "no_billing_cloudflare")
}
