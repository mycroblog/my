// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_billing_cloudflare

package platform

import (
	"context"
	"net/http"
	"time"

	billingcloudflare "github.com/mycroblog/my/internal/adapter/billing/cloudflare"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { billingSourceAdapters[billingcloudflare.Name] = newCloudflareBillingSource }

// cloudflareBillingTimeout bounds one call of Cloudflare's billable usage API.
const cloudflareBillingTimeout = 30 * time.Second

// newCloudflareBillingSource returns the BillingSource of billing_sources = cloudflare (ADR-0068
// §11). It reads with billing_cloudflare_api_token, never with the CLOUDFLARE_* variables of the
// environment. Cloudflare's API is a destination the operator chose, so the client is not the
// SSRF-guarded kernel.HTTPClient; it follows no redirect, so that the token reaches Cloudflare only.
func newCloudflareBillingSource(_ context.Context, cfg *config.Config, _ kernel.Meter) (finance.BillingSource, error) {
	return billingcloudflare.New(billingcloudflare.Options{
		AccountID: cfg.BillingCloudflareAccountID,
		Token:     cfg.BillingCloudflareAPIToken,
		Client: &http.Client{
			Timeout:       cloudflareBillingTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	})
}
