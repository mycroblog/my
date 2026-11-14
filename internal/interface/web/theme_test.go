// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/interface/web"
	"github.com/mycroblog/my/pkg/testkit"
)

var defaultSurface, _ = instance.LookupThemeToken("color-surface")

// fakeThemes serves the current theme and every revision by ID.
type fakeThemes struct {
	current   string
	revisions map[string]instance.ThemeRevision
}

func (f *fakeThemes) theme(context.Context) (instance.ThemeRevision, bool, error) {
	rev, ok := f.revisions[f.current]
	return rev, ok && !rev.Empty(), nil
}

func (f *fakeThemes) revision(_ context.Context, id string) (instance.ThemeRevision, bool, error) {
	rev, ok := f.revisions[id]
	return rev, ok, nil
}

func themeHandler(t *testing.T, f *fakeThemes) http.Handler {
	t.Helper()
	_, mux := newHandler(t, web.Options{Theme: f.theme, ThemeRevision: f.revision})
	return mux
}

// themeRevisionID is the theme revision the fake serves.
var themeRevisionID = testkit.NamedID("theme-revision")

func newThemes() *fakeThemes {
	return &fakeThemes{revisions: map[string]instance.ThemeRevision{
		themeRevisionID: {
			ThemePublication: instance.ThemePublication{ID: themeRevisionID, PublishedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)},
			Set: instance.ThemeSet{
				Tokens: []instance.ThemeTokenValue{{Token: "color-surface", Light: "#fffdf5", Dark: "#101010"}, {Token: "radius-control", Value: "2px"}},
				Stale:  []instance.StaleOverride{{ThemeTokenValue: instance.ThemeTokenValue{Token: "elevation-card", Value: "none"}, Reason: instance.ThemeUnknownToken}},
			},
		},
		"reset": {ThemePublication: instance.ThemePublication{ID: "reset"}},
	}}
}

func TestShellWithoutTheme(t *testing.T) {
	mux := themeHandler(t, newThemes())
	body := get(mux, "/", nil).Body.String()
	for _, want := range []string{
		`<meta name="theme-color" media="(prefers-color-scheme: light)" content="` + defaultSurface.Light + `">`,
		`<meta name="theme-color" media="(prefers-color-scheme: dark)" content="` + defaultSurface.Dark + `">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, web.PathTheme) {
		t.Errorf("shell without a theme links a theme stylesheet:\n%s", body)
	}
}

func TestShellWithTheme(t *testing.T) {
	f := newThemes()
	mux := themeHandler(t, f)
	before := get(mux, "/", nil).Header().Get("ETag")

	f.current = themeRevisionID
	rec := get(mux, "/home", nil)
	body := rec.Body.String()
	link := `<link rel="stylesheet" href="` + web.ThemeStylesheetPath(themeRevisionID) + `">`
	build := `<link rel="stylesheet" href="/assets/shared-DDDD.css">`
	if i, j := strings.Index(body, build), strings.Index(body, link); i < 0 || j < i {
		t.Errorf("the theme stylesheet must follow the build's stylesheets:\n%s", body)
	}
	for _, want := range []string{`content="#fffdf5"`, `content="#101010"`} {
		if !strings.Contains(body, want) {
			t.Errorf("shell lacks theme-color %s", want)
		}
	}
	if rec.Header().Get("ETag") == before {
		t.Error("the shell's ETag did not change with the theme")
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "style-src 'self';") {
		t.Errorf("CSP = %q, want style-src 'self' unchanged", csp)
	}

	var m struct {
		ThemeColor      string `json:"theme_color"`
		BackgroundColor string `json:"background_color"`
	}
	if err := json.Unmarshal(get(mux, web.PathWebManifest, nil).Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.ThemeColor != "#fffdf5" || m.BackgroundColor != "#fffdf5" {
		t.Errorf("manifest colors = %+v, want the light surface", m)
	}

	// A reset theme loads no stylesheet again.
	f.current = "reset"
	if body := get(mux, "/", nil).Body.String(); strings.Contains(body, web.PathTheme) {
		t.Errorf("shell of a reset theme links a stylesheet:\n%s", body)
	}
}

func TestThemeStylesheet(t *testing.T) {
	mux := themeHandler(t, newThemes())
	rec := get(mux, web.ThemeStylesheetPath(themeRevisionID), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET stylesheet = %d", rec.Code)
	}
	h := rec.Header()
	if h.Get("Content-Type") != "text/css; charset=utf-8" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Cache-Control"), "immutable") || h.Get("ETag") == "" {
		t.Errorf("headers = %v", h)
	}
	body := rec.Body.String()
	for _, want := range []string{"--color-surface: light-dark(#fffdf5, #101010);", "--radius-control: 2px;"} {
		if !strings.Contains(body, want) {
			t.Errorf("stylesheet lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "elevation-card") {
		t.Errorf("stylesheet has an override that no longer applies:\n%s", body)
	}
	for _, path := range []string{
		web.ThemeStylesheetPath("unknown"), web.ThemeStylesheetPath("reset"), web.PathTheme + themeRevisionID + ".js", web.PathTheme + themeRevisionID,
	} {
		if rec := get(mux, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
	// /theme.js at the root of the build is not under the stylesheets' prefix.
	if rec := get(mux, "/theme.js", nil); rec.Code != http.StatusOK {
		t.Errorf("GET /theme.js = %d", rec.Code)
	}
}
