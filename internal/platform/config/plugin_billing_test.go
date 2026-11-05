// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestPluginBillingSource(t *testing.T) {
	address, location, provider := "MY_PLUGIN_BILLING_ADDRESS", "MY_PLUGIN_BILLING_LOCATION", "MY_PLUGIN_BILLING_PROVIDER"
	full := map[string]string{"MY_BILLING_SOURCES": "plugin", address: "http://billing.internal:8080", location: "Asia/Tokyo", provider: "  Hetzner "}
	cfg, issues := loadPlugin(t, full)
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if !slices.Equal(cfg.BillingSources, []string{"plugin"}) || cfg.PluginBillingLocation != "Asia/Tokyo" || cfg.PluginBillingProvider != "Hetzner" {
		t.Errorf("config = %v, %q, %q", cfg.BillingSources, cfg.PluginBillingLocation, cfg.PluginBillingProvider)
	}

	// Each setting is required once the source is selected, and only then.
	for env, key := range map[string]string{address: "plugin_billing_address", location: "plugin_billing_location", provider: "plugin_billing_provider"} {
		partial := map[string]string{}
		for k, v := range full {
			if k != env {
				partial[k] = v
			}
		}
		if _, issues := loadPlugin(t, partial); !hasIssue(issues, key) {
			t.Errorf("without %s: issues = %+v", env, issues)
		}
	}
	if _, issues := loadPlugin(t, map[string]string{location: "UTC", provider: "Hetzner"}); len(issues) != 0 {
		t.Errorf("without the source: issues = %+v", issues)
	}

	// A wrong value of each one stops the start.
	for env, tc := range map[string]struct{ value, key string }{
		location:             {"Local", "plugin_billing_location"},
		provider:             {strings.Repeat("x", 101), "plugin_billing_provider"},
		"MY_BILLING_SOURCES": {"plugin,plugin", "billing_sources"},
	} {
		wrong := maps.Clone(full)
		wrong[env] = tc.value
		if _, issues := loadPlugin(t, wrong); !hasIssue(issues, tc.key) {
			t.Errorf("%s = %s: issues = %+v", env, tc.value, issues)
		}
	}
	for _, zone := range []string{"Nowhere/Land", "../../etc/passwd"} {
		wrong := maps.Clone(full)
		wrong[location] = zone
		if _, issues := loadPlugin(t, wrong); !hasIssue(issues, "plugin_billing_location") {
			t.Errorf("location %s: issues = %+v", zone, issues)
		}
	}
}
