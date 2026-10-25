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
// notices the change instead of the setting being silently ignored (ADR-0087 §5).
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
// through (ADR-0087 §4).
var retiredKeys []retiredKey

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

// UnknownEnv returns, sorted, the environment variables of src that start with EnvPrefix but
// set nothing: not a setting, its NAME_FILE when it is a secret, EnvConfigFile, a retired key
// (Load reports those as errors), or a variable that Kubernetes adds for a Service. The server
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
	if name == EnvConfigFile {
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
