// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_meilisearch

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mycroblog/my/internal/adapter/search/meilisearch"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/platform/config"
)

func init() { searchIndexAdapters["meilisearch"] = newMeilisearchIndex }

// newMeilisearchIndex returns the Index of search = meilisearch (ADR-0149 §14). It connects to
// nothing: the first call creates the index. A request has 10 seconds, as long as a call of the
// search module, which bounds the waiting for a task with its own context.
//
// The settings file is read here, once a start; its content never reaches a log.
func newMeilisearchIndex(_ context.Context, cfg *config.Config, _ *slog.Logger) (search.Index, error) {
	var settings []byte
	if cfg.SearchMeilisearchSettingsFile != "" {
		b, err := os.ReadFile(cfg.SearchMeilisearchSettingsFile)
		if err != nil {
			return nil, fmt.Errorf("search_meilisearch_settings_file: %w", err)
		}
		settings = b
	}
	return meilisearch.New(meilisearch.Options{
		URL: cfg.SearchMeilisearchURL, APIKey: cfg.SearchMeilisearchAPIKey, Index: cfg.SearchMeilisearchIndex,
		Settings: settings, HTTPClient: searchHTTPClient(10 * time.Second),
	})
}
