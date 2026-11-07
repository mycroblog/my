// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/search"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform/config"
)

// searchIndexAdapters builds the Index of each value of search whose adapter is in this build
// (ADR-0149 §3): the files of the adapters, under their build tags, add theirs. meter is for an
// adapter that counts its calls, as the plugin client does.
var searchIndexAdapters = map[string]func(ctx context.Context, cfg *config.Config, logger *slog.Logger, meter kernel.Meter) (search.Index, error){}

// searchHTTPClient returns the HTTP client of a search backend. The adapter reaches only the
// engine that the operator configured, which may be on a private address, so the client is not
// the SSRF-guarded kernel.HTTPClient (ADR-0149 §3); a call has timeout and follows no redirect.
func searchHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// newSearchIndex returns the Index of search, AppOptions.SearchIndex standing in for it, and nil
// with search = none, which opens no connection to anything (ADR-0149 §2).
func newSearchIndex(ctx context.Context, cfg *config.Config, opts AppOptions, logger *slog.Logger, meter kernel.Meter) (search.Index, error) {
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
	index, err := build(ctx, cfg, logger, meter)
	if err != nil {
		return nil, fmt.Errorf("search = %s: %w", cfg.Search, err)
	}
	return index, nil
}

// setUpSearch builds the search module and registers its job types, which exist whatever search
// says, and with a backend the projector that keeps the index in step with the database; with
// search = none no fact enqueues a job (ADR-0149 §2, §7). It comes after the account, instance,
// moderation, and content modules, and before the erasure, which it takes part in.
func (a *App) setUpSearch(ctx context.Context, cfg *config.Config, repo search.Repository, opts AppOptions) error {
	index, err := newSearchIndex(ctx, cfg, opts, a.Logger, a.Meter)
	if err != nil {
		return err
	}
	outbox := a.Outbox
	if outbox == nil {
		// The inline queue has no transactional enqueue.
		outbox = queue.AfterCommit(a.Projectors, a.Jobs, a.Logger)
	}
	a.Search = search.NewService(search.Deps{
		Index: index, UnitOfWork: a.UnitOfWork, Outbox: outbox,
		Statuses: a.Content, Accounts: a.Accounts, Settings: a.Instance, Viewing: a.Timeline, Sanitizer: newHTMLSanitizer(),
		Rebuilds: repo, Clock: a.Clock, IDs: a.IDs, RebuildBatch: opts.SearchRebuildBatch,
	})
	a.Search.RegisterJobs(a.Handlers)
	if a.Search.Enabled() {
		a.Search.RegisterProjectors(a.Projectors)
	}
	return nil
}

// searchAPI returns the full-text search of the client API, nil with search = none.
func (a *App) searchAPI() clientapi.FullTextSearch {
	if a.Search == nil || !a.Search.Enabled() {
		return nil
	}
	return a.Search
}
