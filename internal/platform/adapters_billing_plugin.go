// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package platform

import (
	"context"
	"fmt"
	"time"

	billingplugin "github.com/mycroblog/my/internal/adapter/billing/plugin"
	"github.com/mycroblog/my/internal/adapter/plugin/kind"
	"github.com/mycroblog/my/internal/app/finance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { billingSourceAdapters[finance.SourcePlugin] = newPluginBillingSource }

// newPluginBillingSource returns the BillingSource of billing_sources = plugin (ADR-0146 §11). It opens no connection: a
// plugin that is down at start fails its imports, and the server still starts (ADR-0146 §15).
func newPluginBillingSource(_ context.Context, cfg *config.Config, meter kernel.Meter) (finance.BillingSource, error) {
	location, err := time.LoadLocation(cfg.PluginBillingLocation)
	if err != nil {
		return nil, fmt.Errorf("plugin_billing_location: %w", err)
	}
	conn, err := NewPluginConn(cfg, kind.Billing, meter)
	if err != nil {
		return nil, err
	}
	return billingplugin.New(conn, location)
}
