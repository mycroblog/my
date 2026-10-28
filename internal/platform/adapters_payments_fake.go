// SPDX-License-Identifier: AGPL-3.0-only

//go:build e2e_fakes

package platform

import (
	"context"
	"errors"
	"log/slog"

	paymentfake "github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/platform/config"
)

// This file is the only one outside the tests that imports the fake payment provider, and only
// the test builds with the build tag e2e_fakes have it (ADR-0117 §2). scripts/go-build-tags.sh
// fails when cmd/my without the tag depends on the fake, the Dockerfile refuses the tag, and
// scripts/check-release-build.sh refuses a binary built with it.

func init() { paymentProviderAdapters["fake"] = newFakePaymentProvider }

// newFakePaymentProvider returns the PaymentProvider of payments_provider = fake: the fake with
// its own checkout page, whose state lives in this process only. Like the configuration's
// check, it refuses any profile but development, so that the fake never records payments as
// paid on a production instance.
func newFakePaymentProvider(ctx context.Context, cfg *config.Config, logger *slog.Logger) (supporters.PaymentProvider, error) {
	if cfg.Profile != "development" {
		return nil, errors.New("fake needs profile = development: it records payments as paid without any money moving")
	}
	logger.WarnContext(ctx, "this is a test build (build tag e2e_fakes): payments_provider = fake records payments as paid without any money moving")
	return paymentfake.NewPages(paymentfake.Options{}), nil
}
