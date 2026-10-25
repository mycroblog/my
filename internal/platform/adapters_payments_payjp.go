// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_payments_payjp

package platform

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	paymentpayjp "github.com/mycroblog/my/internal/adapter/payment/payjp"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { paymentProviderAdapters[paymentpayjp.Name] = newPayjpPaymentProvider }

// newPayjpPaymentProvider returns the PaymentProvider of payments_provider = payjp (ADR-0099 §2,
// §7). The adapter reaches only api.pay.jp, the endpoint of the key that the operator
// configured, so its HTTP client is not the SSRF-guarded kernel.HTTPClient; a request has 30
// seconds in all.
func newPayjpPaymentProvider(_ context.Context, cfg *config.Config, logger *slog.Logger) (supporters.PaymentProvider, error) {
	return paymentpayjp.New(paymentpayjp.Options{
		SecretKey: cfg.PaymentsPayjpSecretKey, PublicKey: cfg.PaymentsPayjpPublicKey, WebhookToken: cfg.PaymentsPayjpWebhookToken,
		HTTPClient: &http.Client{Timeout: 30 * time.Second}, Logger: logger,
	})
}
