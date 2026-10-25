// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/interface/clientapi"
	paymentshttp "github.com/mycroblog/my/internal/interface/payments"
	"github.com/mycroblog/my/internal/platform/config"
)

// paymentProviderAdapters builds the PaymentProvider of each value of payments_provider whose
// adapter is in this build (ADR-0074 §8): the files of the adapters, under their build tags,
// add theirs.
var paymentProviderAdapters = map[string]func(ctx context.Context, cfg *config.Config, logger *slog.Logger) (supporters.PaymentProvider, error){}

// newPaymentProvider returns the PaymentProvider of payments_provider, AppOptions.PaymentProvider
// standing in for it, and nil with payments_provider = none.
func newPaymentProvider(ctx context.Context, cfg *config.Config, opts AppOptions, logger *slog.Logger) (supporters.PaymentProvider, error) {
	if opts.PaymentProvider != nil {
		return opts.PaymentProvider, nil
	}
	if cfg.PaymentsProvider == "" || cfg.PaymentsProvider == "none" {
		return nil, nil
	}
	build, ok := paymentProviderAdapters[cfg.PaymentsProvider]
	if !ok {
		return nil, fmt.Errorf("payments_provider = %s: this build has no adapter of %s", cfg.PaymentsProvider, cfg.PaymentsProvider)
	}
	prov, err := build(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("payments_provider = %s: %w", cfg.PaymentsProvider, err)
	}
	return prov, nil
}

// paymentsAPI returns the Payments of the admin API: nil without a payment provider, so that its
// operations answer 404 rather than call a nil *supporters.Payments.
func (a *App) paymentsAPI() clientapi.Payments {
	if a.Payments == nil {
		return nil
	}
	return a.Payments
}

// setUpPayments builds what carries the payment provider's facts to the supporters module
// (ADR-0074 §3, §4) when there is a provider (newPaymentProvider): the webhook endpoint, the
// jobs of the deliveries, the reconciliation every payments_reconcile_interval, and the purge of
// the receipts, which hand the facts to sync (the supporters core, or AppOptions.SupporterSync
// in its place). Without a provider nothing runs and the endpoint answers 404 like any unknown
// path.
func (a *App) setUpPayments(cfg *config.Config, repo supporters.PaymentRepository, prov supporters.PaymentProvider, sync supporters.Syncer) error {
	if prov == nil {
		return nil
	}
	if sync == nil {
		return fmt.Errorf("payments_provider = %s: no subscriptions of the supporters module apply the payments", prov.Name())
	}
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Payments = supporters.NewPayments(supporters.PaymentsDeps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Provider: prov, Syncer: sync, Outbox: outbox,
		Clock: a.Clock, IDs: a.IDs, RateLimiter: a.RateLimiter,
		ReconcileInterval: cfg.PaymentsReconcileInterval, ReconcileMaxRequests: cfg.PaymentsReconcileMaxRequests,
		PageEnv: supporters.PageEnv{Locales: a.Locales, Stylesheet: func() string { return a.webStylesheet }, Logger: a.Logger},
		Logger:  a.Logger,
	})
	a.Payments.RegisterJobs(a.Handlers, a.Scheduler)
	paymentshttp.New(paymentshttp.Options{Webhooks: a.Payments, Pages: a.Payments, Problem: a.writeStatusProblem, Logger: a.Logger}).
		Register(surfaceRouter{a, surfacePayments})
	return nil
}
