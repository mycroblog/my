// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/maintenance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// exportDBTimeout bounds how long `my maintenance export` waits for the database before it writes
// the pages from the configuration alone.
const exportDBTimeout = 5 * time.Second

// runMaintenance implements `my maintenance <subcommand>`.
func runMaintenance(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 0:
		maintenanceUsage(stderr)
		return 2
	case args[0] == "-h" || args[0] == "-help" || args[0] == "--help":
		maintenanceUsage(stderr)
		return 0
	case args[0] != "export":
		_, _ = fmt.Fprintf(stderr, "my maintenance: unknown subcommand %q\n\n", args[0])
		maintenanceUsage(stderr)
		return 2
	}
	const name = "my maintenance export"
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { maintenanceUsage(stderr) }
	out := fs.String("out", "", "directory to write the pages to (required)")
	langs := fs.String("lang", "", "comma-separated locales, in the order of index.html (default: the instance's default locale, or en)")
	file := fs.String("config", "", "TOML config file (default: $"+config.EnvConfigFile+")")
	offline := fs.Bool("offline", false, "do not read the database: use the configuration and the default theme")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "%s: unexpected argument %q\n", name, fs.Arg(0))
		return 2
	}
	if *out == "" {
		_, _ = fmt.Fprintf(stderr, "%s: -out is required\n\n", name)
		maintenanceUsage(stderr)
		return 2
	}
	bundle, err := i18n.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	locales, code := parseLocales(name, *langs, bundle, stderr)
	if code >= 0 {
		return code
	}

	src := config.Source{File: *file}
	cfg, _, err := config.LoadMaintenance(src)
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	page := platform.MaintenancePage(cfg, nil)
	if !*offline {
		info, err := readExportInstance(src)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: warning: the database cannot be read, so the pages use the configuration and the default theme: %v\n", name, err)
		} else {
			page = platform.MaintenancePage(cfg, info.theme)
			if cfg.MaintenanceServerName == "" && info.name != "" {
				page.ServerName = info.name
			}
			if len(locales) == 0 && slices.Contains(bundle.Locales(), info.locale) {
				locales = []string{info.locale}
			}
		}
	}
	if len(locales) == 0 {
		locales = []string{"en"}
	}
	if err := writeMaintenancePages(*out, page, bundle, locales); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	for _, l := range locales {
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(*out, l+".html"))
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(*out, "index.html"))
	return 0
}

// exportInstance is what `my maintenance export` reads from the database.
type exportInstance struct {
	name, locale string
	// theme holds the overrides of the design tokens; the custom CSS is never exported (ADR-0084 §5).
	theme []instance.ThemeTokenValue
}

// readExportInstance reads the instance's name, default locale, and theme within exportDBTimeout.
func readExportInstance(src config.Source) (exportInstance, error) {
	cfg, _, err := config.Load(src)
	if err != nil {
		return exportInstance{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), exportDBTimeout)
	defer cancel()
	app, err := platform.NewApp(ctx, cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		return exportInstance{}, err
	}
	defer func() { _ = app.Close() }()
	if err := app.Ready(ctx); err != nil {
		return exportInstance{}, err
	}
	return readInstanceForExport(ctx, app)
}

// readInstanceForExport reads the instance's name, default locale, and theme from app.
func readInstanceForExport(ctx context.Context, app *platform.App) (exportInstance, error) {
	var info exportInstance
	text, ok, err := app.Instance.Text(ctx, instance.TextName)
	if err != nil {
		return exportInstance{}, err
	}
	if ok {
		info.name = text.Body
	}
	if info.locale, err = app.Instance.DefaultLocale(ctx); err != nil {
		return exportInstance{}, err
	}
	rev, _, err := app.Instance.Theme(ctx)
	if err != nil {
		return exportInstance{}, err
	}
	info.theme = rev.Tokens
	return info, nil
}

// writeMaintenancePages writes <dir>/<locale>.html for each locale and <dir>/index.html with every
// locale in order, each with its Content-Security-Policy in a <meta> element.
func writeMaintenancePages(dir string, page maintenance.Page, bundle *i18n.Bundle, locales []string) error {
	return writeStaticPages(dir, bundle, locales, func(ls []*i18n.Localizer) ([]byte, error) {
		html, _, err := maintenance.Render(page, ls, maintenance.RenderOptions{MetaCSP: true})
		return html, err
	})
}

// writeStaticPages writes <dir>/<locale>.html for each locale and <dir>/index.html with every
// locale in order, each rendered by render with the localizers of its locales.
func writeStaticPages(dir string, bundle *i18n.Bundle, locales []string, render func([]*i18n.Localizer) ([]byte, error)) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	all := make([]*i18n.Localizer, len(locales))
	for i, l := range locales {
		all[i] = bundle.Localizer(l)
		html, err := render(all[i : i+1])
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, l+".html"), html, 0o644); err != nil { //nolint:gosec // G306: public pages
			return err
		}
	}
	html, err := render(all)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), html, 0o644) //nolint:gosec // G306: public pages
}

// parseLocales reads -lang: comma-separated locales, canonicalized, without repeats, each with a
// catalog in bundle. It returns the exit code to stop with, or -1 to go on.
func parseLocales(name, langs string, bundle *i18n.Bundle, stderr io.Writer) ([]string, int) {
	var locales []string
	for l := range strings.SplitSeq(langs, ",") {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		c := i18n.Canonicalize(l)
		if !slices.Contains(bundle.Locales(), c) {
			_, _ = fmt.Fprintf(stderr, "%s: unknown locale %q; want one of %s\n", name, l, strings.Join(bundle.Locales(), ", "))
			return nil, 2
		}
		if !slices.Contains(locales, c) {
			locales = append(locales, c)
		}
	}
	return locales, -1
}

func maintenanceUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: my maintenance export -out <dir> [-lang en,ja] [-config <file>] [-offline]

export writes the maintenance page of the maintenance mode (ADR-0084) as static,
self-contained HTML for a load balancer, CDN, or reverse proxy to serve while
my serve cannot run: <dir>/<locale>.html for each locale of -lang, and
<dir>/index.html with every locale one after another, for the front ends that
cannot choose a page by Accept-Language. The pages have inline CSS, no script,
and no external resource; their Content-Security-Policy is in a <meta> element.

The words come from the maintenance_* settings and status_page_url. Unless
-offline is given, export also reads the database, waiting at most 5 seconds,
for the instance's name (when maintenance_server_name is unset), its default
locale (when -lang is not given), and the token overrides of its theme; the
custom CSS of the theme is never included. When the database cannot be read,
export warns, writes the pages from the configuration and the default theme,
and still exits 0.
`)
}
