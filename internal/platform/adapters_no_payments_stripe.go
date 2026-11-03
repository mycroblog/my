// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_payments_stripe

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { paymentProviderAdapters["stripe"] = newStripePaymentProvider }

// newStripePaymentProvider reports that the build tag no_payments_stripe left the Stripe adapter
// out.
func newStripePaymentProvider(context.Context, *config.Config, *slog.Logger) (supporters.PaymentProvider, error) {
	return nil, config.ExcludedError("payments", "stripe", "no_payments_stripe")
}
