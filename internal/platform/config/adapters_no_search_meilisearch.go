// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_search_meilisearch

package config

func init() { exclude("no_search_meilisearch", "search", "meilisearch") }

// checkMeilisearchURL accepts every value: no adapter of this build reads search_meilisearch_url.
func checkMeilisearchURL(string) error { return nil }

// checkMeilisearchIndex accepts every value: no adapter of this build reads
// search_meilisearch_index.
func checkMeilisearchIndex(string) error { return nil }

// checkMeilisearchSettings accepts every file: no adapter of this build reads it.
func checkMeilisearchSettings([]byte) error { return nil }
