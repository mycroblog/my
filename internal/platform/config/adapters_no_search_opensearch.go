// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_search_opensearch

package config

func init() { exclude("no_search_opensearch", "search", "opensearch") }
