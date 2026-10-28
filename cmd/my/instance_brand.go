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
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// runInstanceBrand implements `my instance brand <show|clear>`: the instance's own icon and logos,
// which are set from the admin screen, since it draws their PNG renditions.
func runInstanceBrand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		instanceBrandUsage(stderr)
		return 2
	}
	sub := args[0]
	nargs := 0
	switch sub {
	case "-h", "-help", "--help":
		instanceBrandUsage(stderr)
		return 0
	case "show":
	case "clear":
		nargs = 1
	default:
		_, _ = fmt.Fprintf(stderr, "my instance brand: unknown subcommand %q\n\n", sub)
		instanceBrandUsage(stderr)
		return 2
	}
	name := "my instance brand " + sub
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { instanceBrandUsage(stderr) }
	file := fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != nargs {
		_, _ = fmt.Fprintf(stderr, "%s: want %d argument(s), got %d\n\n", name, nargs, fs.NArg())
		instanceBrandUsage(stderr)
		return 2
	}
	var kind instance.BrandKind
	if sub == "clear" {
		k, ok := instance.ParseBrandKind(fs.Arg(0))
		if !ok {
			_, _ = fmt.Fprintf(stderr, "%s: unknown kind %q; want icon, logo, or logo_dark\n", name, fs.Arg(0))
			return 2
		}
		kind = k
	}

	cfg, _, err := config.Load(config.Source{File: *file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	ctx := context.Background()
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	defer func() { _ = app.Close() }()

	if sub == "clear" {
		rev, err := app.Instance.ClearBrand(ctx, kind, "")
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "%s: the built-in image is in use (revision %s)\n", kind, rev.ID)
		return 0
	}
	brands, err := app.Instance.Brands(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	printBrands(stdout, cfg.ServerOrigin(), brands)
	return 0
}

// printBrands prints each brand kind: its revision, when and by whom it was set, the background of
// the icon, and the URL of its SVG; or that it uses the built-in image.
func printBrands(w io.Writer, origin string, b instance.Brands) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tREVISION\tSET AT\tBY\tBACKGROUND\tSVG")
	for _, k := range instance.BrandKinds {
		rev := b.Get(k)
		if rev == nil {
			_, _ = fmt.Fprintf(tw, "%s\t(built-in)\t-\t-\t-\t-\n", k)
			continue
		}
		by := rev.RevisedByUserID
		if by == "" {
			by = "(cli)"
		}
		bg := rev.Background
		if bg == "" {
			bg = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", k, rev.ID, rev.RevisedAt.UTC().Format(time.RFC3339), by, bg,
			strings.TrimSuffix(origin, "/")+instance.BrandFilePath(rev.ID, instance.RenditionSVG))
	}
	_ = tw.Flush()
}

func instanceBrandUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage:
  my instance brand show [-config <file>]
  my instance brand clear [-config <file>] <icon|logo|logo_dark>

Shows and clears the instance's own icon, logo, and dark logo
(docs/operations/instance-settings.md). They are set from the server settings
of the admin screen, which draws the PNG versions of the uploaded SVG; the
command line cannot set them.

show lists each image with its revision, when and by whom it was set, the
background color of the icon, and the URL of its SVG, or (built-in) when the
built-in image is in use. clear brings one image back to the built-in one and
records the change; the files of earlier revisions stay at their URLs.
`)
}
