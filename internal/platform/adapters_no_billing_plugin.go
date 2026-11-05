// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_plugins

package platform

import (
	"context"

	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { billingSourceAdapters[finance.SourcePlugin] = newPluginBillingSource }

// newPluginBillingSource reports that the build tag no_plugins left the plugins out.
func newPluginBillingSource(context.Context, *config.Config, kernel.Meter) (finance.BillingSource, error) {
	return nil, config.ExcludedError("billing_sources", finance.SourcePlugin, "no_plugins")
}
