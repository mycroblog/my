// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_search_opensearch

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["opensearch"] = newOpenSearchIndex }

// newOpenSearchIndex reports that the build tag no_search_opensearch left the OpenSearch adapter
// out.
func newOpenSearchIndex(context.Context, *config.Config, *slog.Logger, kernel.Meter) (search.Index, error) {
	return nil, config.ExcludedError("search", "opensearch", "no_search_opensearch")
}
