// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"encoding/json"
	"errors"
	"os"
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
	if path := vals["search_opensearch_index_settings_file"]; path != "" {
		if msg := checkSearchSettingsFile(path); msg != "" {
			issues = append(issues, Issue{Key: "search_opensearch_index_settings_file", Message: msg})
		}
	}
	if vals["search_opensearch_aws_signing"] == "es" && present["search_opensearch_username"] {
		issues = append(issues, Issue{Key: "search_opensearch_username", Message: "must be empty with search_opensearch_aws_signing = es: the requests carry the AWS signature instead"})
	}
	return issues
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

// checkSearchSettingsFile returns why the file at path cannot be the settings of the index, empty
// when it can: it must be a JSON object, and hold no mapping, which the server owns. The message
// never quotes the file, which only the operator should read.
func checkSearchSettingsFile(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // the operator chose the path
	if err != nil {
		return "cannot be read: " + pathError(err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(b, &settings); err != nil || settings == nil {
		return "must hold a JSON object, the settings of the index"
	}
	if _, ok := settings["mappings"]; ok {
		return "must hold the settings only, without mappings: the server owns the mapping"
	}
	return ""
}

// pathError says why a file could not be read, without the bytes of the file.
func pathError(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
