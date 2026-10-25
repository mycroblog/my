// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/internal/platform/config"
)

func runMaintenanceCmd(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(append([]string{"maintenance"}, args...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // G304: a file the test wrote
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// externalResource matches what would make a page load something: a script, an external src or href, @import,
// and url() other than data:.
var externalResource = regexp.MustCompile(`(?i)<script|<link|<img|<iframe|\bsrc=|@import|url\(\s*['"]?[^d'"]|href="(?:https?:)?//[^"]*"[^>]*rel="(?:stylesheet|preload|icon)"`)

func TestMaintenanceExportOffline(t *testing.T) {
	t.Setenv(config.EnvConfigFile, "")
	t.Setenv("MY_HOST", "social.example")
	t.Setenv("MY_MAINTENANCE_MESSAGE", "Moving to a new database.")
	t.Setenv("MY_STATUS_PAGE_URL", "https://status.example.org/")
	t.Setenv("MY_DB", "nosuchdb") // not read with -offline
	dir := filepath.Join(t.TempDir(), "out")
	code, stdout, stderr := runMaintenanceCmd(t, "export", "-out", dir, "-lang", "en,ja", "-offline")
	if code != 0 || stderr != "" {
		t.Fatalf("export = %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	en, ja, index := readFile(t, filepath.Join(dir, "en.html")), readFile(t, filepath.Join(dir, "ja.html")), readFile(t, filepath.Join(dir, "index.html"))
	if !strings.Contains(en, `<html lang="en" dir="ltr">`) || !strings.Contains(ja, `<html lang="ja" dir="ltr">`) || !strings.Contains(ja, "メンテナンス中") {
		t.Errorf("en.html or ja.html is not in its locale:\n%s\n%s", en, ja)
	}
	i, j := strings.Index(index, `<section lang="en"`), strings.Index(index, `<section lang="ja"`)
	if i < 0 || j < i || !strings.Contains(index, "Under maintenance") || !strings.Contains(index, "メンテナンス中") {
		t.Errorf("index.html lacks en then ja:\n%s", index)
	}
	for name, page := range map[string]string{"en.html": en, "ja.html": ja, "index.html": index} {
		if m := externalResource.FindString(page); m != "" {
			t.Errorf("%s loads a resource (%s):\n%s", name, m, page)
		}
		csp := regexp.MustCompile(`<meta http-equiv="Content-Security-Policy" content="([^"]*)">`).FindStringSubmatch(page)
		if csp == nil || strings.Contains(csp[1], "frame-ancestors") || !strings.Contains(csp[1], "style-src &#39;sha256-") {
			t.Errorf("%s has no meta CSP without frame-ancestors: %v", name, csp)
		}
		if !strings.Contains(page, "Moving to a new database.") || !strings.Contains(page, `href="https://status.example.org/"`) {
			t.Errorf("%s lacks the configured notice or status page:\n%s", name, page)
		}
	}

	// The maintenance mode answers with the same page, but for the CSP that its header carries instead.
	cfg, _, err := config.LoadMaintenance(config.Source{})
	if err != nil {
		t.Fatal(err)
	}
	app, err := platform.NewMaintenanceApp(cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	for lang, exported := range map[string]string{"en": en, "ja": ja} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://social.example/", nil)
		req.Header.Set("Accept-Language", lang)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		served, _ := io.ReadAll(rec.Body)
		withoutMeta := regexp.MustCompile(`<meta http-equiv="Content-Security-Policy"[^>]*>\n`).ReplaceAllString(exported, "")
		if string(served) != withoutMeta {
			t.Errorf("%s: the maintenance mode's page differs from the exported one:\n%s\n---\n%s", lang, served, withoutMeta)
		}
	}
}

func TestMaintenanceExportReadsTheDatabase(t *testing.T) {
	newAdminEnv(t)
	t.Setenv("MY_MAINTENANCE_SERVER_NAME", "")
	mustInstance(t, "", "set", "name", "Example Social")
	mustInstance(t, "", "set", "default_locale", "ja")
	mustInstance(t, `{"tokens":[{"token":"color-accent","light":"#123456","dark":"#abcdef"}],"custom_css":"body{display:none}"}`, "theme", "set")
	dir := t.TempDir()
	code, _, stderr := runMaintenanceCmd(t, "export", "-out", dir)
	if code != 0 || stderr != "" {
		t.Fatalf("export = %d, stderr %q", code, stderr)
	}
	page := readFile(t, filepath.Join(dir, "ja.html"))
	for _, want := range []string{`<html lang="ja"`, `<p class="server" dir="auto">Example Social</p>`, "--color-accent:#123456;", "--color-accent:#abcdef;"} {
		if !strings.Contains(page, want) {
			t.Errorf("ja.html lacks %s:\n%s", want, page)
		}
	}
	if strings.Contains(page, "display:none") {
		t.Errorf("ja.html has the theme's custom CSS:\n%s", page)
	}
	if _, err := os.Stat(filepath.Join(dir, "en.html")); err == nil {
		t.Error("export wrote en.html without -lang en")
	}

	// maintenance_server_name wins over the instance's name.
	t.Setenv("MY_MAINTENANCE_SERVER_NAME", "Configured")
	if code, _, stderr := runMaintenanceCmd(t, "export", "-out", dir, "-lang", "en"); code != 0 || stderr != "" {
		t.Fatalf("export = %d, stderr %q", code, stderr)
	}
	if page := readFile(t, filepath.Join(dir, "en.html")); !strings.Contains(page, `<p class="server" dir="auto">Configured</p>`) {
		t.Errorf("en.html does not use maintenance_server_name:\n%s", page)
	}
}

func TestMaintenanceExportWithoutTheDatabase(t *testing.T) {
	newAdminEnv(t)
	t.Setenv("MY_DATABASE_URL", filepath.Join(t.TempDir(), "missing", "my.db"))
	dir := t.TempDir()
	code, _, stderr := runMaintenanceCmd(t, "export", "-out", dir, "-lang", "en")
	if code != 0 || !strings.Contains(stderr, "warning: the database cannot be read") {
		t.Fatalf("export = %d, stderr %q; want 0 and a warning", code, stderr)
	}
	if page := readFile(t, filepath.Join(dir, "en.html")); !strings.Contains(page, `<p class="server" dir="auto">social.example</p>`) {
		t.Errorf("en.html does not fall back to the host:\n%s", page)
	}
}

func TestMaintenanceArguments(t *testing.T) {
	t.Setenv("MY_HOST", "social.example")
	for _, tt := range []struct {
		args []string
		code int
		want string
	}{
		{nil, 2, "Usage: my maintenance export"},
		{[]string{"import"}, 2, `unknown subcommand "import"`},
		{[]string{"export", "-offline"}, 2, "-out is required"},
		{[]string{"export", "-out", t.TempDir(), "-lang", "xx", "-offline"}, 2, `unknown locale "xx"`},
		{[]string{"export", "-out", t.TempDir(), "extra"}, 2, `unexpected argument "extra"`},
	} {
		if code, _, stderr := runMaintenanceCmd(t, tt.args...); code != tt.code || !strings.Contains(stderr, tt.want) {
			t.Errorf("maintenance %q = %d %q, want %d and %q", tt.args, code, stderr, tt.code, tt.want)
		}
	}
}
