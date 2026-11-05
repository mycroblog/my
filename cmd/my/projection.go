// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/mycroblog/my/internal/app/projection"
	"github.com/mycroblog/my/internal/platform/config"
)

// maxDiffRows is how many differing rows `my projection verify` prints per table.
const maxDiffRows = 20

// runProjection implements `my projection <subcommand>` (data-model §9): list the projections,
// rebuild them from their sources, or verify that they hold what a rebuild derives.
func runProjection(args []string, stdout, stderr io.Writer) int {
	sub, code := dispatch("projection", args, []string{"list", "rebuild", "verify"}, stderr, projectionUsage)
	if code >= 0 {
		return code
	}
	name := "my projection " + sub
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { projectionUsage(stderr) }
	c := subcommand{name: name, fs: fs, file: fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")}
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	maxArgs := 1
	if sub == "list" {
		maxArgs = 0
	}
	if fs.NArg() > maxArgs {
		_, _ = fmt.Fprintf(stderr, "%s: want at most %d argument(s), got %d\n\n", name, maxArgs, fs.NArg())
		projectionUsage(stderr)
		return 2
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	svc := app.Projections
	if sub == "list" {
		printProjections(stdout, svc.List())
		return 0
	}
	targets := svc.List()
	if fs.NArg() == 1 {
		p, err := svc.Lookup(fs.Arg(0))
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v (see my projection list)\n", name, err)
			return 2
		}
		targets = []projection.Projection{p}
	}
	drifted := false
	for _, p := range targets {
		var (
			r   projection.Report
			err error
		)
		if sub == "rebuild" {
			r, err = svc.Rebuild(ctx, p)
		} else {
			r, err = svc.Verify(ctx, p)
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return 1
		}
		if sub == "rebuild" {
			_, _ = fmt.Fprintf(stdout, "%s: rebuilt (%d rows deleted, %d inserted)\n", p.Name, r.Deleted, r.Inserted)
			continue
		}
		if !r.Drifted() {
			_, _ = fmt.Fprintf(stdout, "%s: ok\n", p.Name)
			continue
		}
		drifted = true
		printDrift(stdout, r)
	}
	if drifted {
		_, _ = fmt.Fprintf(stderr, "%s: projections differ from their sources; run my projection rebuild\n", name)
		return 1
	}
	return 0
}

func printProjections(w io.Writer, ps []projection.Projection) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tMODULE\tTABLES")
	for _, p := range ps {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", p.Name, p.Module, strings.Join(p.Tables, ", "))
	}
	_ = tw.Flush()
}

// printDrift prints the rows of each table that differ, "-" for a row that the table holds and
// a rebuild does not derive and "+" for one that a rebuild derives and the table lacks.
func printDrift(w io.Writer, r projection.Report) {
	_, _ = fmt.Fprintf(w, "%s: differs from its sources\n", r.Projection.Name)
	for _, t := range r.Tables {
		if t.Empty() {
			continue
		}
		_, _ = fmt.Fprintf(w, "  %s: %d extra, %d missing\n", t.Table, len(t.Extra), len(t.Missing))
		printed := 0
		for _, rows := range []struct {
			mark string
			rows []projection.Row
		}{{"-", t.Extra}, {"+", t.Missing}} {
			for _, row := range rows.rows {
				if printed == maxDiffRows {
					break
				}
				_, _ = fmt.Fprintf(w, "    %s %s\n", rows.mark, row)
				printed++
			}
		}
		if rest := len(t.Extra) + len(t.Missing) - printed; rest > 0 {
			_, _ = fmt.Fprintf(w, "    ... and %d more\n", rest)
		}
	}
}

func projectionUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my projection list [-config <file>]
  my projection verify [-config <file>] [<name>]
  my projection rebuild [-config <file>] [<name>]

Projections are tables derived from the events and resources of their module.
list shows each projection with its module and tables. verify derives every
projection, or the one named, from its sources inside a transaction that it
rolls back, prints the rows that differ from what the tables hold ("-" held but
not derived, "+" derived but missing), and exits with 1 when any differ; it
changes nothing. rebuild derives them the same way and writes the difference, at
most 500 rows per transaction, so that readers never see an emptied projection.
Run rebuild while the instance is quiet: a row that changes during the rebuild
keeps its new value, and verify reports any drift that is left. <name> is a
projection or one of its tables. The worker runs the same check and correction
once a day (projection.verify).
`)
}
