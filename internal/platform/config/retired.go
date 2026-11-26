// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// retiredKey is a setting that a release renamed or removed. Using it in the config file or the
// environment is an error that names the new key or the reason, so that an operator who upgrades
// notices the change instead of the setting being silently ignored (ADR-0023 §8).
type retiredKey struct {
	// key is the old TOML key; its environment variable is EnvPrefix + upper(key).
	key string
	// renamedTo is the key that replaces it, or "" when the setting was removed.
	renamedTo string
	// removed tells why the setting was removed and what to do instead, when renamedTo is "".
	removed string
}

// retiredKeys lists the renamed and removed settings, oldest first. Keep an entry for at least
// the minor release that changes the key and the next one, the releases an upgrade passes
// through (ADR-0023 §7). scripts/release-notes.sh reads the literal for the release notes: keep
// its strings literal, or that script fails.
var retiredKeys = []retiredKey{
	{key: "voice_p2p_max_participants", renamedTo: "voice_max_participants"},
	{key: "voice_sfu_max_participants", removed: "set voice_max_participants (2 to 500), the most people a room of any voice method may hold"},
	{key: "voice_livekit_url", removed: "LiveKit is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_livekit_api_url", removed: "LiveKit is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_livekit_api_key", removed: "LiveKit is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_livekit_api_secret", removed: "LiveKit is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_mode", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_signaling_urls", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_api_url", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_api_header_name", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_api_header_value", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_webhook_username", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_webhook_password", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_cloud_project_id", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_cloud_api_key", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
	{key: "voice_sora_join_secret", removed: "Sora is no longer supported; delete it and set voice = sfu, the server's own media nodes, which need the browsers to reach voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) of each node, or voice = p2p"},
}

// retiredValue is a value of an enumerated setting that a release removed. Setting it is an error
// that tells what to set instead, rather than the list of the values that the setting takes.
type retiredValue struct {
	key, value string
	// removed tells why the value was removed and what to set instead.
	removed string
}

// retiredValues lists the removed values of the settings, oldest first, kept as long as a
// retiredKeys entry.
var retiredValues = []retiredValue{
	{key: "voice", value: "livekit", removed: "LiveKit is no longer supported; set voice = sfu, where the server's own media nodes carry the audio: a process with role = all or sfu runs one, which the browsers reach at voice_sfu_public_url over WebSocket and at voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) for the audio; or set voice = p2p. A room still open on LiveKit ends at the first sweep."},
	{key: "voice", value: "sora", removed: "Sora is no longer supported; set voice = sfu, where the server's own media nodes carry the audio: a process with role = all or sfu runs one, which the browsers reach at voice_sfu_public_url over WebSocket and at voice_sfu_udp_port (UDP) and voice_sfu_tcp_port (TCP) for the audio; or set voice = p2p. A room still open on Sora ends at the first sweep."},
}

// retiredValueError reports that key = value was removed, or nil when the value was not.
func retiredValueError(key, value string) error {
	for _, r := range retiredValues {
		if r.key == key && r.value == value {
			return fmt.Errorf("%q was removed: %s", value, r.removed)
		}
	}
	return nil
}

func (r *retiredKey) env() string { return EnvPrefix + strings.ToUpper(r.key) }

// explain tells what replaced the retired setting, naming the new key in the form the operator
// used: name formats a key as a TOML key or as an environment variable.
func (r *retiredKey) explain(name func(key string) string) string {
	if r.renamedTo != "" {
		return "was renamed to " + name(r.renamedTo)
	}
	return "was removed: " + r.removed
}

func tomlKeyName(key string) string { return fmt.Sprintf("%q", key) }

func envName(key string) string { return EnvPrefix + strings.ToUpper(key) }

// retiredIssues reports every retired key used in the config file at path or in env, the
// names of the environment variables.
func retiredIssues(path string, file map[string]any, env []string) []Issue {
	var issues []Issue
	for i := range retiredKeys {
		r := &retiredKeys[i]
		if _, ok := file[r.key]; ok {
			issues = append(issues, Issue{Message: fmt.Sprintf("config file %s: key %q %s", path, r.key, r.explain(tomlKeyName))})
		}
		for _, name := range []string{r.env(), r.env() + "_FILE"} {
			if slices.Contains(env, name) {
				issues = append(issues, Issue{Message: fmt.Sprintf("environment variable %s %s", name, r.explain(envName))})
			}
		}
	}
	return issues
}

// serviceLinkEnv matches the variables that Kubernetes adds to a container for each Service of
// its namespace (enableServiceLinks, on by default): a Service named "my" or "my-..." yields
// MY_SERVICE_HOST, MY_SERVICE_PORT, MY_SERVICE_PORT_<PORT NAME>, MY_PORT, and
// MY_PORT_<PORT>_<PROTOCOL>[_PROTO|_PORT|_ADDR].
var serviceLinkEnv = regexp.MustCompile(`^MY(_[A-Z0-9_]+)?_(SERVICE_HOST|SERVICE_PORT(_[A-Z0-9_]+)?|PORT(_[0-9]+_(TCP|UDP|SCTP)(_(PROTO|PORT|ADDR))?)?)$`)

// composeEnv lists the variables of the env file of the Compose deployment (deploy/docker/example.env)
// that only the compose files read: the file reaches the containers whole.
var composeEnv = []string{EnvPrefix + "VERSION", EnvPrefix + "HTTP_PORT"}

// UnknownEnv returns, sorted, the environment variables of src that start with EnvPrefix but
// set nothing: not a setting, its NAME_FILE when it is a secret, EnvConfigFile, a retired key
// (Load reports those as errors), a variable that Kubernetes adds for a Service, or one of
// composeEnv. The server
// ignores them; the caller warns about them, since a typo in a variable's name leaves the
// setting at its default.
func UnknownEnv(src Source) []string {
	src = withDefaults(src)
	var unknown []string
	for _, name := range src.EnvNames() {
		if !strings.HasPrefix(name, EnvPrefix) || knownEnv(name) || serviceLinkEnv.MatchString(name) {
			continue
		}
		unknown = append(unknown, name)
	}
	slices.Sort(unknown)
	return slices.Compact(unknown)
}

func knownEnv(name string) bool {
	if name == EnvConfigFile || slices.Contains(composeEnv, name) {
		return true
	}
	for i := range schema {
		f := &schema[i]
		if name == f.env() || (f.secret && name == f.env()+"_FILE") {
			return true
		}
	}
	for i := range retiredKeys {
		if name == retiredKeys[i].env() || name == retiredKeys[i].env()+"_FILE" {
			return true
		}
	}
	return false
}
