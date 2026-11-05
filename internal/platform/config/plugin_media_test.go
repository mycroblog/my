// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import (
	"strings"
	"testing"
)

func TestPluginMedia(t *testing.T) {
	const address = "unix:///run/my/media.sock"
	cfg, issues := loadPlugin(t, map[string]string{"MY_MEDIA": "plugin", "MY_PLUGIN_MEDIA_ADDRESS": address})
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if d := cfg.PluginDestination("media"); cfg.Media != "plugin" || d.Address != address || d.Protocol != "" {
		t.Errorf("config = %q, %+v", cfg.Media, d)
	}

	// The address is required once the plugin is selected, and an address alone selects nothing.
	if _, issues := loadPlugin(t, map[string]string{"MY_MEDIA": "plugin"}); !hasIssue(issues, "plugin_media_address") {
		t.Errorf("without the address: issues = %+v", issues)
	}
	if cfg, issues := loadPlugin(t, map[string]string{"MY_PLUGIN_MEDIA_ADDRESS": address}); len(issues) != 0 || cfg.Media != "native" {
		t.Errorf("an address without media = plugin: media = %q, issues = %+v", cfg.Media, issues)
	}

	// The calls stream both ways, which needs HTTP/2: json stops the start, proto and grpc do not.
	_, issues = loadPlugin(t, map[string]string{"MY_MEDIA": "plugin", "MY_PLUGIN_MEDIA_ADDRESS": address, "MY_PLUGIN_MEDIA_PROTOCOL": "json"})
	if !hasIssue(issues, "plugin_media_protocol") {
		t.Errorf("json: issues = %+v", issues)
	}
	for _, protocol := range []string{"proto", "grpc"} {
		if _, issues := loadPlugin(t, map[string]string{"MY_MEDIA": "plugin", "MY_PLUGIN_MEDIA_ADDRESS": address, "MY_PLUGIN_MEDIA_PROTOCOL": protocol}); len(issues) != 0 {
			t.Errorf("%s: issues = %+v", protocol, issues)
		}
	}

	// Plain TCP works, with a warning that names the kind and the address.
	const plain = "http://media.internal:9000"
	cfg, issues = loadPlugin(t, map[string]string{"MY_MEDIA": "plugin", "MY_PLUGIN_MEDIA_ADDRESS": plain})
	if len(issues) != 0 {
		t.Fatalf("plain TCP: issues = %+v", issues)
	}
	if w := PluginWarnings(cfg); len(w) != 1 || w[0].Key != "plugin_media_address" || !strings.Contains(w[0].Message, "media") || !strings.Contains(w[0].Message, plain) {
		t.Errorf("plain TCP: warnings = %+v", w)
	}
}
