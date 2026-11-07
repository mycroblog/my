// SPDX-License-Identifier: AGPL-3.0-only

package config

// searchMeilisearchChecks rejects the settings of search = meilisearch that cannot reach the
// engine (ADR-0149 §14). With another search it checks nothing: the search_meilisearch_* settings
// are not used.
func searchMeilisearchChecks(vals map[string]string, present map[string]bool) []Issue {
	if vals["search"] != "meilisearch" {
		return nil
	}
	var issues []Issue
	if !present["search_meilisearch_url"] {
		issues = append(issues, Issue{Key: "search_meilisearch_url", Missing: true, Message: "required when search is meilisearch"})
	}
	if path := vals["search_meilisearch_settings_file"]; path != "" {
		// The error says what is wrong with the file, never what it holds.
		if err := checkMeilisearchSettingsFile(path); err != nil {
			issues = append(issues, Issue{Key: "search_meilisearch_settings_file", Message: err.Error()})
		}
	}
	return issues
}
