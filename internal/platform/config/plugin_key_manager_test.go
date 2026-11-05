// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import "testing"

func TestPluginKeyManager(t *testing.T) {
	const address = "unix:///run/my/key_manager.sock"
	const cloudKey = "projects/p/locations/global/keyRings/r/cryptoKeys/k"
	cfg, issues := loadPlugin(t, map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": address})
	if len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
	if cfg.KeyManager != "plugin" || cfg.PluginDestination("key_manager").Address != address {
		t.Errorf("config = %q, %+v", cfg.KeyManager, cfg.PluginDestination("key_manager"))
	}

	// The address is required once the plugin is selected, and kms_key is not.
	_, issues = loadPlugin(t, map[string]string{"MY_KEY_MANAGER": "plugin"})
	if !hasIssue(issues, "plugin_key_manager_address") || hasIssue(issues, "kms_key") {
		t.Errorf("without the address: issues = %+v", issues)
	}

	// The local KEK may stay to decrypt, while the secrets move to the plugin.
	if _, issues := loadPlugin(t, map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": address, "MY_KEK_PREVIOUS": testKEK1}); len(issues) != 0 {
		t.Errorf("with kek_previous: issues = %+v", issues)
	}

	// While the secrets move back, the plugin only decrypts.
	if cfg, issues := loadPlugin(t, map[string]string{"MY_PLUGIN_KEY_MANAGER_ADDRESS": address}); len(issues) != 0 || cfg.KeyManager != "local" {
		t.Errorf("an address with key_manager = local: %q, issues = %+v", cfg.KeyManager, issues)
	}

	// A plugin and a KMS never serve together: the secrets move between them through the local KEK.
	for name, tc := range map[string]struct {
		env   map[string]string
		issue string
	}{
		"plugin with kms_key": {
			map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": address, "MY_KMS_KEY": cloudKey}, "kms_key",
		},
		"plugin with kms_key_previous": {
			map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": address, "MY_KMS_KEY_PREVIOUS": cloudKey}, "kms_key_previous",
		},
		"local with both": {
			map[string]string{"MY_PLUGIN_KEY_MANAGER_ADDRESS": address, "MY_KMS_KEY": cloudKey}, "kms_key",
		},
		"a KMS with an address": {
			map[string]string{"MY_KEY_MANAGER": "gcpkms", "MY_KMS_KEY": cloudKey, "MY_PLUGIN_KEY_MANAGER_ADDRESS": address}, "plugin_key_manager_address",
		},
	} {
		if _, issues := loadPlugin(t, tc.env); !hasIssue(issues, tc.issue) {
			t.Errorf("%s: issues = %+v", name, issues)
		}
	}

	// Plain TCP stops the start, on loopback too.
	for _, plain := range []string{"http://127.0.0.1:8200", "http://localhost:8200", "http://[::1]:8200", "http://vault.internal:8200"} {
		if _, issues := loadPlugin(t, map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": plain}); !hasIssue(issues, "plugin_key_manager_address") {
			t.Errorf("%s: issues = %+v", plain, issues)
		}
	}
	// ... and in the role that only decrypts.
	if _, issues := loadPlugin(t, map[string]string{"MY_PLUGIN_KEY_MANAGER_ADDRESS": "http://127.0.0.1:8200"}); !hasIssue(issues, "plugin_key_manager_address") {
		t.Errorf("plain TCP with key_manager = local: issues = %+v", issues)
	}
	// So does a TLS destination stay accepted.
	if _, issues := loadPlugin(t, map[string]string{"MY_KEY_MANAGER": "plugin", "MY_PLUGIN_KEY_MANAGER_ADDRESS": "https://vault.internal:8200", "MY_PLUGIN_KEY_MANAGER_TOKEN": "t"}); len(issues) != 0 {
		t.Errorf("https://: issues = %+v", issues)
	}
}
