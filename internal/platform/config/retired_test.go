// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

// withRetired replaces retiredKeys for the test with a renamed and a removed key.
func withRetired(t *testing.T) {
	t.Helper()
	saved := retiredKeys
	retiredKeys = []retiredKey{
		{key: "old_host", renamedTo: "host"},
		{key: "old_secret", renamedTo: "kek"},
		{key: "gone", removed: "the server now always does this"},
	}
	t.Cleanup(func() { retiredKeys = saved })
}

// envSource is source with EnvNames listing the keys of env.
func envSource(env, files map[string]string) Source {
	src := source(env, files)
	src.EnvNames = func() []string { return slices.Collect(maps.Keys(env)) }
	return src
}

func issueStrings(t *testing.T, err error) []string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want *ValidationError", err)
	}
	got := make([]string, len(ve.Issues))
	for i, is := range ve.Issues {
		got[i] = is.String()
	}
	return got
}

func TestRetiredKeys(t *testing.T) {
	withRetired(t)
	tests := []struct {
		name  string
		env   map[string]string
		files map[string]string
		want  []string
	}{
		{
			name:  "renamed key in the config file",
			env:   minimal(),
			files: map[string]string{"my.toml": `old_host = "social.example.org"`},
			want:  []string{`config file my.toml: key "old_host" was renamed to "host"`},
		},
		{
			name: "renamed key in the environment",
			env:  with(minimal(), "MY_OLD_HOST", "social.example.org"),
			want: []string{"environment variable MY_OLD_HOST was renamed to MY_HOST"},
		},
		{
			name: "renamed secret through its file",
			env:  with(minimal(), "MY_OLD_SECRET_FILE", "/run/secrets/kek"),
			want: []string{"environment variable MY_OLD_SECRET_FILE was renamed to MY_KEK"},
		},
		{
			name:  "removed key in both",
			env:   with(minimal(), "MY_GONE", "1"),
			files: map[string]string{"my.toml": `gone = "1"`},
			want: []string{
				`config file my.toml: key "gone" was removed: the server now always does this`,
				"environment variable MY_GONE was removed: the server now always does this",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := envSource(tt.env, tt.files)
			if tt.files != nil {
				src.File = "my.toml"
			}
			for name, load := range map[string]func(Source) (*Config, []Setting, error){"Load": Load, "LoadMaintenance": LoadMaintenance} {
				cfg, _, err := load(src)
				if cfg != nil {
					t.Fatalf("%s() config = %+v, want nil", name, cfg)
				}
				if got := issueStrings(t, err); !slices.Equal(got, tt.want) {
					t.Errorf("%s() issues =\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
				}
			}
			if _, err := MaintenanceRequested(src); err == nil {
				t.Error("MaintenanceRequested() error = nil, want the retired key")
			}
		})
	}
}

func TestRetiredKeysWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range retiredKeys {
		if seen[r.key] {
			t.Errorf("retired key %q is listed twice", r.key)
		}
		seen[r.key] = true
		if slices.Contains(Keys(), r.key) {
			t.Errorf("retired key %q is still a setting", r.key)
		}
		switch {
		case r.renamedTo != "" && r.removed != "":
			t.Errorf("retired key %q is both renamed and removed", r.key)
		case r.renamedTo != "" && !slices.Contains(Keys(), r.renamedTo):
			t.Errorf("retired key %q is renamed to %q, which is not a setting", r.key, r.renamedTo)
		case r.renamedTo == "" && r.removed == "":
			t.Errorf("retired key %q tells neither its new key nor why it was removed", r.key)
		}
	}
}

func TestUnknownEnv(t *testing.T) {
	withRetired(t)
	env := with(minimal(),
		"MY_HSOT", "typo.example", // a typo
		"MY_HOST_FILE", "/x", // _FILE of a setting that is not a secret
		"MY_KEK_FILE", "/run/secrets/kek",
		"MY_CONFIG", "my.toml",
		"MY_OLD_HOST", "x", // retired: an error of Load, not a warning
		// Kubernetes service links of Services named "my" and "my-db".
		"MY_SERVICE_HOST", "10.0.0.1",
		"MY_SERVICE_PORT", "8080",
		"MY_SERVICE_PORT_HTTP", "8080",
		"MY_PORT", "tcp://10.0.0.1:8080",
		"MY_PORT_8080_TCP", "tcp://10.0.0.1:8080",
		"MY_PORT_8080_TCP_PROTO", "tcp",
		"MY_PORT_8080_TCP_PORT", "8080",
		"MY_PORT_8080_TCP_ADDR", "10.0.0.1",
		"MY_DB_SERVICE_HOST", "10.0.0.2",
		"MY_DB_PORT_5432_TCP", "tcp://10.0.0.2:5432",
		"PATH", "/bin",
		"HOME", "/home/my",
	)
	got := UnknownEnv(envSource(env, nil))
	want := []string{"MY_HOST_FILE", "MY_HSOT"}
	if !slices.Equal(got, want) {
		t.Errorf("UnknownEnv() = %q, want %q", got, want)
	}
	if _, _, err := Load(envSource(with(minimal(), "MY_HSOT", "typo.example"), nil)); err != nil {
		t.Errorf("Load() with an unknown variable = %v, want nil: it is only a warning", err)
	}
}

func TestEnvNamesDefault(t *testing.T) {
	t.Setenv("MY_TEST_UNKNOWN_ENV", "1")
	if got := UnknownEnv(Source{}); !slices.Contains(got, "MY_TEST_UNKNOWN_ENV") {
		t.Errorf("UnknownEnv(Source{}) = %q, want the process environment's MY_TEST_UNKNOWN_ENV", got)
	}
	// A Source with its own LookupEnv lists no variable of the process unless it sets EnvNames.
	if got := UnknownEnv(source(map[string]string{}, nil)); len(got) != 0 {
		t.Errorf("UnknownEnv() with LookupEnv and no EnvNames = %q, want none", got)
	}
}

func TestReferenceListsRetiredKeys(t *testing.T) {
	withRetired(t)
	ref := Reference()
	for _, want := range []string{
		"| `old_host` | `MY_OLD_HOST` | [`host`](#host) |",
		"| `gone` | `MY_GONE` | the server now always does this |",
	} {
		if !strings.Contains(ref, want) {
			t.Errorf("Reference() lacks %q", want)
		}
	}
}
