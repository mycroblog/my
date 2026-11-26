// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"slices"
	"testing"
)

// TestVoiceSFUConnectSources checks the origins of the media nodes that the web client may
// connect to (ADR-0101 §11): none without voice = sfu, and otherwise voice_sfu_origins and the
// origin of the process's own node.
func TestVoiceSFUConnectSources(t *testing.T) {
	for _, c := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"p2p", with(sfu(), "MY_VOICE", "p2p"), nil},
		{"sfu on the server", sfu(), []string{"wss://social.example.org"}},
		{"sfu with other nodes", with(sfu(), "MY_VOICE_SFU_ORIGINS", "wss://*.sfu.example.org, wss://sfu.example.net:8443"),
			[]string{"wss://*.sfu.example.org", "wss://sfu.example.net:8443", "wss://social.example.org"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg, _, err := Load(source(c.env, nil))
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.VoiceSFUConnectSources(); !slices.Equal(got, c.want) {
				t.Errorf("VoiceSFUConnectSources() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestVoiceSFUOriginsInvalid checks that voice_sfu_origins takes only wss: origins.
func TestVoiceSFUOriginsInvalid(t *testing.T) {
	for _, v := range []string{"https://sfu.example.org", "wss://sfu.example.org/voice/sfu", "ws://sfu.example.org", "sfu.example.org", "wss://sfu.example.org?x=1"} {
		if _, _, err := Load(source(with(sfu(), "MY_VOICE_SFU_ORIGINS", v), nil)); err == nil {
			t.Errorf("voice_sfu_origins %q: Load accepted it", v)
		}
	}
	if _, _, err := Load(source(with(sfu(), "MY_VOICE_SFU_ORIGINS", "ws://127.0.0.1:7880"), nil)); err != nil {
		t.Errorf("a ws: origin on loopback: %v", err)
	}
}
