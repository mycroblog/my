// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// TestLoadSearchElasticsearch checks the settings of search = elasticsearch.
func TestLoadSearchElasticsearch(t *testing.T) {
	base := func(kv ...string) map[string]string {
		return with(with(minimal(), "MY_SEARCH", "elasticsearch", "MY_SEARCH_ELASTICSEARCH_URL", "https://es.internal.example:9200"), kv...)
	}
	t.Run("a URL and defaults", func(t *testing.T) {
		cfg, _, err := Load(source(base("MY_SEARCH_ELASTICSEARCH_API_KEY", "a2V5OnNlY3JldA=="), nil))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Search != "elasticsearch" || cfg.SearchElasticsearchURL != "https://es.internal.example:9200" || cfg.SearchElasticsearchAPIKey != "a2V5OnNlY3JldA==" ||
			cfg.SearchElasticsearchIndex != "" || cfg.SearchElasticsearchIndexSettings != nil {
			t.Errorf("cfg = %+v", cfg)
		}
	})
	t.Run("a user and password over http", func(t *testing.T) {
		cfg, _, err := Load(source(base("MY_SEARCH_ELASTICSEARCH_URL", "http://elasticsearch:9200", "MY_SEARCH_ELASTICSEARCH_USERNAME", "my",
			"MY_SEARCH_ELASTICSEARCH_PASSWORD_FILE", "/run/secrets/es", "MY_SEARCH_ELASTICSEARCH_INDEX", "social_statuses", "MY_SEARCH_ELASTICSEARCH_INDEX_SETTINGS_FILE", "/etc/my/es.json"),
			map[string]string{"/run/secrets/es": "hunter2\n", "/etc/my/es.json": `{"number_of_replicas": 0, "analysis": {"analyzer": {"my_text": {"type": "standard"}}}}`}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.SearchElasticsearchUsername != "my" || cfg.SearchElasticsearchPassword != "hunter2" || cfg.SearchElasticsearchIndex != "social_statuses" || !strings.Contains(string(cfg.SearchElasticsearchIndexSettings), "my_text") {
			t.Errorf("cfg = %+v", cfg)
		}
		if s := cfg.LogValue().String(); strings.Contains(s, "hunter2") || strings.Contains(s, "number_of_replicas") {
			t.Errorf("the log value %s holds the password or the index settings", s)
		}
	})
	t.Run("another search checks nothing of them", func(t *testing.T) {
		if _, _, err := Load(source(with(minimal(), "MY_SEARCH_ELASTICSEARCH_PASSWORD", "p", "MY_SEARCH_ELASTICSEARCH_INDEX_SETTINGS_FILE", "/nowhere.json"), nil)); err != nil {
			t.Error(err)
		}
	})
	t.Run("a settings file that is not one JSON object", func(t *testing.T) {
		for _, content := range []string{"[]", "null", "{", `{"a":1}{"b":2}`, "number_of_replicas: 0"} {
			_, _, err := Load(source(base("MY_SEARCH_ELASTICSEARCH_INDEX_SETTINGS_FILE", "/etc/my/es.json"), map[string]string{"/etc/my/es.json": content}))
			var ve *ValidationError
			if !errors.As(err, &ve) || !slices.ContainsFunc(ve.Issues, func(is Issue) bool { return is.Key == "search_elasticsearch_index_settings_file" }) {
				t.Errorf("Load with the settings %q: %v, want an issue of the file", content, err)
			} else if strings.Contains(err.Error(), "number_of_replicas") {
				t.Errorf("the error %v quotes the file", err)
			}
		}
	})
	for _, tc := range []struct {
		name string
		env  map[string]string
		key  string
	}{
		{"a URL is required", with(minimal(), "MY_SEARCH", "elasticsearch"), "search_elasticsearch_url"},
		{"a URL with credentials", base("MY_SEARCH_ELASTICSEARCH_URL", "https://u:p@es.example"), "search_elasticsearch_url"},
		{"a URL with a query", base("MY_SEARCH_ELASTICSEARCH_URL", "https://es.example?x=1"), "search_elasticsearch_url"},
		{"a URL of another scheme", base("MY_SEARCH_ELASTICSEARCH_URL", "es.example:9200"), "search_elasticsearch_url"},
		{"an API key and a user", base("MY_SEARCH_ELASTICSEARCH_API_KEY", "k", "MY_SEARCH_ELASTICSEARCH_USERNAME", "u", "MY_SEARCH_ELASTICSEARCH_PASSWORD", "p"), "search_elasticsearch_api_key"},
		{"a user without a password", base("MY_SEARCH_ELASTICSEARCH_USERNAME", "u"), "search_elasticsearch_password"},
		{"a password without a user", base("MY_SEARCH_ELASTICSEARCH_PASSWORD", "p"), "search_elasticsearch_username"},
		{"an index name in capitals", base("MY_SEARCH_ELASTICSEARCH_INDEX", "Statuses"), "search_elasticsearch_index"},
		{"a missing settings file", base("MY_SEARCH_ELASTICSEARCH_INDEX_SETTINGS_FILE", "/nowhere.json"), "search_elasticsearch_index_settings_file"},
		{"a CA file over http", base("MY_SEARCH_ELASTICSEARCH_URL", "http://es:9200", "MY_SEARCH_ELASTICSEARCH_CA_FILE", "/ca.pem"), "search_elasticsearch_ca_file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Load(source(tc.env, nil))
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Load: %v, want a ValidationError", err)
			}
			if !slices.ContainsFunc(ve.Issues, func(is Issue) bool { return is.Key == tc.key }) {
				t.Errorf("Load: %v, want an issue of %s", err, tc.key)
			}
		})
	}
}
