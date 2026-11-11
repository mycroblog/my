// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// settingsJSONPath is the list of settings that the OpenTofu modules check their settings and secrets inputs against,
// relative to the repository root.
const settingsJSONPath = "deploy/opentofu/modules/internal/settings/settings.json"

// settingEntry is what the OpenTofu modules know of one setting.
type settingEntry struct {
	Env    string `json:"env"`
	Secret bool   `json:"secret"`
}

// settingsJSON returns every setting by its key, with its environment variable and whether it is a secret.
func settingsJSON() string {
	entries := make(map[string]settingEntry, len(schema))
	for i := range schema {
		entries[schema[i].key] = settingEntry{Env: schema[i].env(), Secret: schema[i].secret}
	}
	b, err := json.MarshalIndent(entries, "", "  ") // encoding/json sorts the keys of a map
	if err != nil {
		panic(err)
	}
	return string(b) + "\n"
}

// TestSettingsJSONUpToDate fails when the committed list differs from the schema.
// Regenerate it with `scripts/task.sh go:config-docs`.
func TestSettingsJSONUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", "..", filepath.FromSlash(settingsJSONPath))
	want := settingsJSON()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path) //nolint:gosec // fixed path inside the repository
	if err != nil {
		t.Fatalf("%v; run `scripts/task.sh go:config-docs`", err)
	}
	if string(got) != want {
		t.Errorf("%s is stale; run `scripts/task.sh go:config-docs`", settingsJSONPath)
	}
}
