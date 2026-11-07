// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"errors"
	"net/url"
)

// maxSearchIndexName bounds search_opensearch_index, so that the name of the index with the
// version of its mapping stays within the 255 bytes of an index name.
const maxSearchIndexName = 200

// searchOpenSearchChecks rejects the settings of search = opensearch that cannot reach the
// cluster. With another search it checks nothing.
func searchOpenSearchChecks(vals map[string]string, present map[string]bool) []Issue {
	if vals["search"] != "opensearch" {
		return nil
	}
	var issues []Issue
	if !present["search_opensearch_url"] {
		issues = append(issues, Issue{Key: "search_opensearch_url", Missing: true, Message: "required when search is opensearch"})
	}
	if present["search_opensearch_username"] && !present["search_opensearch_password"] {
		issues = append(issues, Issue{Key: "search_opensearch_password", Missing: true, Message: "required when search_opensearch_username is set"})
	}
	if present["search_opensearch_password"] && !present["search_opensearch_username"] {
		issues = append(issues, Issue{Key: "search_opensearch_username", Missing: true, Message: "required when search_opensearch_password is set"})
	}
	if vals["search_opensearch_aws_signing"] == "es" && present["search_opensearch_username"] {
		issues = append(issues, Issue{Key: "search_opensearch_username", Message: "must be empty with search_opensearch_aws_signing = es: the requests carry the AWS signature instead"})
	}
	return issues
}

// checkSearchEngineURL accepts an http or https URL with a host and no credentials, query, or
// fragment. The engine is on the operator's network, so http is accepted on any host.
func checkSearchEngineURL(v string) error {
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("must be an https or http URL with a host and no credentials, query, or fragment")
	}
	return nil
}

// checkSearchIndexName accepts the name of an index or an alias of OpenSearch that the server
// can add a version to: lowercase letters, digits, - and _, starting with a letter or a digit.
func checkSearchIndexName(v string) error {
	if v == "" || len(v) > maxSearchIndexName || v[0] == '-' || v[0] == '_' {
		return errors.New("must be 1 to 200 lowercase letters, digits, - and _, starting with a letter or a digit")
	}
	for _, r := range v {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return errors.New("must be 1 to 200 lowercase letters, digits, - and _, starting with a letter or a digit")
		}
	}
	return nil
}
