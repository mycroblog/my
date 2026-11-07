// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_meilisearch

package platform

import (
	"context"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/meilisearch"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["meilisearch"] = newMeilisearchIndex }

// newMeilisearchIndex returns the Index of search = meilisearch (ADR-0149 §14). It connects to
// nothing: the first call creates the index. A request has 10 seconds, as long as a call of the
// search module, which bounds the waiting for a task with its own context.
//
// Load read and checked the settings file; its content never reaches a log.
func newMeilisearchIndex(_ context.Context, cfg *config.Config, _ *slog.Logger, _ kernel.Meter) (search.Index, error) {
	return meilisearch.New(meilisearch.Options{
		URL: cfg.SearchMeilisearchURL, APIKey: cfg.SearchMeilisearchAPIKey, Index: cfg.SearchMeilisearchIndex,
		Settings: cfg.SearchMeilisearchSettings, HTTPClient: searchHTTPClient(10 * time.Second),
	})
}
