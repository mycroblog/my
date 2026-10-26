// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

const testJoinSecret = "0123456789abcdef0123456789abcdef"

// soraSelfHosted is a valid environment of voice = sora on a self-hosted Sora.
func soraSelfHosted() map[string]string {
	return with(minimal(), "MY_VOICE", "sora", "MY_VOICE_SORA_MODE", "self_hosted",
		"MY_VOICE_SORA_SIGNALING_URLS", "wss://sora1.example.org/signaling, wss://sora2.example.org/signaling",
		"MY_VOICE_SORA_API_URL", "https://sora-api.example.org", "MY_VOICE_SORA_WEBHOOK_USERNAME", "sora",
		"MY_VOICE_SORA_WEBHOOK_PASSWORD", "hunter2", "MY_VOICE_JOIN_SECRET", testJoinSecret)
}

// TestLoadVoice checks the voice settings that start (ADR-0101 §1, ADR-0102 §5).
func TestLoadVoice(t *testing.T) {
	t.Run("off checks nothing else", func(t *testing.T) {
		cfg, _, err := Load(source(with(minimal(), "MY_VOICE_ICE_TRANSPORT_POLICY", "relay", "MY_VOICE_TURN", "coturn"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Voice != "off" || cfg.VoiceLiveKitAPIURL != "" || len(VoiceWarnings(cfg)) != 0 {
			t.Errorf("voice = %q, api url %q, warnings %v; want off and nothing", cfg.Voice, cfg.VoiceLiveKitAPIURL, VoiceWarnings(cfg))
		}
	})
	t.Run("p2p with coturn and relay", func(t *testing.T) {
		cfg, _, err := Load(source(with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_P2P_MAX_PARTICIPANTS", "50",
			"MY_VOICE_MAX_ROOM_DURATION", "4h", "MY_VOICE_MAX_OPEN_ROOMS", "20",
			"MY_VOICE_STUN_URLS", "stun:stun.example.org:3478, stuns:stun.example.org",
			"MY_VOICE_TURN", "coturn", "MY_VOICE_TURN_URLS", "turn:turn.example.org:3478?transport=udp,turns:turn.example.org:5349",
			"MY_VOICE_TURN_SECRET_FILE", "/run/secrets/turn", "MY_VOICE_ICE_TRANSPORT_POLICY", "relay"),
			map[string]string{"/run/secrets/turn": "s3cret\n"}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceP2PMaxParticipants != 50 || cfg.VoiceMaxRoomDuration != 4*time.Hour || cfg.VoiceMaxOpenRooms != 20 ||
			!slices.Equal(cfg.VoiceSTUNURLs, []string{"stun:stun.example.org:3478", "stuns:stun.example.org"}) ||
			!slices.Equal(cfg.VoiceTURNURLs, []string{"turn:turn.example.org:3478?transport=udp", "turns:turn.example.org:5349"}) ||
			cfg.VoiceTURNSecret != "s3cret" || cfg.VoiceICETransportPolicy != "relay" || cfg.VoiceTURNCredentialTTL != 6*time.Hour {
			t.Errorf("config = %+v", cfg)
		}
		if w := VoiceWarnings(cfg); len(w) != 0 {
			t.Errorf("warnings = %v, want none", w)
		}
	})
	t.Run("p2p with cloudflare at 48h", func(t *testing.T) {
		cfg, _, err := Load(source(with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_TURN", "cloudflare",
			"MY_VOICE_TURN_CLOUDFLARE_KEY_ID", "key", "MY_VOICE_TURN_CLOUDFLARE_API_TOKEN", "token", "MY_VOICE_TURN_CREDENTIAL_TTL", "48h"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceTURNCloudflareKeyID != "key" || cfg.VoiceTURNCloudflareAPIToken != "token" || cfg.VoiceTURNCredentialTTL != 48*time.Hour {
			t.Errorf("config = %+v", cfg)
		}
	})
	t.Run("livekit derives its API URL", func(t *testing.T) {
		for _, tc := range []struct{ url, api string }{
			{"wss://livekit.example.org", "https://livekit.example.org"},
			{"ws://localhost:7880", "http://localhost:7880"},
		} {
			cfg, _, err := Load(source(with(minimal(), "MY_VOICE", "livekit", "MY_VOICE_LIVEKIT_URL", tc.url,
				"MY_VOICE_LIVEKIT_API_KEY", "APIkey", "MY_VOICE_LIVEKIT_API_SECRET", "secret"), nil))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.VoiceLiveKitAPIURL != tc.api || cfg.VoiceLiveKitAPIKey != "APIkey" || cfg.VoiceLiveKitAPISecret != "secret" {
				t.Errorf("livekit %s: api url %q, want %q", tc.url, cfg.VoiceLiveKitAPIURL, tc.api)
			}
		}
		cfg, _, err := Load(source(with(minimal(), "MY_VOICE", "livekit", "MY_VOICE_LIVEKIT_URL", "wss://livekit.example.org",
			"MY_VOICE_LIVEKIT_API_URL", "https://livekit-api.internal.example.org", "MY_VOICE_LIVEKIT_API_KEY", "k", "MY_VOICE_LIVEKIT_API_SECRET", "s"), nil))
		if err != nil || cfg.VoiceLiveKitAPIURL != "https://livekit-api.internal.example.org" {
			t.Errorf("explicit api url = %+v, %v", cfg, err)
		}
	})
	t.Run("sora self-hosted and cloud", func(t *testing.T) {
		cfg, _, err := Load(source(with(soraSelfHosted(), "MY_VOICE_SORA_API_HEADER_NAME", "Authorization", "MY_VOICE_SORA_API_HEADER_VALUE", "Basic abc"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceSoraMode != "self_hosted" || len(cfg.VoiceSoraSignalingURLs) != 2 || cfg.VoiceSoraAPIHeaderName != "Authorization" ||
			cfg.VoiceSoraAPIHeaderValue != "Basic abc" || cfg.VoiceSoraWebhookPassword != "hunter2" || cfg.VoiceJoinSecret != testJoinSecret {
			t.Errorf("config = %+v", cfg)
		}
		cfg, _, err = Load(source(with(minimal(), "MY_VOICE", "sora", "MY_VOICE_SORA_MODE", "cloud",
			"MY_VOICE_SORA_SIGNALING_URLS", "wss://sora.example.com/signaling", "MY_VOICE_SORA_CLOUD_PROJECT_ID", "project",
			"MY_VOICE_SORA_CLOUD_API_KEY", "key", "MY_VOICE_JOIN_SECRET", testJoinSecret), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceSoraCloudProjectID != "project" || cfg.VoiceSoraCloudAPIKey != "key" {
			t.Errorf("config = %+v", cfg)
		}
	})
}

// TestLoadVoiceInvalid checks every start-up check of the voice settings (ADR-0101 §1,
// ADR-0102 §5): a configuration that cannot connect a room does not start.
func TestLoadVoiceInvalid(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		file  string
		files map[string]string
		want  []string
	}{
		{
			name: "relay without TURN",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_ICE_TRANSPORT_POLICY", "relay"),
			want: []string{"voice_ice_transport_policy (MY_VOICE_ICE_TRANSPORT_POLICY): relay needs a TURN server; set voice_turn"},
		},
		{
			name: "coturn without URLs and secret",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_TURN", "coturn"),
			want: []string{
				"voice_turn_urls (MY_VOICE_TURN_URLS): required when voice_turn is coturn",
				"voice_turn_secret (MY_VOICE_TURN_SECRET): required when voice_turn is coturn",
			},
		},
		{
			name: "cloudflare without key and token",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_TURN", "cloudflare"),
			want: []string{
				"voice_turn_cloudflare_key_id (MY_VOICE_TURN_CLOUDFLARE_KEY_ID): required when voice_turn is cloudflare",
				"voice_turn_cloudflare_api_token (MY_VOICE_TURN_CLOUDFLARE_API_TOKEN): required when voice_turn is cloudflare",
			},
		},
		{
			name: "cloudflare TTL over 48h",
			env: with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_TURN", "cloudflare", "MY_VOICE_TURN_CLOUDFLARE_KEY_ID", "key",
				"MY_VOICE_TURN_CLOUDFLARE_API_TOKEN", "token", "MY_VOICE_TURN_CREDENTIAL_TTL", "48h1s"),
			want: []string{"voice_turn_credential_ttl (MY_VOICE_TURN_CREDENTIAL_TTL): must be at most 48h with voice_turn = cloudflare"},
		},
		{
			name: "p2p room limits out of range",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_P2P_MAX_PARTICIPANTS", "51", "MY_VOICE_MAX_OPEN_ROOMS", "-1", "MY_VOICE_MAX_ROOM_DURATION", "-1h"),
			want: []string{
				"voice_p2p_max_participants (MY_VOICE_P2P_MAX_PARTICIPANTS): must be a whole number from 2 to 50",
				"voice_max_room_duration (MY_VOICE_MAX_ROOM_DURATION): must be 0 or a positive Go duration",
				"voice_max_open_rooms (MY_VOICE_MAX_OPEN_ROOMS): must be a whole number from 0",
			},
		},
		{
			name: "STUN and TURN URLs of the wrong schemes",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_STUN_URLS", "turn:turn.example.org", "MY_VOICE_TURN", "coturn", "MY_VOICE_TURN_URLS", "https://turn.example.org", "MY_VOICE_TURN_SECRET", "s"),
			want: []string{
				"voice_stun_urls (MY_VOICE_STUN_URLS): must be comma-separated stun: or stuns: URLs",
				"voice_turn_urls (MY_VOICE_TURN_URLS): must be comma-separated turn: or turns: URLs",
			},
		},
		{
			name: "livekit without its server",
			env:  with(minimal(), "MY_VOICE", "livekit"),
			want: []string{
				"voice_livekit_url (MY_VOICE_LIVEKIT_URL): required when voice is livekit",
				"voice_livekit_api_key (MY_VOICE_LIVEKIT_API_KEY): required when voice is livekit",
				"voice_livekit_api_secret (MY_VOICE_LIVEKIT_API_SECRET): required when voice is livekit",
			},
		},
		{
			name: "livekit over plain ws to a remote host",
			env: with(minimal(), "MY_VOICE", "livekit", "MY_VOICE_LIVEKIT_URL", "ws://livekit.example.org",
				"MY_VOICE_LIVEKIT_API_URL", "http://livekit.example.org", "MY_VOICE_LIVEKIT_API_KEY", "k", "MY_VOICE_LIVEKIT_API_SECRET", "s"),
			want: []string{
				"voice_livekit_url (MY_VOICE_LIVEKIT_URL): must be wss; ws is accepted only for localhost",
				"voice_livekit_api_url (MY_VOICE_LIVEKIT_API_URL): must be https; http is accepted only for localhost",
			},
		},
		{
			name:  "livekit secret in the config file",
			env:   with(minimal(), "MY_VOICE", "livekit", "MY_VOICE_LIVEKIT_URL", "wss://livekit.example.org", "MY_VOICE_LIVEKIT_API_KEY", "k"),
			file:  "my.toml",
			files: map[string]string{"my.toml": "voice_livekit_api_secret = \"s\"\n"},
			want: []string{
				"voice_livekit_api_secret (MY_VOICE_LIVEKIT_API_SECRET): is a secret; set MY_VOICE_LIVEKIT_API_SECRET or MY_VOICE_LIVEKIT_API_SECRET_FILE instead of the config file",
			},
		},
		{
			name: "sora without mode, signaling, and join secret",
			env:  with(minimal(), "MY_VOICE", "sora"),
			want: []string{
				"voice_sora_mode (MY_VOICE_SORA_MODE): required when voice is sora",
				"voice_sora_signaling_urls (MY_VOICE_SORA_SIGNALING_URLS): required when voice is sora",
				"voice_join_secret (MY_VOICE_JOIN_SECRET): required when voice is sora",
			},
		},
		{
			name: "sora self_hosted without webhook Basic auth and API",
			env:  with(soraSelfHosted(), "MY_VOICE_SORA_API_URL", unset, "MY_VOICE_SORA_WEBHOOK_USERNAME", unset, "MY_VOICE_SORA_WEBHOOK_PASSWORD", unset),
			want: []string{
				"voice_sora_api_url (MY_VOICE_SORA_API_URL): required when voice_sora_mode is self_hosted",
				"voice_sora_webhook_username (MY_VOICE_SORA_WEBHOOK_USERNAME): required when voice_sora_mode is self_hosted",
				"voice_sora_webhook_password (MY_VOICE_SORA_WEBHOOK_PASSWORD): required when voice_sora_mode is self_hosted",
			},
		},
		{
			name: "sora self_hosted without the webhook password",
			env:  with(soraSelfHosted(), "MY_VOICE_SORA_WEBHOOK_PASSWORD", unset),
			want: []string{"voice_sora_webhook_password (MY_VOICE_SORA_WEBHOOK_PASSWORD): required when voice_sora_mode is self_hosted"},
		},
		{
			name: "sora API header without its value",
			env:  with(soraSelfHosted(), "MY_VOICE_SORA_API_HEADER_NAME", "Authorization"),
			want: []string{"voice_sora_api_header_value (MY_VOICE_SORA_API_HEADER_VALUE): required when voice_sora_api_header_name is set"},
		},
		{
			name: "sora API header value without its name",
			env:  with(soraSelfHosted(), "MY_VOICE_SORA_API_HEADER_VALUE", "Basic abc"),
			want: []string{"voice_sora_api_header_name (MY_VOICE_SORA_API_HEADER_NAME): required when voice_sora_api_header_value is set"},
		},
		{
			name: "sora cloud without project and key",
			env:  with(soraSelfHosted(), "MY_VOICE_SORA_MODE", "cloud"),
			want: []string{
				"voice_sora_cloud_project_id (MY_VOICE_SORA_CLOUD_PROJECT_ID): required when voice_sora_mode is cloud",
				"voice_sora_cloud_api_key (MY_VOICE_SORA_CLOUD_API_KEY): required when voice_sora_mode is cloud",
			},
		},
		{
			name: "sora with a short join secret and an insecure signaling URL",
			env:  with(soraSelfHosted(), "MY_VOICE_JOIN_SECRET", "short", "MY_VOICE_SORA_SIGNALING_URLS", "wss://sora.example.org/signaling, ws://sora.example.org/signaling"),
			want: []string{
				"voice_sora_signaling_urls (MY_VOICE_SORA_SIGNALING_URLS): must be wss; ws is accepted only for localhost",
				"voice_join_secret (MY_VOICE_JOIN_SECRET): must be at least 32 bytes",
			},
		},
		{
			name: "unknown voice method",
			env:  with(minimal(), "MY_VOICE", "jitsi"),
			want: []string{`voice (MY_VOICE): unknown value "jitsi"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := source(tt.env, tt.files)
			src.File = tt.file
			cfg, _, err := Load(src)
			if cfg != nil {
				t.Errorf("Load() config = %+v, want nil", cfg)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Load() error = %v, want *ValidationError", err)
			}
			got := make([]string, len(ve.Issues))
			for i, is := range ve.Issues {
				got[i] = is.String()
			}
			if len(got) != len(tt.want) {
				t.Fatalf("issues =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
			for i := range got {
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("issue %d = %q, want prefix %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestVoiceWarnings checks the warnings at start: p2p without STUN and TURN, and the STUN and
// TURN settings that an SFU does not use.
func TestVoiceWarnings(t *testing.T) {
	keys := func(env map[string]string) []string {
		t.Helper()
		cfg, _, err := Load(source(env, nil))
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range VoiceWarnings(cfg) {
			if f.Severity != SeverityWarning {
				t.Errorf("%s: severity %s, want warning", f.Key, f.Severity)
			}
			out = append(out, f.Key)
		}
		return out
	}
	if got := keys(with(minimal(), "MY_VOICE", "p2p")); !slices.Equal(got, []string{"voice"}) {
		t.Errorf("p2p without STUN and TURN warns about %v, want voice", got)
	}
	if got := keys(with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_STUN_URLS", "stun:stun.example.org")); len(got) != 0 {
		t.Errorf("p2p with STUN warns about %v, want nothing", got)
	}
	if got := keys(with(soraSelfHosted(), "MY_VOICE_STUN_URLS", "stun:stun.example.org", "MY_VOICE_TURN", "coturn",
		"MY_VOICE_ICE_TRANSPORT_POLICY", "relay")); !slices.Equal(got, []string{"voice_stun_urls", "voice_turn", "voice_ice_transport_policy"}) {
		t.Errorf("sora with STUN and TURN warns about %v", got)
	}
	if got := keys(soraSelfHosted()); len(got) != 0 {
		t.Errorf("sora warns about %v, want nothing", got)
	}
}
