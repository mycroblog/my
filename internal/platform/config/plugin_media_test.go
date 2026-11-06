// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_plugins

package config

import (
	"strings"
	"testing"
)

// mediaSetting returns the selection of key in cfg: media_image, media_video, or media_audio.
func mediaSetting(cfg *Config, key string) string {
	return map[string]string{"media_image": cfg.MediaImage, "media_video": cfg.MediaVideo, "media_audio": cfg.MediaAudio}[key]
}

func TestPluginMedia(t *testing.T) {
	const address = "unix:///run/my/media.sock"
	for _, key := range []string{"media_image", "media_video", "media_audio"} {
		t.Run(key, func(t *testing.T) {
			env := "MY_" + strings.ToUpper(key)
			addressEnv, addressKey := "MY_PLUGIN_"+strings.ToUpper(key)+"_ADDRESS", "plugin_"+key+"_address"
			protocolEnv, protocolKey := "MY_PLUGIN_"+strings.ToUpper(key)+"_PROTOCOL", "plugin_"+key+"_protocol"

			cfg, issues := loadPlugin(t, map[string]string{env: "plugin", addressEnv: address})
			if len(issues) != 0 {
				t.Fatalf("issues = %+v", issues)
			}
			if d := cfg.PluginDestination(key); mediaSetting(cfg, key) != "plugin" || d.Address != address || d.Protocol != "" {
				t.Errorf("config = %q, %+v", mediaSetting(cfg, key), d)
			}
			// The other kinds stay in the server.
			for _, other := range []string{"media_image", "media_video", "media_audio"} {
				if other != key && mediaSetting(cfg, other) != "native" {
					t.Errorf("%s = %q, want native", other, mediaSetting(cfg, other))
				}
			}

			// The address is required once the plugin is selected, and an address alone selects nothing.
			if _, issues := loadPlugin(t, map[string]string{env: "plugin"}); !hasIssue(issues, addressKey) {
				t.Errorf("without the address: issues = %+v", issues)
			}
			if cfg, issues := loadPlugin(t, map[string]string{addressEnv: address}); len(issues) != 0 || mediaSetting(cfg, key) != "native" {
				t.Errorf("an address without %s = plugin: %q, issues = %+v", key, mediaSetting(cfg, key), issues)
			}

			// The calls stream both ways, which needs HTTP/2: json stops the start, proto and grpc do not.
			_, issues = loadPlugin(t, map[string]string{env: "plugin", addressEnv: address, protocolEnv: "json"})
			if !hasIssue(issues, protocolKey) {
				t.Errorf("json: issues = %+v", issues)
			}
			for _, protocol := range []string{"proto", "grpc"} {
				if _, issues := loadPlugin(t, map[string]string{env: "plugin", addressEnv: address, protocolEnv: protocol}); len(issues) != 0 {
					t.Errorf("%s: issues = %+v", protocol, issues)
				}
			}

			// Plain TCP works, with a warning that names the kind and the address.
			const plain = "http://media.internal:9000"
			cfg, issues = loadPlugin(t, map[string]string{env: "plugin", addressEnv: plain})
			if len(issues) != 0 {
				t.Fatalf("plain TCP: issues = %+v", issues)
			}
			if w := PluginWarnings(cfg); len(w) != 1 || w[0].Key != addressKey || !strings.Contains(w[0].Message, key) || !strings.Contains(w[0].Message, plain) {
				t.Errorf("plain TCP: warnings = %+v", w)
			}
		})
	}
}

// One plugin may take every kind: the three addresses may be the same.
func TestPluginMediaOneAddressForEveryKind(t *testing.T) {
	const address = "unix:///run/my/media.sock"
	vals := map[string]string{}
	for _, key := range []string{"MEDIA_IMAGE", "MEDIA_VIDEO", "MEDIA_AUDIO"} {
		vals["MY_"+key], vals["MY_PLUGIN_"+key+"_ADDRESS"] = "plugin", address
	}
	if _, issues := loadPlugin(t, vals); len(issues) != 0 {
		t.Errorf("issues = %+v", issues)
	}
}
