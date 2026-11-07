// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_search_elasticsearch

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["elasticsearch"] = newElasticsearchIndex }

// newElasticsearchIndex reports that the build tag no_search_elasticsearch left the Elasticsearch
// adapter out.
func newElasticsearchIndex(context.Context, *config.Config, *slog.Logger, kernel.Meter) (search.Index, error) {
	return nil, config.ExcludedError("search", "elasticsearch", "no_search_elasticsearch")
}
