// SPDX-License-Identifier: AGPL-3.0-only

//go:build no_search_meilisearch

package platform

import (
	"context"
	"log/slog"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

// The build tag no_search_meilisearch left the Meilisearch adapter out; the configuration refuses
// search = meilisearch first, and this stands in for a configuration built without that check.
func init() {
	searchIndexAdapters["meilisearch"] = func(context.Context, *config.Config, *slog.Logger, kernel.Meter) (search.Index, error) {
		return nil, config.ExcludedError("search", "meilisearch", "no_search_meilisearch")
	}
}
