// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_elasticsearch

package config

import (
	"strings"

	"github.com/mycroblog/my/internal/adapter/search/elasticsearch"
)

func checkElasticsearchIndex(v string) error {
	return elasticsearch.CheckIndexName(strings.TrimSpace(v))
}
