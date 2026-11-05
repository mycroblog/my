// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import "testing"

func TestPluginMailer(t *testing.T) {
	const from, address = "noreply@social.example", "unix:///run/my/mailer.sock"
	cfg, issues := loadPlugin(t, map[string]string{"MY_MAIL": "plugin", "MY_MAIL_FROM": from, "MY_PLUGIN_MAILER_ADDRESS": address})
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if cfg.Mail != "plugin" || cfg.MailFrom != from || cfg.PluginDestination("mailer").Address != address {
		t.Errorf("config = %q, %q, %+v", cfg.Mail, cfg.MailFrom, cfg.PluginDestination("mailer"))
	}

	// The address and the sender are required once the plugin is selected.
	if _, issues := loadPlugin(t, map[string]string{"MY_MAIL": "plugin", "MY_MAIL_FROM": from}); !hasIssue(issues, "plugin_mailer_address") {
		t.Errorf("without the address: issues = %+v", issues)
	}
	if _, issues := loadPlugin(t, map[string]string{"MY_MAIL": "plugin", "MY_PLUGIN_MAILER_ADDRESS": address}); !hasIssue(issues, "mail_from") {
		t.Errorf("without the sender: issues = %+v", issues)
	}
	// An address alone selects nothing.
	if cfg, issues := loadPlugin(t, map[string]string{"MY_PLUGIN_MAILER_ADDRESS": address}); len(issues) != 0 || cfg.Mail != "none" {
		t.Errorf("an address without mail = plugin: mail = %q, issues = %+v", cfg.Mail, issues)
	}

	// Plain TCP stops the start, on loopback too.
	for _, plain := range []string{"http://127.0.0.1:8080", "http://localhost:8080", "http://mail.internal:8080"} {
		if _, issues := loadPlugin(t, map[string]string{"MY_MAIL": "plugin", "MY_MAIL_FROM": from, "MY_PLUGIN_MAILER_ADDRESS": plain}); !hasIssue(issues, "plugin_mailer_address") {
			t.Errorf("%s: issues = %+v", plain, issues)
		}
	}
}
