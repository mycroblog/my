// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_meilisearch

package config

import "github.com/mycroblog/my/internal/adapter/search/meilisearch"

func checkMeilisearchURL(v string) error { return meilisearch.CheckURL(v) }

func checkMeilisearchIndex(v string) error { return meilisearch.CheckIndexUID(v) }

func checkMeilisearchSettings(b []byte) error { return meilisearch.CheckSettings(b) }
