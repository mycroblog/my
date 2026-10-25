// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_payments_payjp

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { paymentProviderAdapters["payjp"] = newPayjpPaymentProvider }

// newPayjpPaymentProvider reports that the build tag no_payments_payjp left the PAY.JP adapter
// out.
func newPayjpPaymentProvider(context.Context, *config.Config, *slog.Logger) (supporters.PaymentProvider, error) {
	return nil, config.ExcludedError("payments_provider", "payjp", "no_payments_payjp")
}
