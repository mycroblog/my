// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_opensearch

package config

import (
	"errors"
	"strings"
	"testing"
)

func TestSearchOpenSearch(t *testing.T) {
	const (
		settings    = "/etc/my/settings.json"
		notObject   = "/etc/my/array.json"
		withMapping = "/etc/my/mapping.json"
		missing     = "/etc/my/missing.json"
	)
	files := map[string]string{
		settings:    `{"analysis": {"analyzer": {"my_text": {"type": "standard"}, "my_text_search": {"type": "standard"}}}}`,
		notObject:   `["secret-looking content"]`,
		withMapping: `{"mappings": {}}`,
	}
	opensearch := func(kv ...string) map[string]string {
		return with(with(minimal(), "MY_SEARCH", "opensearch", "MY_SEARCH_OPENSEARCH_URL", "https://search.example.org:9200"), kv...)
	}
	t.Run("accepts a cluster", func(t *testing.T) {
		for _, env := range []map[string]string{
			opensearch(),
			opensearch("MY_SEARCH_OPENSEARCH_URL", "http://opensearch:9200/prefix", "MY_SEARCH_OPENSEARCH_USERNAME", "my", "MY_SEARCH_OPENSEARCH_PASSWORD", "pw"),
			opensearch("MY_SEARCH_OPENSEARCH_AWS_SIGNING", "es", "MY_SEARCH_OPENSEARCH_AWS_REGION", "ap-northeast-1", "MY_SEARCH_OPENSEARCH_INDEX", "social_2"),
			opensearch("MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE", settings),
			// Another search does not read the file.
			with(minimal(), "MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE", missing),
		} {
			cfg, _, err := Load(source(env, files))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.SearchOpenSearchURL == "" && cfg.Search == "opensearch" {
				t.Errorf("config %+v", cfg)
			}
			if cfg.SearchOpenSearchIndexSettingsFile == settings && string(cfg.SearchOpenSearchIndexSettings) != files[settings] {
				t.Errorf("SearchOpenSearchIndexSettings = %q, want the file", cfg.SearchOpenSearchIndexSettings)
			}
		}
	})
	for _, tt := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"no URL", with(minimal(), "MY_SEARCH", "opensearch"), []string{"search_opensearch_url (MY_SEARCH_OPENSEARCH_URL): required when search is opensearch"}},
		{"a URL with credentials", opensearch("MY_SEARCH_OPENSEARCH_URL", "https://admin:pw@search.example.org"), []string{"search_opensearch_url (MY_SEARCH_OPENSEARCH_URL): must be an https or http URL"}},
		{"a username without a password", opensearch("MY_SEARCH_OPENSEARCH_USERNAME", "my"), []string{"search_opensearch_password (MY_SEARCH_OPENSEARCH_PASSWORD): required when search_opensearch_username is set"}},
		{"a password without a username", opensearch("MY_SEARCH_OPENSEARCH_PASSWORD", "pw"), []string{"search_opensearch_username (MY_SEARCH_OPENSEARCH_USERNAME): required when search_opensearch_password is set"}},
		{"signing and a username", opensearch("MY_SEARCH_OPENSEARCH_AWS_SIGNING", "es", "MY_SEARCH_OPENSEARCH_USERNAME", "my", "MY_SEARCH_OPENSEARCH_PASSWORD", "pw"), []string{"search_opensearch_username (MY_SEARCH_OPENSEARCH_USERNAME): must be empty with search_opensearch_aws_signing = es"}},
		{"Serverless", opensearch("MY_SEARCH_OPENSEARCH_AWS_SIGNING", "aoss"), []string{`search_opensearch_aws_signing (MY_SEARCH_OPENSEARCH_AWS_SIGNING): unknown value "aoss"`}},
		{"a settings file that is not there", opensearch("MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE", missing), []string{"search_opensearch_index_settings_file (MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE): cannot read"}},
		{"a settings file that is not an object", opensearch("MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE", notObject), []string{"search_opensearch_index_settings_file (MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE): /etc/my/array.json: must hold one JSON object"}},
		{"a settings file with a mapping", opensearch("MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE", withMapping), []string{"search_opensearch_index_settings_file (MY_SEARCH_OPENSEARCH_INDEX_SETTINGS_FILE): /etc/my/mapping.json: must hold the settings only"}},
		{"an index name of capitals", opensearch("MY_SEARCH_OPENSEARCH_INDEX", "My"), []string{"search_opensearch_index (MY_SEARCH_OPENSEARCH_INDEX): must be 1 to 200 lowercase letters"}},
		{"an index name with a leading underscore", opensearch("MY_SEARCH_OPENSEARCH_INDEX", "_my"), []string{"search_opensearch_index (MY_SEARCH_OPENSEARCH_INDEX): must be 1 to 200 lowercase letters"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Load(source(tt.env, files))
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
				if strings.Contains(got[i], "secret-looking") {
					t.Errorf("issue %d quotes the file: %q", i, got[i])
				}
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("issue %d = %q, want prefix %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
