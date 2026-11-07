// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import "testing"

func TestPluginSearch(t *testing.T) {
	const address = "unix:///run/my/search.sock"
	cfg, issues := loadPlugin(t, map[string]string{"MY_SEARCH": "plugin", "MY_PLUGIN_SEARCH_ADDRESS": address})
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if cfg.Search != "plugin" || cfg.PluginDestination("search").Address != address {
		t.Errorf("config = %q, %+v", cfg.Search, cfg.PluginDestination("search"))
	}

	// The address is required once the plugin is selected.
	if _, issues := loadPlugin(t, map[string]string{"MY_SEARCH": "plugin"}); !hasIssue(issues, "plugin_search_address") {
		t.Errorf("without the address: issues = %+v", issues)
	}
	// An address alone selects nothing.
	if cfg, issues := loadPlugin(t, map[string]string{"MY_PLUGIN_SEARCH_ADDRESS": address}); len(issues) != 0 || cfg.Search != "none" {
		t.Errorf("an address without search = plugin: search = %q, issues = %+v", cfg.Search, issues)
	}

	// Plain TCP stops the start, on loopback too: the plugin receives the text of posts and the queries of people.
	for _, plain := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://search.internal:8080"} {
		if _, issues := loadPlugin(t, map[string]string{"MY_SEARCH": "plugin", "MY_PLUGIN_SEARCH_ADDRESS": plain}); !hasIssue(issues, "plugin_search_address") {
			t.Errorf("%s: issues = %+v", plain, issues)
		}
	}

	// A request of the search API waits for the plugin, so its deadline stays below the bound of a request.
	if _, issues := loadPlugin(t, map[string]string{"MY_SEARCH": "plugin", "MY_PLUGIN_SEARCH_ADDRESS": address, "MY_PLUGIN_SEARCH_TIMEOUT": "30s"}); !hasIssue(issues, "plugin_search_timeout") {
		t.Errorf("a timeout of 30s: issues = %+v", issues)
	}
}
