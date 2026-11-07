// SPDX-License-Identifier: AGPL-3.0-only

package config

// searchMeilisearchChecks rejects the settings of search = meilisearch that cannot reach the
// engine (ADR-0149 §14). With another search it checks nothing: the search_meilisearch_* settings
// are not used.
func searchMeilisearchChecks(vals map[string]string, present map[string]bool) []Issue {
	if vals["search"] != "meilisearch" {
		return nil
	}
	if !present["search_meilisearch_url"] {
		return []Issue{{Key: "search_meilisearch_url", Missing: true, Message: "required when search is meilisearch"}}
	}
	return nil
}
