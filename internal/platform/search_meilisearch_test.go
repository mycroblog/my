// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_search_meilisearch

package platform_test

import (
	"log/slog"
	"testing"

	"github.com/mycroblog/my/internal/platform"
)

// TestSearchMeilisearchStartsWithoutTheEngine starts a server with search = meilisearch whose
// engine is not there: the adapter connects to nothing at start, so the engine being down never
// stops the server, and the first call creates the index.
func TestSearchMeilisearchStartsWithoutTheEngine(t *testing.T) {
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_SEARCH": "meilisearch", "MY_SEARCH_MEILISEARCH_URL": "http://127.0.0.1:9",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if !app.Search.Enabled() {
		t.Error("Search.Enabled() = false with search = meilisearch")
	}
}
