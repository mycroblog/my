// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"strings"
	"testing"
)

// sfu is a valid environment of voice = sfu with the defaults.
func sfu() map[string]string { return with(minimal(), "MY_VOICE", "sfu") }

// TestLoadVoiceSFU checks the settings of voice = sfu that start (ADR-0101 §1, §15).
func TestLoadVoiceSFU(t *testing.T) {
	t.Run("the defaults", func(t *testing.T) {
		cfg, _, err := Load(source(sfu(), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Voice != "sfu" || cfg.VoiceMaxParticipants != 10 || cfg.VoiceSFUUDPPort != 7882 || cfg.VoiceSFUTCPPort != 7881 ||
			cfg.VoiceSFUPublicIP != "" || cfg.VoiceSFUPublicURL != "" || cfg.VoiceSFUNodeCapacity != 500 {
			t.Errorf("voice %q, max %d, udp %d, tcp %d, ip %q, url %q, capacity %d; want sfu, 10, 7882, 7881, none, none, 500",
				cfg.Voice, cfg.VoiceMaxParticipants, cfg.VoiceSFUUDPPort, cfg.VoiceSFUTCPPort, cfg.VoiceSFUPublicIP, cfg.VoiceSFUPublicURL, cfg.VoiceSFUNodeCapacity)
		}
	})
	t.Run("every key set", func(t *testing.T) {
		cfg, _, err := Load(source(with(sfu(), "MY_VOICE_MAX_PARTICIPANTS", "500",
			"MY_VOICE_SFU_UDP_PORT", "50000", "MY_VOICE_SFU_TCP_PORT", "443", "MY_VOICE_SFU_PUBLIC_IP", "203.0.113.7",
			"MY_VOICE_SFU_PUBLIC_URL", "wss://sfu.example.org/voice/sfu", "MY_VOICE_SFU_NODE_CAPACITY", "1"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceMaxParticipants != 500 || cfg.VoiceSFUUDPPort != 50000 || cfg.VoiceSFUTCPPort != 443 || cfg.VoiceSFUPublicIP != "203.0.113.7" ||
			cfg.VoiceSFUPublicURL != "wss://sfu.example.org/voice/sfu" || cfg.VoiceSFUNodeCapacity != 1 {
			t.Errorf("config = %+v", cfg)
		}
	})
	t.Run("the limits of the ranges", func(t *testing.T) {
		for _, env := range []map[string]string{
			with(sfu(), "MY_VOICE_MAX_PARTICIPANTS", "2", "MY_VOICE_SFU_UDP_PORT", "1", "MY_VOICE_SFU_TCP_PORT", "65535"),
			with(sfu(), "MY_VOICE_SFU_PUBLIC_IP", "2001:db8::7"),
			with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_MAX_PARTICIPANTS", "2"),
		} {
			if _, _, err := Load(source(env, nil)); err != nil {
				t.Errorf("Load(%v) = %v, want nil", env, err)
			}
		}
	})
	t.Run("ws on loopback", func(t *testing.T) {
		for _, url := range []string{"ws://127.0.0.1:7880/voice/sfu", "ws://[::1]:7880/voice/sfu"} {
			cfg, _, err := Load(source(with(sfu(), "MY_VOICE_SFU_PUBLIC_URL", url), nil))
			if err != nil {
				t.Errorf("voice_sfu_public_url %s: %v, want nil", url, err)
				continue
			}
			if cfg.VoiceSFUURL() != url {
				t.Errorf("VoiceSFUURL() = %q, want %q", cfg.VoiceSFUURL(), url)
			}
		}
	})
	t.Run("TURN with sfu", func(t *testing.T) {
		cfg, _, err := Load(source(with(sfu(), "MY_VOICE_TURN", "coturn", "MY_VOICE_TURN_COTURN_URLS", "turns:turn.example.org:5349",
			"MY_VOICE_TURN_COTURN_SECRET", "s3cret"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoiceTURN != "coturn" || cfg.VoiceTURNSecret != "s3cret" {
			t.Errorf("turn %q, secret %q; want coturn and its secret", cfg.VoiceTURN, cfg.VoiceTURNSecret)
		}
	})
	t.Run("role sfu", func(t *testing.T) {
		cfg, _, err := Load(source(with(sfu(), "MY_ROLE", "sfu"), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Role != RoleSFU {
			t.Errorf("role = %q, want sfu", cfg.Role)
		}
	})
}

// TestVoiceSFUURL checks the URL of the media node's WebSocket (ADR-0101 §11).
func TestVoiceSFUURL(t *testing.T) {
	for _, c := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"on the server's https origin", sfu(), "wss://social.example.org" + VoiceSFUPath},
		{"on a plain-HTTP origin", with(sfu(), "MY_HOST", "localhost:8080", "MY_WEB_ORIGIN", "http://localhost:8080"), "ws://localhost:8080" + VoiceSFUPath},
		{"the public URL", with(sfu(), "MY_VOICE_SFU_PUBLIC_URL", "wss://sfu1.example.org/media"), "wss://sfu1.example.org/media"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg, _, err := Load(source(c.env, nil))
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.VoiceSFUURL(); got != c.want {
				t.Errorf("VoiceSFUURL() = %q, want %q", got, c.want)
			}
		})
	}
	if VoiceSFUPath != "/voice/sfu" {
		t.Errorf("VoiceSFUPath = %q, want /voice/sfu", VoiceSFUPath)
	}
}

// TestRoleSFU checks what each role runs (ADR-0101 §10): an SFU node serves no API and runs no
// jobs.
func TestRoleSFU(t *testing.T) {
	for _, c := range []struct {
		role      Role
		api, jobs bool
	}{
		{RoleAll, true, true},
		{RoleWeb, true, false},
		{RoleWorker, false, true},
		{RoleSFU, false, false},
	} {
		if c.role.ServesAPI() != c.api || c.role.RunsJobs() != c.jobs {
			t.Errorf("%s: ServesAPI %v, RunsJobs %v; want %v, %v", c.role, c.role.ServesAPI(), c.role.RunsJobs(), c.api, c.jobs)
		}
	}
	if RoleSFU != "sfu" {
		t.Errorf("RoleSFU = %q, want sfu", RoleSFU)
	}
}

// TestLoadVoiceSFUInvalid checks the start checks of voice = sfu, role = sfu and the room's
// maximum (ADR-0101 §1, §10, §15): each refuses the start and names what to set.
func TestLoadVoiceSFUInvalid(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		// want holds, for every issue in order, the parts its message must contain.
		want [][]string
	}{
		{
			name: "relay with sfu",
			env: with(sfu(), "MY_VOICE_ICE_TRANSPORT_POLICY", "relay", "MY_VOICE_TURN", "coturn",
				"MY_VOICE_TURN_COTURN_URLS", "turns:turn.example.org:5349", "MY_VOICE_TURN_COTURN_SECRET", "s3cret"),
			want: [][]string{{"voice_ice_transport_policy (MY_VOICE_ICE_TRANSPORT_POLICY): ", "relay", "voice = sfu"}},
		},
		{
			name: "role sfu with p2p",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_ROLE", "sfu"),
			want: [][]string{{"role (MY_ROLE): ", "voice = sfu"}},
		},
		{
			name: "role sfu with voice off",
			env:  with(minimal(), "MY_ROLE", "sfu"),
			want: [][]string{{"role (MY_ROLE): ", "voice = sfu"}},
		},
		{
			name: "coturn without URLs and secret with sfu",
			env:  with(sfu(), "MY_VOICE_TURN", "coturn"),
			want: [][]string{
				{"voice_turn_coturn_urls (MY_VOICE_TURN_COTURN_URLS): required when voice_turn is coturn"},
				{"voice_turn_coturn_secret (MY_VOICE_TURN_COTURN_SECRET): required when voice_turn is coturn"},
			},
		},
		{
			name: "cloudflare without key and token with sfu",
			env:  with(sfu(), "MY_VOICE_TURN", "cloudflare"),
			want: [][]string{
				{"voice_turn_cloudflare_key_id (MY_VOICE_TURN_CLOUDFLARE_KEY_ID): required when voice_turn is cloudflare"},
				{"voice_turn_cloudflare_api_token (MY_VOICE_TURN_CLOUDFLARE_API_TOKEN): required when voice_turn is cloudflare"},
			},
		},
		{
			name: "cloudflare TTL over 48h with sfu",
			env: with(sfu(), "MY_VOICE_TURN", "cloudflare", "MY_VOICE_TURN_CLOUDFLARE_KEY_ID", "key",
				"MY_VOICE_TURN_CLOUDFLARE_API_TOKEN", "token", "MY_VOICE_TURN_CREDENTIAL_TTL", "49h"),
			want: [][]string{{"voice_turn_credential_ttl (MY_VOICE_TURN_CREDENTIAL_TTL): must be at most 48h with voice_turn = cloudflare"}},
		},
		{
			name: "a maximum below 2",
			env:  with(sfu(), "MY_VOICE_MAX_PARTICIPANTS", "1"),
			want: [][]string{{"voice_max_participants (MY_VOICE_MAX_PARTICIPANTS): ", "2", "500"}},
		},
		{
			name: "a maximum above 500",
			env:  with(sfu(), "MY_VOICE_MAX_PARTICIPANTS", "501"),
			want: [][]string{{"voice_max_participants (MY_VOICE_MAX_PARTICIPANTS): ", "2", "500"}},
		},
		{
			name: "a maximum above 50 with p2p",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_MAX_PARTICIPANTS", "500"),
			want: [][]string{{"voice_max_participants (MY_VOICE_MAX_PARTICIPANTS): must be at most 50 with voice = p2p"}},
		},
		{
			name: "ports out of range",
			env:  with(sfu(), "MY_VOICE_SFU_UDP_PORT", "0", "MY_VOICE_SFU_TCP_PORT", "65536"),
			want: [][]string{
				{"voice_sfu_udp_port (MY_VOICE_SFU_UDP_PORT): ", "1", "65535"},
				{"voice_sfu_tcp_port (MY_VOICE_SFU_TCP_PORT): ", "1", "65535"},
			},
		},
		{
			name: "a public IP that is a host name",
			env:  with(sfu(), "MY_VOICE_SFU_PUBLIC_IP", "sfu.example.org"),
			want: [][]string{{"voice_sfu_public_ip (MY_VOICE_SFU_PUBLIC_IP): ", "IP"}},
		},
		{
			name: "a node capacity of 0",
			env:  with(sfu(), "MY_VOICE_SFU_NODE_CAPACITY", "0"),
			want: [][]string{{"voice_sfu_node_capacity (MY_VOICE_SFU_NODE_CAPACITY): "}},
		},
		{
			name: "a negative node capacity",
			env:  with(sfu(), "MY_VOICE_SFU_NODE_CAPACITY", "-1"),
			want: [][]string{{"voice_sfu_node_capacity (MY_VOICE_SFU_NODE_CAPACITY): "}},
		},
	}
	for _, url := range []string{
		"https://sfu.example.org/voice/sfu",
		"ws://sfu.example.org/voice/sfu",
		"wss://sfu.example.org/voice/sfu?node=1",
		"sfu.example.org/voice/sfu",
	} {
		tests = append(tests, struct {
			name string
			env  map[string]string
			want [][]string
		}{
			name: "public URL " + url,
			env:  with(sfu(), "MY_VOICE_SFU_PUBLIC_URL", url),
			want: [][]string{{"voice_sfu_public_url (MY_VOICE_SFU_PUBLIC_URL): ", "wss"}},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _, err := Load(source(tt.env, nil))
			if cfg != nil {
				t.Errorf("Load() config = %+v, want nil", cfg)
			}
			got := issueStrings(t, err)
			if len(got) != len(tt.want) {
				t.Fatalf("issues =\n%s\nwant %d, each with %q", strings.Join(got, "\n"), len(tt.want), tt.want)
			}
			for i, parts := range tt.want {
				if !strings.HasPrefix(got[i], parts[0]) {
					t.Errorf("issue %d = %q, want prefix %q", i, got[i], parts[0])
				}
				for _, p := range parts[1:] {
					if !strings.Contains(got[i], p) {
						t.Errorf("issue %d = %q, want it to say %q", i, got[i], p)
					}
				}
			}
		})
	}
}

// TestRetiredVoiceKeys checks that the maximums of each method fail the start and say what to set
// instead (ADR-0101 §1, §16).
func TestRetiredVoiceKeys(t *testing.T) {
	for _, c := range []struct {
		name  string
		env   map[string]string
		files map[string]string
		want  string
	}{
		{
			name: "voice_p2p_max_participants in the environment",
			env:  with(minimal(), "MY_VOICE", "p2p", "MY_VOICE_P2P_MAX_PARTICIPANTS", "10"),
			want: "environment variable MY_VOICE_P2P_MAX_PARTICIPANTS was renamed to MY_VOICE_MAX_PARTICIPANTS",
		},
		{
			name:  "voice_p2p_max_participants in the config file",
			env:   with(minimal(), "MY_VOICE", "p2p"),
			files: map[string]string{"my.toml": "voice_p2p_max_participants = 10\n"},
			want:  `config file my.toml: key "voice_p2p_max_participants" was renamed to "voice_max_participants"`,
		},
		{
			name: "voice_sfu_max_participants in the environment",
			env:  with(sfu(), "MY_VOICE_SFU_MAX_PARTICIPANTS", "100"),
			want: "environment variable MY_VOICE_SFU_MAX_PARTICIPANTS was removed: set voice_max_participants",
		},
		{
			name:  "voice_sfu_max_participants in the config file",
			env:   sfu(),
			files: map[string]string{"my.toml": "voice_sfu_max_participants = 100\n"},
			want:  `config file my.toml: key "voice_sfu_max_participants" was removed: set voice_max_participants`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := envSource(c.env, c.files)
			if c.files != nil {
				src.File = "my.toml"
			}
			cfg, _, err := Load(src)
			if cfg != nil {
				t.Fatalf("Load() config = %+v, want nil", cfg)
			}
			got := issueStrings(t, err)
			if len(got) != 1 || !strings.HasPrefix(got[0], c.want) {
				t.Errorf("issues =\n%s\nwant one starting %q", strings.Join(got, "\n"), c.want)
			}
		})
	}
}
