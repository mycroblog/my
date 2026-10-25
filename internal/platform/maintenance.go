// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/mycroblog/my/internal/adapter/system"
	"github.com/mycroblog/my/internal/adapter/telemetry/none"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/maintenance"
	"github.com/mycroblog/my/internal/platform/config"
)

// MaintenanceApp is the server of the maintenance mode (ADR-0084): it opens no adapter, runs no
// job, and answers every request with the maintenance responses of internal/interface/maintenance.
// It passes the middleware of the App that needs no adapter: request IDs, the access log, the
// security headers, and panic recovery.
type MaintenanceApp struct {
	app     *App
	handler http.Handler
}

// NewMaintenanceApp assembles the maintenance mode from a configuration that
// config.LoadMaintenance read. Unlike NewApp, it creates no adapter, so it starts while the
// database, the KV, the queue, and the blob store are out of reach.
func NewMaintenanceApp(cfg *config.Config, opts AppOptions) (*MaintenanceApp, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	var i18nOpts []i18n.Option
	if opts.PseudoLocales {
		i18nOpts = append(i18nOpts, i18n.WithPseudoLocales())
	}
	messages, err := i18n.Load(i18nOpts...)
	if err != nil {
		return nil, err
	}
	a := &App{
		Config: cfg, Logger: opts.Logger, Messages: messages, Locales: &i18n.Resolver{Bundle: messages},
		Clock: system.Clock{}, IDs: system.IDGenerator{}, Meter: none.New(), Tracer: none.NewTracer(),
	}
	h := maintenance.New(maintenance.Options{
		Page: MaintenancePage(cfg, nil), RetryAfter: cfg.MaintenanceRetryAfter, Locales: a.Locales,
		Now: a.Clock.Now, Logger: a.Logger,
	})
	return &MaintenanceApp{app: a, handler: h}, nil
}

// MaintenancePage is the maintenance page of cfg (ADR-0084 §4) in the default theme with the
// overrides of the instance's theme (ADR-0059 §2; nil for none). The server's name is
// maintenance_server_name, or the host name.
func MaintenancePage(cfg *config.Config, overrides []instance.ThemeTokenValue) maintenance.Page {
	name := cfg.MaintenanceServerName
	if name == "" {
		name = cfg.Host
	}
	return maintenance.Page{
		ServerName: name, Message: cfg.MaintenanceMessage, Until: cfg.MaintenanceUntil, StatusPageURL: cfg.StatusPageURL,
		Theme: maintenanceTheme(overrides),
	}
}

// maintenanceTheme fills the tokens of the maintenance page with the defaults of theme.css and
// then the overrides.
func maintenanceTheme(overrides []instance.ThemeTokenValue) maintenance.Theme {
	t := maintenance.Theme{Light: map[string]string{}, Dark: map[string]string{}}
	for _, name := range maintenance.TokenNames {
		tok, ok := instance.LookupThemeToken(name)
		if !ok {
			continue
		}
		if tok.Kind == instance.ThemeTokenColor {
			t.Light[name], t.Dark[name] = tok.Light, tok.Dark
		} else {
			t.Light[name], t.Dark[name] = tok.Default, tok.Default
		}
	}
	for _, v := range overrides {
		if _, ok := t.Light[v.Token]; !ok {
			continue
		}
		if v.Value != "" {
			t.Light[v.Token], t.Dark[v.Token] = v.Value, v.Value
			continue
		}
		if v.Light != "" {
			t.Light[v.Token] = v.Light
		}
		if v.Dark != "" {
			t.Dark[v.Token] = v.Dark
		}
	}
	return t
}

// Handler returns the HTTP handler of the maintenance mode.
func (m *MaintenanceApp) Handler() http.Handler {
	a := m.app
	h := a.recoverPanics(m.handler)
	h = securityHeaders(h)
	h = a.accessLog(h)
	return a.requestIDs(h)
}

// ServeMaintenance runs the maintenance mode until ctx ends, then shuts down gracefully, as
// Serve does. Whatever the role, it serves HTTP only: no job runner, no scheduler, no streams,
// and no metrics listener.
func ServeMaintenance(ctx context.Context, m *MaintenanceApp, opts ServeOptions) error {
	a := m.app
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = DefaultShutdownTimeout
	}
	ln := opts.Listener
	if ln == nil {
		var err error
		if ln, err = (&net.ListenConfig{}).Listen(ctx, "tcp", a.Config.Listen); err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}
	srv := &http.Server{
		Handler:           m.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(a.Logger.Handler(), slog.LevelWarn),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	a.Logger.InfoContext(ctx, "started in maintenance mode", slog.String("addr", ln.Addr().String()))
	var err error
	select {
	case <-ctx.Done():
	case err = <-serveErr:
		err = fmt.Errorf("http server: %w", err)
	}
	graceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opts.ShutdownTimeout)
	defer cancel()
	if shutdownErr := srv.Shutdown(graceCtx); shutdownErr != nil && !errors.Is(shutdownErr, http.ErrServerClosed) {
		err = errors.Join(err, fmt.Errorf("http server shutdown: %w", shutdownErr))
	}
	if err == nil {
		a.Logger.InfoContext(ctx, "stopped")
	}
	return err
}
