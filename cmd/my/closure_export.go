// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/maintenance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// exportClosureHTML implements `my admin closure export-html` (ADR-0088 §6): the static closure
// page in the template of the maintenance page (ADR-0084 §4), as <out>/<locale>.html and
// <out>/index.html, and the examples of the front ends' configuration that serve it, for the
// domain to answer once the server is stopped. It reads the closure and the instance from the
// database, which the export needs: the closure is refused before the server is closed.
func exportClosureHTML(c subcommand, out, langs string, stdout, stderr io.Writer) int {
	if out == "" {
		_, _ = fmt.Fprintf(stderr, "%s: -out is required\n\n", c.name)
		adminClosureUsage(stderr)
		return 2
	}
	bundle, err := i18n.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	locales, code := parseLocales(c.name, langs, bundle, stderr)
	if code >= 0 {
		return code
	}
	cfg, _, err := config.Load(config.Source{File: *c.file})
	if err != nil {
		printLoadError(stderr, err)
		return 1
	}
	ctx := context.Background()
	app, code := c.open(ctx, stderr)
	if code >= 0 {
		return code
	}
	defer func() { _ = app.Close() }()
	closure, err := app.Instance.Closure(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	switch closure.CurrentState() {
	case instance.ClosureDeliveryCompleted:
	case instance.ClosureClosed:
		_, _ = fmt.Fprintf(stderr, "warning: the Deletes of the closure are still being delivered; keep my serve and the workers running until my admin closure status says %s, and switch to the static page after the export deadline\n",
			instance.ClosureDeliveryCompleted)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: the server is not closed (state: %s); close it first with my admin closure close\n", c.name, closure.CurrentState())
		return 1
	}
	info, err := readInstanceForExport(ctx, app)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	if len(locales) == 0 {
		locales = bundle.Locales()
		if i := slices.Index(locales, info.locale); i > 0 {
			locales = append([]string{info.locale}, slices.Delete(locales, i, i+1)...)
		}
	}
	base := platform.MaintenancePage(cfg, info.theme)
	page := maintenance.ClosurePage{
		ServerName: base.ServerName, Message: closure.Schedule.Message, ClosesAt: closure.Schedule.ClosesAt,
		StatusPageURL: base.StatusPageURL, Theme: base.Theme,
	}
	if cfg.MaintenanceServerName == "" && info.name != "" {
		page.ServerName = info.name
	}
	if err := writeClosureFiles(out, cfg.Host, page, bundle, locales); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", c.name, err)
		return 1
	}
	for _, l := range locales {
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(out, l+".html"))
	}
	for _, name := range append([]string{"index.html"}, maintenance.ClosureConfigs...) {
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(out, name))
	}
	return 0
}

// writeClosureFiles writes the closure pages of locales and the examples of configuration for host.
func writeClosureFiles(dir, host string, page maintenance.ClosurePage, bundle *i18n.Bundle, locales []string) error {
	err := writeStaticPages(dir, bundle, locales, func(ls []*i18n.Localizer) ([]byte, error) {
		return maintenance.RenderClosure(page, ls)
	})
	if err != nil {
		return err
	}
	for _, name := range maintenance.ClosureConfigs {
		b, err := maintenance.ClosureConfig(name, host, locales)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil { //nolint:gosec // G306: examples to copy
			return err
		}
	}
	return nil
}
