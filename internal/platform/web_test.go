// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/platform"
)

const (
	acceptBrowser  = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	acceptMastodon = `application/activity+json, application/ld+json; profile="https://www.w3.org/ns/activitystreams", text/html;q=0.1`
)

// TestWebClient checks how the composition root splits requests between the web client's
// shell, its files, ActivityPub, and the other endpoints (ADR-0028).
func TestWebClient(t *testing.T) {
	t.Parallel()
	cfg := loadConfig(t, map[string]string{"MY_DATABASE_URL": newDatabase(t, true)})
	build := fstest.MapFS{
		".vite/manifest.json": {Data: []byte(`{"index.html": {"file": "assets/index-AAAA.js", "isEntry": true, "css": ["assets/index-CCCC.css"]},
			"src/server-pages.css": {"file": "assets/server-pages-FFFF.css", "isEntry": true}}`)},
		"assets/index-AAAA.js":         {Data: []byte("export {}")},
		"assets/index-CCCC.css":        {Data: []byte("body{}")},
		"assets/server-pages-FFFF.css": {Data: []byte("main{}")},
		"sw.js":                        {Data: []byte("// sw")},
	}
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler), WebBuild: build})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	reg, err := app.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{
		Username: "alice", Password: "correct horse battery staple", DisplayName: "Alice",
	})
	if err != nil {
		t.Fatal(err)
	}
	h := app.Handler()

	const shell, problem, file = "shell", "problem", "file"
	tests := []struct {
		name, method, path, accept string
		status                     int
		kind                       string
		vary                       []string
	}{
		{"profile page, browser", http.MethodGet, "/@alice", acceptBrowser, http.StatusOK, shell, []string{"Accept"}},
		{"profile page, ActivityPub", http.MethodGet, "/@alice", acceptMastodon, http.StatusFound, "", []string{"Accept"}},
		{"missing profile, ActivityPub", http.MethodGet, "/@nobody", acceptMastodon, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"missing profile, browser", http.MethodGet, "/@nobody", acceptBrowser, http.StatusNotFound, shell, []string{"Accept"}},
		{"missing post page, browser", http.MethodGet, "/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f", acceptBrowser, http.StatusNotFound, shell, []string{"Accept"}},
		{"missing post page, ActivityPub", http.MethodGet, "/@alice/status/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f", acceptMastodon, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"root", http.MethodGet, "/", acceptBrowser, http.StatusOK, shell, []string{"Accept"}},
		{"single-segment route", http.MethodGet, "/home", "", http.StatusOK, shell, []string{"Accept"}},
		{"single-segment route, ActivityPub", http.MethodGet, "/home", acceptMastodon, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"nested route", http.MethodGet, "/settings/profile", acceptBrowser, http.StatusOK, shell, []string{"Accept"}},
		{"HEAD", http.MethodHead, "/settings/profile", acceptBrowser, http.StatusOK, "", []string{"Accept"}},
		{"actor, browser", http.MethodGet, "/ap/actors/" + reg.AccountID, acceptBrowser, http.StatusSeeOther, "", []string{"Accept"}},
		{"asset", http.MethodGet, "/assets/index-AAAA.js", "", http.StatusOK, file, nil},
		{"root file", http.MethodGet, "/sw.js", "", http.StatusOK, file, []string{"Accept"}},
		{"missing asset", http.MethodGet, "/assets/gone.js", "", http.StatusNotFound, problem, []string{"Accept-Language"}},
		{"manifest", http.MethodGet, "/manifest.webmanifest", "", http.StatusOK, "", nil},
		{"client API", http.MethodGet, "/api/v1/nope", acceptBrowser, http.StatusNotFound, problem, []string{"Origin", "Accept", "Accept-Language"}},
		{"sign-in page", http.MethodGet, "/auth/sign_in", acceptBrowser, http.StatusOK, "signin", []string{"Accept-Language, Cookie"}},
		{"unknown auth page", http.MethodGet, "/auth/nope", acceptBrowser, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"unknown OAuth path", http.MethodGet, "/oauth/nope", acceptBrowser, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"ActivityPub", http.MethodGet, "/ap/nope/x/y", acceptBrowser, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"NodeInfo", http.MethodGet, "/nodeinfo", acceptBrowser, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"well-known", http.MethodGet, "/.well-known/nope", acceptBrowser, http.StatusNotFound, problem, []string{"Accept", "Accept-Language"}},
		{"media", http.MethodGet, "/media/a/b.png", acceptBrowser, http.StatusNotFound, problem, []string{"Accept-Language"}},
		{"POST", http.MethodPost, "/settings/profile", acceptBrowser, http.StatusNotFound, problem, []string{"Accept-Language"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, "https://"+cfg.Host+tt.path, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.status)
			}
			hd := rec.Header()
			switch tt.kind {
			case shell:
				if !strings.Contains(rec.Body.String(), `<script type="module" src="/assets/index-AAAA.js"></script>`) {
					t.Errorf("not the shell: %s", rec.Body)
				}
				csp := hd.Get("Content-Security-Policy")
				if !strings.Contains(csp, "script-src 'self';") || strings.Contains(csp, "nonce") {
					t.Errorf("shell CSP = %q", csp)
				}
			case "signin":
				// The server-rendered screens load the stylesheet the web build makes for them (ADR-0090).
				body := rec.Body.String()
				if !strings.Contains(body, `<link rel="stylesheet" href="/assets/server-pages-FFFF.css">`) || strings.Contains(body, "index-CCCC.css") ||
					strings.Contains(body, `id="config"`) {
					t.Errorf("sign-in page is the shell or lacks the server pages' CSS: %s", body)
				}
			case problem:
				if hd.Get("Content-Type") != "application/problem+json" {
					t.Errorf("Content-Type = %q, want a problem", hd.Get("Content-Type"))
				}
			case file:
				if !strings.Contains(hd.Get("Cache-Control"), "immutable") && hd.Get("Cache-Control") != "no-cache" {
					t.Errorf("file Cache-Control = %q", hd.Get("Cache-Control"))
				}
			}
			if got := hd.Values("Vary"); !reflect.DeepEqual(got, tt.vary) {
				t.Errorf("Vary = %q, want %q", got, tt.vary)
			}
			if hd.Get("X-Content-Type-Options") != "nosniff" || hd.Get("X-Frame-Options") != "DENY" {
				t.Errorf("security headers missing: %v", hd)
			}
		})
	}
}
