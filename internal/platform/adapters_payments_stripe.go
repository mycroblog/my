// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_payments_stripe

package platform

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	paymentstripe "github.com/mycroblog/my/internal/adapter/payment/stripe"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { paymentProviderAdapters[paymentstripe.Name] = newStripePaymentProvider }

// newStripePaymentProvider returns the PaymentProvider of payments = stripe (ADR-0074
// §2, §7). The SDK reaches only api.stripe.com, the endpoint of the key that the operator
// configured, so its HTTP client is not the SSRF-guarded kernel.HTTPClient; a request has 30
// seconds in all.
func newStripePaymentProvider(ctx context.Context, cfg *config.Config, logger *slog.Logger) (supporters.PaymentProvider, error) {
	return paymentstripe.New(ctx, paymentstripe.Options{
		SecretKey: cfg.PaymentsStripeSecretKey, WebhookSecret: cfg.PaymentsStripeWebhookSecret,
		HTTPClient: &http.Client{Timeout: 30 * time.Second}, Logger: logger,
	})
}
