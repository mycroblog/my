// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"strings"
	"testing"
)

func TestSearchOpenSearch(t *testing.T) {
	opensearch := func(kv ...string) map[string]string {
		return with(with(minimal(), "MY_SEARCH", "opensearch", "MY_SEARCH_OPENSEARCH_URL", "https://search.example.org:9200"), kv...)
	}
	t.Run("accepts a cluster", func(t *testing.T) {
		for _, env := range []map[string]string{
			opensearch(),
			opensearch("MY_SEARCH_OPENSEARCH_URL", "http://opensearch:9200/prefix", "MY_SEARCH_OPENSEARCH_USERNAME", "my", "MY_SEARCH_OPENSEARCH_PASSWORD", "pw"),
			opensearch("MY_SEARCH_OPENSEARCH_AWS_SIGNING", "es", "MY_SEARCH_OPENSEARCH_AWS_REGION", "ap-northeast-1", "MY_SEARCH_OPENSEARCH_INDEX", "social_2"),
		} {
			cfg, _, err := Load(source(env, nil))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.SearchOpenSearchURL == "" {
				t.Errorf("config %+v", cfg)
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
		{"an index name of capitals", opensearch("MY_SEARCH_OPENSEARCH_INDEX", "My"), []string{"search_opensearch_index (MY_SEARCH_OPENSEARCH_INDEX): must be 1 to 200 lowercase letters"}},
		{"an index name with a leading underscore", opensearch("MY_SEARCH_OPENSEARCH_INDEX", "_my"), []string{"search_opensearch_index (MY_SEARCH_OPENSEARCH_INDEX): must be 1 to 200 lowercase letters"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Load(source(tt.env, nil))
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
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("issue %d = %q, want prefix %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
