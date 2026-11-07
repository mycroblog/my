// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_meilisearch

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMeilisearch(t *testing.T) {
	cfg, _, err := Load(source(with(minimal(), "MY_SEARCH", "meilisearch", "MY_SEARCH_MEILISEARCH_URL", "http://meilisearch:7700",
		"MY_SEARCH_MEILISEARCH_API_KEY", " key ", "MY_SEARCH_MEILISEARCH_INDEX", "statuses_a"), nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SearchMeilisearchURL != "http://meilisearch:7700" || cfg.SearchMeilisearchAPIKey != "key" || cfg.SearchMeilisearchIndex != "statuses_a" {
		t.Errorf("Load() = %q %q %q", cfg.SearchMeilisearchURL, cfg.SearchMeilisearchAPIKey, cfg.SearchMeilisearchIndex)
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"no URL", with(minimal(), "MY_SEARCH", "meilisearch"), []string{"search_meilisearch_url (MY_SEARCH_MEILISEARCH_URL): required when search is meilisearch"}},
		{
			"a URL with credentials and a bad index",
			with(minimal(), "MY_SEARCH", "meilisearch", "MY_SEARCH_MEILISEARCH_URL", "https://u:hunter2@meili.example.org", "MY_SEARCH_MEILISEARCH_INDEX", "a b"),
			[]string{"search_meilisearch_url (MY_SEARCH_MEILISEARCH_URL): ", "search_meilisearch_index (MY_SEARCH_MEILISEARCH_INDEX): "},
		},
	} {
		_, _, err := Load(source(tc.env, nil))
		var ve *ValidationError
		if !errors.As(err, &ve) || len(ve.Issues) != len(tc.want) {
			t.Fatalf("%s: Load() = %v, want %d issues", tc.name, err, len(tc.want))
		}
		for i, w := range tc.want {
			if got := ve.Issues[i].String(); !strings.HasPrefix(got, w) || strings.Contains(got, "hunter2") {
				t.Errorf("%s: issue %d = %q, want prefix %q and no password", tc.name, i, got, w)
			}
		}
	}
}

func TestLoadMeilisearchSettingsFile(t *testing.T) {
	dir := t.TempDir()
	file := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	meili := func(path string) map[string]string {
		return with(minimal(), "MY_SEARCH", "meilisearch", "MY_SEARCH_MEILISEARCH_URL", "http://meilisearch:7700",
			"MY_SEARCH_MEILISEARCH_SETTINGS_FILE", path)
	}
	ok := file("ok.json", `{"stopWords": ["the"]}`)
	cfg, _, err := Load(source(meili(ok), nil))
	if err != nil || cfg.SearchMeilisearchSettingsFile != ok {
		t.Fatalf("Load() = %v, %v", cfg, err)
	}
	for name, path := range map[string]string{
		"missing": filepath.Join(dir, "missing.json"),
		"owned":   file("owned.json", `{"sortableAttributes": ["hidden-value"]}`),
		"broken":  file("broken.json", `{"stopWords": hidden-value`),
	} {
		_, _, err := Load(source(meili(path), nil))
		var ve *ValidationError
		if !errors.As(err, &ve) || len(ve.Issues) != 1 || ve.Issues[0].Key != "search_meilisearch_settings_file" {
			t.Errorf("%s: Load() = %v, want one issue of search_meilisearch_settings_file", name, err)
		} else if strings.Contains(err.Error(), "hidden-value") {
			t.Errorf("%s: Load() = %v, which quotes the file", name, err)
		}
	}
	// With another search the file is not read.
	if _, _, err := Load(source(with(minimal(), "MY_SEARCH_MEILISEARCH_SETTINGS_FILE", filepath.Join(dir, "missing.json")), nil)); err != nil {
		t.Errorf("Load() with search = none: %v", err)
	}
}
