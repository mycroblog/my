// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/mycroblog/my/internal/app/search"
)

// searchFailuresShown is how many failures of a rebuild `my search status` prints.
const searchFailuresShown = 20

// runSearch implements `my search <subcommand>`: the index of the full-text search of posts, which a
// server with search = none does not have (ADR-0149 §10, §12).
func runSearch(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("search", args, []string{"rebuild", "clear", "status"}, stderr, searchUsage)
	if code >= 0 {
		return code
	}
	var (
		keep bool
		rate int
	)
	c, code := parseSubcommand("my search "+sub, args[1:], 0, stderr, searchUsage, func(fs *flag.FlagSet) {
		if sub == "rebuild" {
			fs.BoolVar(&keep, "keep", false, "add the posts without emptying the index first")
			fs.IntVar(&rate, "rate", 0, fmt.Sprintf("the most posts a second, up to %d (default %d)", search.MaxRebuildRate, search.DefaultRebuildRate))
		}
	})
	if code >= 0 {
		return code
	}
	if rate < 0 || rate > search.MaxRebuildRate {
		_, _ = fmt.Fprintf(stderr, "%s: -rate must be between 1 and %d\n", c.name, search.MaxRebuildRate)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	var err error
	switch sub {
	case "rebuild":
		err = searchRebuild(ctx, app.Search, search.RebuildOptions{Keep: keep, Rate: rate}, stdout)
	case "clear":
		if err = app.Search.ClearIndex(ctx); err == nil {
			_, _ = fmt.Fprintln(stdout, "cleared the search index, and ended any rebuild")
		}
	case "status":
		err = searchStatus(ctx, app.Search, app.Config.Search, stdout)
	}
	switch {
	case errors.Is(err, search.ErrNoBackend):
		_, _ = fmt.Fprintln(stdout, noBackendMessage(sub))
		return 0
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	return 0
}

// noBackendMessage says that a command did nothing because search = none.
func noBackendMessage(sub string) string {
	switch sub {
	case "clear":
		return "search = none: this server has no connection to a search engine, so nothing was cleared. Delete an index left in an engine in the engine itself."
	case "status":
		return "search = none: this server has no full-text search of posts."
	}
	return "search = none: this server has no search engine to rebuild an index in, so nothing was done."
}

func searchRebuild(ctx context.Context, svc *search.Service, opts search.RebuildOptions, stdout io.Writer) error {
	out, err := svc.StartRebuild(ctx, opts)
	if errors.Is(err, search.ErrRebuildModeDiffers) {
		return fmt.Errorf("an unfinished rebuild has the mode %s; run the command as it was run to resume it, or my search clear and then this command to start over", out.Rebuild.Mode)
	}
	if err != nil {
		return err
	}
	if out.Cleared {
		_, _ = fmt.Fprintln(stdout, "cleared the search index")
	}
	if out.Resumed {
		_, _ = fmt.Fprintf(stdout, "resumed the rebuild started at %s from where it stopped, after %d posts\n", out.Rebuild.StartedAt.Format(time.RFC3339), out.Rebuild.Processed)
	} else {
		_, _ = fmt.Fprintln(stdout, "started a rebuild of the search index")
	}
	_, _ = fmt.Fprintf(stdout, "the worker takes up to %d posts a second; my search status shows the progress\n", out.Rebuild.Rate)
	return nil
}

func searchStatus(ctx context.Context, svc *search.Service, backend string, stdout io.Writer) error {
	r, failures, ok, err := svc.RebuildProgress(ctx, searchFailuresShown)
	if err != nil {
		return err
	}
	covered, err := svc.Visibilities(ctx)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "backend: %s\nsearch_visibilities: %v\n", backend, covered)
	if !ok {
		_, _ = fmt.Fprintln(stdout, "rebuild: none has run")
		return nil
	}
	printSearchRebuild(stdout, r, failures)
	return nil
}

func printSearchRebuild(w io.Writer, r search.Rebuild, failures []search.RebuildFailure) {
	_, _ = fmt.Fprintf(w, "rebuild: %s (mode %s)\nstarted: %s\nupdated: %s\nposts handled: %d\nposts failed: %d\nrate: %d a second\n",
		r.Phase, r.Mode, r.StartedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339), r.Processed, r.Failed, r.Rate)
	if r.After != "" {
		_, _ = fmt.Fprintf(w, "last post handled: %s\n", r.After)
	}
	if r.LastError != "" {
		_, _ = fmt.Fprintf(w, "last error: %s\n", oneLine(r.LastError))
	}
	for _, f := range failures {
		_, _ = fmt.Fprintf(w, "failed: %s %s %s\n", f.StatusID, f.FailedAt.Format(time.RFC3339), oneLine(f.Message))
	}
}

func searchUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage:
  my search rebuild [-config <file>] [--keep] [--rate <posts a second>]
  my search clear [-config <file>]
  my search status [-config <file>]

The index of the full-text search of posts is a copy of the database, and needs
a search engine: with search = none each command says so and does nothing.

rebuild empties the index and then has the worker add every post that qualifies,
in small batches, up to %d posts a second by default. --keep adds them without
emptying the index, which keeps search working but leaves documents that should
have gone. Run it again to resume a rebuild that stopped: it goes on from where
it was, and does not empty the index again.

clear empties the index and ends any rebuild. Run it before you change search.

status shows the backend and the progress and failures of the latest rebuild.
`, search.DefaultRebuildRate)
}
