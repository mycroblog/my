// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

// The maintenance mode passes the middleware that needs no adapter (ADR-0084 §3): the security headers of
// ADR-0019, the request ID, and the page's own CSP, in the default theme of theme.css.
func TestMaintenanceApp(t *testing.T) {
	env := map[string]string{"MY_HOST": "example.test", "MY_DB": "postgres", "MY_DATABASE_URL": "postgres://unreachable.invalid/my"}
	cfg, _, err := config.LoadMaintenance(config.Source{LookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok }})
	if err != nil {
		t.Fatal(err)
	}
	app, err := platform.NewMaintenanceApp(cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/", nil))
	h := rec.Header()
	if rec.Code != http.StatusServiceUnavailable || h.Get("Strict-Transport-Security") == "" || h.Get("X-Frame-Options") != "DENY" ||
		h.Get("X-Request-Id") == "" || h.Get("Retry-After") != "600" || !strings.Contains(h.Get("Content-Security-Policy"), "style-src 'sha256-") {
		t.Errorf("GET / = %d %v", rec.Code, h)
	}
	surface, _ := instance.LookupThemeToken("color-surface")
	body := rec.Body.String()
	if !strings.Contains(body, "--color-surface:"+surface.Light+";") || !strings.Contains(body, "--color-surface:"+surface.Dark+";") ||
		!strings.Contains(body, `<p class="server" dir="auto">example.test</p>`) {
		t.Errorf("page lacks the default theme or the host name:\n%s", body)
	}
}

// MaintenancePage applies the instance theme's overrides of the tokens the page uses (ADR-0059 §2).
func TestMaintenancePageTheme(t *testing.T) {
	p := platform.MaintenancePage(&config.Config{Host: "example.test", MaintenanceServerName: "Example"}, []instance.ThemeTokenValue{
		{Token: "color-accent", Light: "#112233", Dark: "#445566"},
		{Token: "font-sans", Value: "Georgia, serif"},
		{Token: "radius-md", Value: "1rem"},
	})
	if p.ServerName != "Example" || p.Theme.Light["color-accent"] != "#112233" || p.Theme.Dark["color-accent"] != "#445566" ||
		p.Theme.Dark["font-sans"] != "Georgia, serif" {
		t.Errorf("page = %+v", p)
	}
	if _, ok := p.Theme.Light["radius-md"]; ok {
		t.Error("the theme has a token the page does not use")
	}
}
