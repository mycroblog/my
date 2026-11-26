// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestLoadVoice checks the voice settings that start (ADR-0101 §1, ADR-0101 §14).
func TestLoadVoice(t *testing.T) {
	t.Run("the limits have defaults", func(t *testing.T) {
		cfg, _, err := Load(source(minimal(), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceMaxRoomDuration != 4*time.Hour || cfg.VoiceMaxParticipants != 10 || cfg.VoiceMaxOpenRooms != 10 {
			t.Errorf("limits = %v, %d, %d; want 4h, 10, 10", cfg.VoiceMaxRoomDuration, cfg.VoiceMaxParticipants, cfg.VoiceMaxOpenRooms)
		}
	})
	t.Run("a limit of 0 removes it", func(t *testing.T) {
		cfg, _, err := Load(source(with(minimal(), "MY_VOICE_MAX_ROOM_DURATION", "0", "MY_VOICE_MAX_OPEN_ROOMS", "0"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceMaxRoomDuration != 0 || cfg.VoiceMaxOpenRooms != 0 {
			t.Errorf("limits = %v, %d; want none", cfg.VoiceMaxRoomDuration, cfg.VoiceMaxOpenRooms)
		}
	})
	t.Run("off checks nothing else", func(t *testing.T) {
		cfg, _, err := Load(source(with(minimal(), "MY_VOICE_ICE_TRANSPORT_POLICY", "relay", "MY_VOICE_TURN", "coturn"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Voice != "off" || len(VoiceWarnings(cfg)) != 0 {
			t.Errorf("voice = %q, warnings %v; want off and nothing", cfg.Voice, VoiceWarnings(cfg))
		}
	})
	t.Run("p2p with coturn and relay", func(t *testing.T) {
		cfg, _, err := Load(source(with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_MAX_PARTICIPANTS", "50",
			"MY_VOICE_MAX_ROOM_DURATION", "4h", "MY_VOICE_MAX_OPEN_ROOMS", "20",
			"MY_VOICE_STUN_URLS", "stun:stun.example.org:3478, stuns:stun.example.org",
			"MY_VOICE_TURN", "coturn", "MY_VOICE_TURN_COTURN_URLS", "turn:turn.example.org:3478?transport=udp,turns:turn.example.org:5349",
			"MY_VOICE_TURN_COTURN_SECRET_FILE", "/run/secrets/turn", "MY_VOICE_ICE_TRANSPORT_POLICY", "relay"),
			map[string]string{"/run/secrets/turn": "s3cret\n"}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceMaxParticipants != 50 || cfg.VoiceMaxRoomDuration != 4*time.Hour || cfg.VoiceMaxOpenRooms != 20 ||
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
}

// TestLoadVoiceInvalid checks every start-up check of the voice settings (ADR-0101 §1,
// ADR-0101 §14): a configuration that cannot connect a room does not start.
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
				"voice_turn_coturn_urls (MY_VOICE_TURN_COTURN_URLS): required when voice_turn is coturn",
				"voice_turn_coturn_secret (MY_VOICE_TURN_COTURN_SECRET): required when voice_turn is coturn",
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
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_MAX_PARTICIPANTS", "51", "MY_VOICE_MAX_OPEN_ROOMS", "-1", "MY_VOICE_MAX_ROOM_DURATION", "-1h"),
			want: []string{
				"voice_max_room_duration (MY_VOICE_MAX_ROOM_DURATION): must be 0 or a positive Go duration",
				"voice_max_open_rooms (MY_VOICE_MAX_OPEN_ROOMS): must be a whole number from 0",
				"voice_max_participants (MY_VOICE_MAX_PARTICIPANTS): must be at most 50 with voice = p2p",
			},
		},
		{
			name: "STUN and TURN URLs of the wrong schemes",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_STUN_URLS", "turn:turn.example.org", "MY_VOICE_TURN", "coturn", "MY_VOICE_TURN_COTURN_URLS", "https://turn.example.org", "MY_VOICE_TURN_COTURN_SECRET", "s"),
			want: []string{
				"voice_stun_urls (MY_VOICE_STUN_URLS): must be comma-separated stun: or stuns: URLs",
				"voice_turn_coturn_urls (MY_VOICE_TURN_COTURN_URLS): must be comma-separated turn: or turns: URLs",
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

// TestLoadRemovedVoiceMethods checks that a deployment set for LiveKit or Sora, which the voice
// setting no longer takes, does not start, and that the error tells to set voice = sfu and what it
// needs, or voice = p2p.
func TestLoadRemovedVoiceMethods(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		file string
		want []string
	}{
		{
			name: "voice = livekit",
			env:  with(minimal(), "MY_VOICE", "livekit"),
			want: []string{`voice (MY_VOICE): "livekit" was removed: LiveKit is no longer supported; set voice = sfu, where the server's own media nodes carry the audio: a process with role = all or sfu runs one, which the browsers reach at voice_sfu_public_url over WebSocket and at voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) for the audio; or set voice = p2p. A room still open on LiveKit ends at the first sweep.`},
		},
		{
			name: "voice = sora",
			env:  with(minimal(), "MY_VOICE", "sora"),
			want: []string{`voice (MY_VOICE): "sora" was removed: Sora is no longer supported; set voice = sfu, where the server's own media nodes carry the audio: a process with role = all or sfu runs one, which the browsers reach at voice_sfu_public_url over WebSocket and at voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) for the audio; or set voice = p2p. A room still open on Sora ends at the first sweep.`},
		},
		{
			name: "a LiveKit setting left with voice = sfu",
			env:  with(minimal(), "MY_VOICE", "sfu", "MY_VOICE_LIVEKIT_API_SECRET_FILE", "/run/secrets/livekit"),
			want: []string{"environment variable MY_VOICE_LIVEKIT_API_SECRET_FILE was removed: LiveKit is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
		},
		{
			name: "a Sora setting in the config file",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_STUN_URLS", "stun:stun.example.org"),
			file: `voice_sora_signaling_urls = "wss://sora.example.org/signaling"`,
			want: []string{`config file my.toml: key "voice_sora_signaling_urls" was removed: Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var files map[string]string
			if tc.file != "" {
				files = map[string]string{"my.toml": tc.file}
			}
			src := envSource(tc.env, files)
			if files != nil {
				src.File = "my.toml"
			}
			cfg, _, err := Load(src)
			if cfg != nil {
				t.Fatalf("Load() config = %+v, want nil", cfg)
			}
			if got := issueStrings(t, err); !slices.Equal(got, tc.want) {
				t.Errorf("issues =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
	for i := range schema {
		if f := &schema[i]; f.key == "voice" && slices.ContainsFunc(f.choices, func(c choice) bool { return c.value == "livekit" || c.value == "sora" }) {
			t.Errorf("voice still takes %v", allowedValues(f))
		}
	}
}

// TestVoiceWarnings checks the warning at start of p2p without STUN and TURN.
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
}

// TestConfigKeyNames keeps the settings of an adapter under <selector>_<adapter>_ and the selectors
// bare, so that a key says what it configures.
func TestConfigKeyNames(t *testing.T) {
	keys := map[string]bool{}
	for _, k := range schema {
		keys[k.key] = true
	}
	for _, want := range []string{
		"database", "payments", "voice_turn_coturn_urls", "voice_turn_coturn_secret",
		"stream_sns_queue_prefix", "mail_smtp_host", "mail_smtp_port", "mail_smtp_username", "mail_smtp_password",
		"mail_ses_region", "mail_ses_configuration_set", "mail_ses_sns_topic_arn", "voice_max_participants",
		"relay_max_activities_per_hour", "media_unattached_retention", "dynamic_client_unredeemed_retention", "dynamic_client_idle_retention",
	} {
		if !keys[want] {
			t.Errorf("no setting %s", want)
		}
	}
	for _, old := range []string{"db", "payments_provider", "smtp_host", "ses_region", "voice_p2p_max_participants", "voice_sfu_max_participants", "voice_livekit_url", "voice_sora_join_secret", "relay_ingest_hourly_limit", "media_unattached_ttl"} {
		if keys[old] {
			t.Errorf("the setting %s still exists", old)
		}
	}
}

// TestNoLimitIsSpelledZero accepts 0 for no limit in a duration limit, and a retry delay as a Go
// duration like the other time settings.
func TestNoLimitIsSpelledZero(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_VOICE_MAX_ROOM_DURATION", "0", "MY_MAINTENANCE_RETRY_AFTER", "10m"), nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VoiceMaxRoomDuration != 0 || cfg.MaintenanceRetryAfter != 10*time.Minute {
		t.Errorf("limit = %v, retry after = %v", cfg.VoiceMaxRoomDuration, cfg.MaintenanceRetryAfter)
	}
}
