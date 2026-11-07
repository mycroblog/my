// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// searchElasticsearchChecks rejects the settings of search = elasticsearch that cannot reach the
// cluster: a missing URL, both kinds of credentials, half of a user and password, and a CA file
// without TLS. With another search it checks nothing: the other search_elasticsearch_* settings
// are not used.
func searchElasticsearchChecks(vals map[string]string, present map[string]bool) []Issue {
	if vals["search"] != "elasticsearch" {
		return nil
	}
	var issues []Issue
	if !present["search_elasticsearch_url"] {
		issues = append(issues, Issue{Key: "search_elasticsearch_url", Missing: true, Message: "required when search is elasticsearch"})
	}
	if present["search_elasticsearch_api_key"] && (present["search_elasticsearch_username"] || present["search_elasticsearch_password"]) {
		issues = append(issues, Issue{Key: "search_elasticsearch_api_key", Message: "set it or search_elasticsearch_username and search_elasticsearch_password, not both"})
	}
	if present["search_elasticsearch_username"] && !present["search_elasticsearch_password"] {
		issues = append(issues, Issue{Key: "search_elasticsearch_password", Missing: true, Message: "required when search_elasticsearch_username is set"})
	}
	if present["search_elasticsearch_password"] && !present["search_elasticsearch_username"] {
		issues = append(issues, Issue{Key: "search_elasticsearch_username", Missing: true, Message: "required when search_elasticsearch_password is set"})
	}
	if present["search_elasticsearch_ca_file"] && present["search_elasticsearch_url"] && !strings.HasPrefix(strings.TrimSpace(vals["search_elasticsearch_url"]), "https://") {
		issues = append(issues, Issue{Key: "search_elasticsearch_ca_file", Message: "needs an https search_elasticsearch_url"})
	}
	return issues
}

// checkSearchURL accepts the absolute http or https URL of a search engine's API, without
// credentials, query, or fragment. The engine is the operator's and often on a private network,
// so http is accepted on any host.
func checkSearchURL(v string) error {
	u, err := url.Parse(strings.TrimSpace(v))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("must be an https or http URL with a host and no credentials, query, or fragment, such as https://search.internal.example:9200")
	}
	return nil
}

// loadSearchElasticsearchSettings reads search_elasticsearch_index_settings_file when search =
// elasticsearch, and keeps it in cfg when it is one JSON object. Its content reaches no log and no
// error: only its path does.
func loadSearchElasticsearchSettings(cfg *Config, src Source) []Issue {
	path := cfg.SearchElasticsearchIndexSettingsFile
	if cfg.Search != "elasticsearch" || path == "" {
		return nil
	}
	const key = "search_elasticsearch_index_settings_file"
	data, err := src.ReadFile(path)
	if err != nil {
		return []Issue{{Key: key, Message: fmt.Sprintf("cannot read it: %v", err)}}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var v map[string]any
	if err := dec.Decode(&v); err != nil || v == nil || dec.More() {
		return []Issue{{Key: key, Message: path + " must hold one JSON object, the settings of an index"}}
	}
	cfg.SearchElasticsearchIndexSettings = data
	return nil
}
