// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import (
	"strings"
	"testing"
)

func TestPluginTranslator(t *testing.T) {
	const address, name = "MY_PLUGIN_TRANSLATOR_ADDRESS", "MY_PLUGIN_TRANSLATOR_NAME"
	const socket = "unix:///run/my/translator.sock"
	cfg, issues := loadPlugin(t, map[string]string{address: socket, name: "  Our translator "})
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if cfg.PluginDestination("translator").Address != socket || cfg.PluginTranslatorName != "Our translator" {
		t.Errorf("config = %+v, %q", cfg.PluginDestination("translator"), cfg.PluginTranslatorName)
	}

	// Readers see the name, so an address needs one, and a name alone does nothing.
	if _, issues := loadPlugin(t, map[string]string{address: socket}); !hasIssue(issues, "plugin_translator_name") {
		t.Errorf("without the name: issues = %+v", issues)
	}
	if cfg, issues := loadPlugin(t, map[string]string{name: "Our translator"}); len(issues) != 0 || cfg.PluginDestination("translator").Enabled() {
		t.Errorf("a name without an address: %+v, issues = %+v", cfg.PluginDestination("translator"), issues)
	}
	for _, wrong := range []string{"   ", strings.Repeat("x", 101), "two\nlines", "a\ttab"} {
		if _, issues := loadPlugin(t, map[string]string{address: socket, name: wrong}); !hasIssue(issues, "plugin_translator_name") {
			t.Errorf("name %q: issues = %+v", wrong, issues)
		}
	}
	if _, issues := loadPlugin(t, map[string]string{address: socket, name: strings.Repeat("訳", 100)}); len(issues) != 0 {
		t.Errorf("100 characters: issues = %+v", issues)
	}

	// Plain TCP stops the start, on loopback too, whatever else is right.
	for _, plain := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://translate.internal:5000"} {
		if _, issues := loadPlugin(t, map[string]string{address: plain, name: "Our translator"}); !hasIssue(issues, "plugin_translator_address") {
			t.Errorf("%s: issues = %+v", plain, issues)
		}
	}
}
