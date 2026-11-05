// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform/config"
)

// searchIndexAdapters builds the Index of each value of search whose adapter is in this build
// (ADR-0149 §3): the files of the adapters, under their build tags, add theirs.
var searchIndexAdapters = map[string]func(ctx context.Context, cfg *config.Config, logger *slog.Logger) (search.Index, error){}

// newSearchIndex returns the Index of search, AppOptions.SearchIndex standing in for it, and nil
// with search = none, which opens no connection to anything (ADR-0149 §2).
func newSearchIndex(ctx context.Context, cfg *config.Config, opts AppOptions, logger *slog.Logger) (search.Index, error) {
	if opts.SearchIndex != nil {
		return opts.SearchIndex, nil
	}
	if cfg.Search == "" || cfg.Search == "none" {
		return nil, nil
	}
	build, ok := searchIndexAdapters[cfg.Search]
	if !ok {
		return nil, fmt.Errorf("search = %s: this build has no adapter of %s", cfg.Search, cfg.Search)
	}
	index, err := build(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("search = %s: %w", cfg.Search, err)
	}
	return index, nil
}

// setUpSearch builds the search module and registers its job types, which exist whatever search
// says (ADR-0149 §7).
func (a *App) setUpSearch(ctx context.Context, cfg *config.Config, opts AppOptions) error {
	index, err := newSearchIndex(ctx, cfg, opts, a.Logger)
	if err != nil {
		return err
	}
	a.Search = search.NewService(index)
	a.Search.RegisterJobs(a.Handlers)
	return nil
}

// searchAPI returns the full-text search of the client API, nil with search = none.
func (a *App) searchAPI() clientapi.FullTextSearch {
	if a.Search == nil || !a.Search.Enabled() {
		return nil
	}
	return a.Search
}
