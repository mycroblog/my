// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"io"

	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// billingSourceAdapters builds the BillingSource of each name of billing_sources whose adapter is
// in this build (ADR-0068 §10): the files of the adapters, under their build tags, add theirs.
var billingSourceAdapters = map[string]func(ctx context.Context, cfg *config.Config) (finance.BillingSource, error){}

// setUpFinance builds the finance module (ADR-0067, ADR-0068): its projectors, the service, the
// sources of billing_sources with the AWS request caps, and the jobs of the billing imports.
func (a *App) setUpFinance(ctx context.Context, cfg *config.Config, repo finance.Repository, opts AppOptions) error {
	var sources []finance.BillingSource
	for _, name := range cfg.BillingSources {
		if src, ok := opts.BillingSources[name]; ok {
			sources = append(sources, src)
			continue
		}
		build, ok := billingSourceAdapters[name]
		if !ok {
			return fmt.Errorf("billing_sources = %s: this build has no adapter of %s", name, name)
		}
		src, err := build(ctx, cfg)
		if err != nil {
			return fmt.Errorf("billing_sources = %s: %w", name, err)
		}
		if c, ok := src.(io.Closer); ok {
			a.billingClosers = append(a.billingClosers, c)
		}
		sources = append(sources, src)
	}
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue; its jobs are enqueued after the commit.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	finance.RegisterProjectors(a.Projectors, repo)
	a.Finance = finance.NewService(finance.Deps{
		UnitOfWork: a.UnitOfWork, Repository: repo, Clock: a.Clock, IDs: a.IDs,
		Sources: sources,
		Caps: map[string]finance.RequestCaps{
			"aws": {PerDay: cfg.BillingAWSMaxRequestsPerDay, PerMonth: cfg.BillingAWSMaxRequestsPerMonth},
		},
		Supporters: supporterDirectory{a},
		Outbox:     outbox, Logger: a.Logger, Meter: a.Meter,
	})
	a.Finance.RegisterJobs(a.Handlers, a.Scheduler)
	return nil
}
