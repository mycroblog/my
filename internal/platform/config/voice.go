// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Bounds of voice_p2p_max_participants (ADR-0101 §1): with at most 50 people, a room of p2p
// links at most 1,225 pairs when everyone speaks, and 45 with the default of 10.
const (
	minVoiceP2PMaxParticipants     = 2
	maxVoiceP2PMaxParticipants     = 50
	defaultVoiceP2PMaxParticipants = 10
)

// maxCloudflareTURNTTL is the longest validity of the credentials of Cloudflare's TURN service
// (ADR-0101 §5).
const maxCloudflareTURNTTL = 48 * time.Hour

// minVoiceJoinSecretBytes bounds the secret that signs the join values of voice = sora from
// below, the size of the HMAC-SHA256 key it keys.
const minVoiceJoinSecretBytes = 32

// voiceChecks rejects the voice settings that cannot connect a room (ADR-0101 §1, ADR-0102 §5).
// With voice = off it checks nothing: the other voice_* settings are not used.
func voiceChecks(vals map[string]string, present map[string]bool) []Issue {
	var issues []Issue
	require := func(cond string, keys ...string) {
		for _, k := range keys {
			if !present[k] {
				issues = append(issues, Issue{Key: k, Missing: true, Message: "required when " + cond})
			}
		}
	}
	switch vals["voice"] {
	case "p2p":
		if vals["voice_ice_transport_policy"] == "relay" && vals["voice_turn"] == "none" {
			issues = append(issues, Issue{Key: "voice_ice_transport_policy", Message: "relay needs a TURN server; set voice_turn"})
		}
		switch vals["voice_turn"] {
		case "coturn":
			require("voice_turn is coturn", "voice_turn_urls", "voice_turn_secret")
		case "cloudflare":
			require("voice_turn is cloudflare", "voice_turn_cloudflare_key_id", "voice_turn_cloudflare_api_token")
			if ttl, err := time.ParseDuration(vals["voice_turn_credential_ttl"]); err == nil && ttl > maxCloudflareTURNTTL {
				issues = append(issues, Issue{Key: "voice_turn_credential_ttl", Message: "must be at most 48h with voice_turn = cloudflare"})
			}
		}
	case "livekit":
		require("voice is livekit", "voice_livekit_url", "voice_livekit_api_key", "voice_livekit_api_secret")
	case "sora":
		require("voice is sora", "voice_sora_mode", "voice_sora_signaling_urls", "voice_join_secret")
		switch vals["voice_sora_mode"] {
		case "self_hosted":
			// The API of a self-hosted Sora has no authentication of its own, and its webhooks
			// are accepted only with Basic authentication (ADR-0102 §5).
			require("voice_sora_mode is self_hosted", "voice_sora_api_url", "voice_sora_webhook_username", "voice_sora_webhook_password")
		case "cloud":
			require("voice_sora_mode is cloud", "voice_sora_cloud_project_id", "voice_sora_cloud_api_key")
		}
		if present["voice_sora_api_header_name"] {
			require("voice_sora_api_header_name is set", "voice_sora_api_header_value")
		}
		if present["voice_sora_api_header_value"] {
			require("voice_sora_api_header_value is set", "voice_sora_api_header_name")
		}
	}
	return issues
}

// VoiceWarnings returns the voice settings that work but that the operator likely did not
// mean, for the log at start (ADR-0101 §1, ADR-0102 §3): p2p without STUN and TURN, which
// connects only the browsers on the same network, and the STUN and TURN settings that an SFU
// does not use.
func VoiceWarnings(c *Config) []Finding {
	var out []Finding
	warn := func(key, msg string) { out = append(out, Finding{Severity: SeverityWarning, Key: key, Message: msg}) }
	switch c.Voice {
	case "p2p":
		if len(c.VoiceSTUNURLs) == 0 && c.VoiceTURN == "none" {
			warn("voice", "is p2p without voice_stun_urls and voice_turn, so only browsers on the same network connect")
		}
	case "livekit", "sora":
		if len(c.VoiceSTUNURLs) > 0 {
			warn("voice_stun_urls", "is not used with voice = "+c.Voice+", whose own STUN and TURN servers serve the browsers")
		}
		if c.VoiceTURN != "" && c.VoiceTURN != "none" {
			warn("voice_turn", "is not used with voice = "+c.Voice+", whose own STUN and TURN servers serve the browsers")
		}
		if c.VoiceICETransportPolicy == "relay" {
			warn("voice_ice_transport_policy", "is not used with voice = "+c.Voice+", whose own STUN and TURN servers serve the browsers")
		}
	}
	return out
}

