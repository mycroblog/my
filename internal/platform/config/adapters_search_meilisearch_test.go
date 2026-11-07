// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_meilisearch

package config

import (
	"errors"
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
