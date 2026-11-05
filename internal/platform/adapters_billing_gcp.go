// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_billing_gcp

package platform

import (
	"context"
	"fmt"

	billinggcp "github.com/mycroblog/my/internal/adapter/billing/gcp"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { billingSourceAdapters["gcp"] = newGCPBillingSource }

// newGCPBillingSource returns the BillingSource of billing_sources = gcp by the method of
// billing_gcp_method (ADR-0068 §7, §8). The credentials are the Application Default Credentials
// (the service account of Cloud Run, GOOGLE_APPLICATION_CREDENTIALS elsewhere). The clients reach
// only the BigQuery and Pub/Sub APIs, so they do not go through the SSRF-guarded kernel.HTTPClient.
func newGCPBillingSource(ctx context.Context, cfg *config.Config, _ kernel.Meter) (finance.BillingSource, error) {
	switch cfg.BillingGCPMethod {
	case "", billinggcp.MethodBigQuery:
		return billinggcp.NewBigQuery(ctx, billinggcp.BigQueryOptions{
			Table: cfg.BillingGCPTable, QueryProject: cfg.BillingGCPQueryProject,
			Projects: cfg.BillingGCPProjects, Label: cfg.BillingGCPLabel, MaxBytesBilled: cfg.BillingGCPMaxBytesBilled,
		})
	case billinggcp.MethodBudget:
		return billinggcp.NewBudget(ctx, billinggcp.BudgetOptions{Subscription: cfg.BillingGCPBudgetSubscription})
	default:
		return nil, fmt.Errorf("billing_gcp_method = %s is not implemented", cfg.BillingGCPMethod)
	}
}
