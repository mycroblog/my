// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_search_elasticsearch

package config

func init() { exclude("no_search_elasticsearch", "search", "elasticsearch") }

// checkElasticsearchIndex accepts every value: no adapter of this build reads
// search_elasticsearch_index.
func checkElasticsearchIndex(string) error { return nil }
