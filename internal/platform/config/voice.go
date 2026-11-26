// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Bounds of voice_max_participants (ADR-0101 §1): with at most 50 people, a room of p2p links
// at most 1,225 pairs when everyone speaks, and 45 with the default of 10.
const (
	minVoiceMaxParticipants     = 2
	maxVoiceMaxParticipants     = 500
	maxVoiceP2PMaxParticipants  = 50
	defaultVoiceMaxParticipants = 10
	// The limits a room starts with, so that its cost has a bound before anyone sets them.
	defaultVoiceMaxRoomDuration = "4h"
	defaultVoiceMaxOpenRooms    = 10
	// The ports and the capacity of a media node of voice = sfu (ADR-0101 §14, §15).
	defaultVoiceSFUUDPPort      = 7882
	defaultVoiceSFUTCPPort      = 7881
	defaultVoiceSFUNodeCapacity = 500
)

// VoiceSFUPath is the path of the media node's WebSocket on the server's own host, the default
// of voice_sfu_public_url (ADR-0101 §11).
const VoiceSFUPath = "/voice/sfu"

// maxCloudflareTURNTTL is the longest validity of the credentials of Cloudflare's TURN service
// (ADR-0101 §5).
const maxCloudflareTURNTTL = 48 * time.Hour

// voiceChecks rejects the voice settings that cannot connect a room (ADR-0101 §1, ADR-0101 §14),
// and role = sfu without voice = sfu (§10). With voice = off it checks no other voice_* setting:
// they are not used.
func voiceChecks(vals map[string]string, present map[string]bool) []Issue {
	var issues []Issue
	require := func(cond string, keys ...string) {
		for _, k := range keys {
			if !present[k] {
				issues = append(issues, Issue{Key: k, Missing: true, Message: "required when " + cond})
			}
		}
	}
	if vals["role"] == string(RoleSFU) && vals["voice"] != "sfu" {
		issues = append(issues, Issue{Key: "role", Message: "sfu runs a media node of voice = sfu; set voice = sfu, or another role"})
	}
	switch vals["voice"] {
	case "p2p", "sfu":
		switch {
		case vals["voice"] == "sfu" && vals["voice_ice_transport_policy"] == "relay":
			// The browsers of an SFU reach only the node, never each other (ADR-0101 §15).
			issues = append(issues, Issue{Key: "voice_ice_transport_policy", Message: "relay does not work with voice = sfu: the participants already reach only the media node, never each other's addresses, and relaying would only add TURN traffic; set it to all"})
		case vals["voice_ice_transport_policy"] == "relay" && vals["voice_turn"] == "none":
			issues = append(issues, Issue{Key: "voice_ice_transport_policy", Message: "relay needs a TURN server; set voice_turn"})
		}
		if n, err := strconv.Atoi(vals["voice_max_participants"]); err == nil && vals["voice"] == "p2p" && n > maxVoiceP2PMaxParticipants {
			issues = append(issues, Issue{Key: "voice_max_participants", Message: fmt.Sprintf("must be at most %d with voice = p2p, where every speaker sends their audio to every participant; lower it, or use voice = sfu", maxVoiceP2PMaxParticipants)})
		}
		switch vals["voice_turn"] {
		case "coturn":
			require("voice_turn is coturn", "voice_turn_coturn_urls", "voice_turn_coturn_secret")
		case "cloudflare":
			require("voice_turn is cloudflare", "voice_turn_cloudflare_key_id", "voice_turn_cloudflare_api_token")
			if ttl, err := time.ParseDuration(vals["voice_turn_credential_ttl"]); err == nil && ttl > maxCloudflareTURNTTL {
				issues = append(issues, Issue{Key: "voice_turn_credential_ttl", Message: "must be at most 48h with voice_turn = cloudflare"})
			}
		}
	}
	return issues
}

// VoiceWarnings returns the voice settings that work but that the operator likely did not
// mean, for the log at start (ADR-0101 §1, ADR-0101 §12): p2p without STUN and TURN, which
// connects only the browsers on the same network.
func VoiceWarnings(c *Config) []Finding {
	var out []Finding
	warn := func(key, msg string) { out = append(out, Finding{Severity: SeverityWarning, Key: key, Message: msg}) }
	if c.Voice == "p2p" && len(c.VoiceSTUNURLs) == 0 && c.VoiceTURN == "none" {
		warn("voice", "is p2p without voice_stun_urls and voice_turn, so only browsers on the same network connect")
	}
	return out
}

func checkVoiceMaxParticipants(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < minVoiceMaxParticipants || n > maxVoiceMaxParticipants {
		return fmt.Errorf("must be a whole number from %d to %d", minVoiceMaxParticipants, maxVoiceMaxParticipants)
	}
	return nil
}

func checkVoiceSFUPort(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("must be a port number from 1 to 65535")
	}
	return nil
}

func checkIPAddress(v string) error {
	if net.ParseIP(strings.TrimSpace(v)) == nil {
		return errors.New("must be an IPv4 or IPv6 address, such as 203.0.113.7")
	}
	return nil
}

// VoiceSFUURL returns the URL of the media node's WebSocket that the clients connect to
// (ADR-0101 §11): voice_sfu_public_url, or by default VoiceSFUPath on ServerOrigin, over wss for
// https and ws for http.
func (c *Config) VoiceSFUURL() string {
	if c.VoiceSFUPublicURL != "" {
		return c.VoiceSFUPublicURL
	}
	origin := c.ServerOrigin()
	if rest, ok := strings.CutPrefix(origin, "http://"); ok {
		return "ws://" + rest + VoiceSFUPath
	}
	return "wss://" + strings.TrimPrefix(origin, "https://") + VoiceSFUPath
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

// checkVoiceSFUOrigins accepts a comma-separated list of wss: origins (ws: on a loopback host),
// each without a path, whose host may start with "*." (ADR-0101 §11).
func checkVoiceSFUOrigins(v string) error {
	for _, o := range splitList(v) {
		host := strings.TrimPrefix(strings.TrimPrefix(o, "wss://"), "ws://")
		wildcard := strings.HasPrefix(host, "*.")
		probe := o
		if wildcard {
			probe = strings.Replace(o, "*.", "x.", 1)
		}
		if err := checkVoiceServerURL(probe, "wss", "ws"); err != nil {
			return fmt.Errorf("%s: %w", o, err)
		}
		if u, _ := url.Parse(probe); u.Path != "" {
			return fmt.Errorf("%s: must be an origin, with no path", o)
		}
	}
	return nil
}

// VoiceSFUConnectSources returns the origins that the web client's connect-src lets it reach for
// the media nodes of voice = sfu: none with another method, and otherwise voice_sfu_origins and
// the origin of this process's own node URL.
func (c *Config) VoiceSFUConnectSources() []string {
	if c.Voice != "sfu" {
		return nil
	}
	sources := append([]string(nil), c.VoiceSFUOrigins...)
	if u, err := url.Parse(c.VoiceSFUURL()); err == nil && u.Host != "" {
		sources = append(sources, u.Scheme+"://"+u.Host)
	}
	return sources
}