// voiceAPIURL returns the default of voice_livekit_api_url: voice_livekit_url with https for
// wss and http for ws.
func voiceAPIURL(livekitURL string) string {
	switch {
	case strings.HasPrefix(livekitURL, "wss://"):
		return "https://" + strings.TrimPrefix(livekitURL, "wss://")
	case strings.HasPrefix(livekitURL, "ws://"):
		return "http://" + strings.TrimPrefix(livekitURL, "ws://")
	}
	return ""
}

func checkVoiceP2PMaxParticipants(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < minVoiceP2PMaxParticipants || n > maxVoiceP2PMaxParticipants {
		return fmt.Errorf("must be a whole number from %d to %d", minVoiceP2PMaxParticipants, maxVoiceP2PMaxParticipants)
	}
	return nil
}

func checkNonNegativeInt(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 || n > 1<<31-1 {
		return fmt.Errorf("must be a whole number from 0 to %d", 1<<31-1)
	}
	return nil
}

func checkNonNegativeDuration(v string) error {
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return errors.New("must be 0 or a positive Go duration, for example 4h or 90m")
	}
	return nil
}

// checkVoiceURLs returns the check of a comma-separated list of the URIs of STUN or TURN servers
// (RFC 7064, RFC 7065) of the given schemes, such as stun:host:3478 or turn:host?transport=tcp.
func checkVoiceURLs(schemes ...string) func(string) error {
	want := strings.Join(schemes, ": or ") + ":"
	return func(v string) error {
		list := splitList(v)
		if len(list) == 0 {
			return fmt.Errorf("must be comma-separated %s URLs", want)
		}
		for _, s := range list {
			u, err := url.Parse(s)
			if err != nil || !slices.Contains(schemes, u.Scheme) || u.Opaque == "" || strings.ContainsAny(s, " \t") {
				return fmt.Errorf("must be comma-separated %s URLs, such as %s:host.example.org:3478; got %q", want, schemes[0], s)
			}
		}
		return nil
	}
}

// checkVoiceWebSocketURL accepts a wss URL, or a ws one on a loopback host.
func checkVoiceWebSocketURL(v string) error {
	return checkVoiceServerURL(v, "wss", "ws")
}

func checkVoiceWebSocketURLs(v string) error {
	list := splitList(v)
	if len(list) == 0 {
		return errors.New("must be comma-separated wss URLs")
	}
	for _, s := range list {
		if err := checkVoiceWebSocketURL(s); err != nil {
			return err
		}
	}
	return nil
}

// checkVoiceHTTPURL accepts an https URL, or an http one on a loopback host.
func checkVoiceHTTPURL(v string) error {
	return checkVoiceServerURL(v, "https", "http")
}

// checkVoiceServerURL accepts an absolute URL of the scheme secure, or of insecure on a loopback
// host, without credentials, query, or fragment.
func checkVoiceServerURL(v, secure, insecure string) error {
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != secure && u.Scheme != insecure) {
		return fmt.Errorf("must be a %s URL with a host and no credentials, query, or fragment", secure)
	}
	if u.Scheme == insecure && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("must be %s; %s is accepted only for localhost, 127.0.0.1, or [::1]", secure, insecure)
	}
	return nil
}

// checkHeaderName accepts an HTTP field name (RFC 9110 §5.1).
func checkHeaderName(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return errors.New("must be an HTTP header name")
	}
	if strings.ContainsFunc(v, func(r rune) bool { return !isTokenChar(r) }) {
		return errors.New("must be an HTTP header name, such as Authorization")
	}
	return nil
}

// isTokenChar reports whether r may be in an HTTP token (RFC 9110 §5.6.2).
func isTokenChar(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)
}

func checkVoiceJoinSecret(v string) error {
	if len(v) < minVoiceJoinSecretBytes {
		return fmt.Errorf("must be at least %d bytes; generate one with openssl rand -base64 32", minVoiceJoinSecretBytes)
	}
	return nil
}
