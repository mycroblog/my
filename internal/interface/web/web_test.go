// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mycroblog/my/internal/interface/web"
)

// fakeBuild is a Vite build with an entry that imports a shared chunk, CSS on both, a
// lazily loaded chunk that must not be preloaded, and the server pages' stylesheet entry.
func fakeBuild() fstest.MapFS {
	return fstest.MapFS{
		".vite/manifest.json": {Data: []byte(`{
			"index.html": {"file": "assets/index-AAAA.js", "src": "index.html", "isEntry": true,
				"imports": ["_shared-BBBB.js"], "dynamicImports": ["src/lazy.tsx"], "css": ["assets/index-CCCC.css"]},
			"_shared-BBBB.js": {"file": "assets/shared-BBBB.js", "css": ["assets/shared-DDDD.css"]},
			"src/lazy.tsx": {"file": "assets/lazy-EEEE.js", "src": "src/lazy.tsx", "isDynamicEntry": true},
			"src/server-pages.css": {"file": "assets/server-pages-FFFF.css", "src": "src/server-pages.css", "isEntry": true},
			"src/server-pages/passkey-sign-in.ts": {"file": "assets/server-pages-passkey-GGGG.js", "src": "src/server-pages/passkey-sign-in.ts", "isEntry": true}
		}`)},
		"index.html":                   {Data: []byte("<!doctype html><title>vite</title>")},
		"assets/index-AAAA.js":         {Data: []byte("console.log('entry')")},
		"assets/shared-BBBB.js":        {Data: []byte("export {}")},
		"assets/index-CCCC.css":        {Data: []byte("body{}")},
		"assets/shared-DDDD.css":       {Data: []byte("p{}")},
		"assets/lazy-EEEE.js":          {Data: []byte("export {}")},
		"assets/server-pages-FFFF.css": {Data: []byte("main{}")},
		"icon.svg":                     {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")},
		"sw.js":                        {Data: []byte("self.addEventListener('fetch', () => {})")},
		"theme.js":                     {Data: []byte("document.documentElement.dataset.theme = 'dark'")},
		"licenses/npm-packages.md":     {Data: []byte("# Licenses\n\n## react - 19.3.0 (MIT)\n")},
		"licenses/LICENSE-Unicode.txt": {Data: []byte("UNICODE LICENSE V3\n")},
		// The PNG icons except icon-512.png, which the manifest then leaves out.
		"icon-192.png":          {Data: []byte("png 192")},
		"icon-maskable-512.png": {Data: []byte("png maskable")},
		"apple-touch-icon.png":  {Data: []byte("png apple")},
	}
}

func locale(l string) func(context.Context) string {
	return func(context.Context) string { return l }
}

// named names the instance n in every locale.
func named(n string) func(context.Context) string {
	return func(context.Context) string { return n }
}

func newHandler(t *testing.T, o web.Options) (*web.Handler, *http.ServeMux) {
	t.Helper()
	if o.Build == nil {
		o.Build = fakeBuild()
	}
	if o.Host == "" {
		o.Host = "social.example"
	}
	h, err := web.New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	mux.Handle("GET /", h)
	return h, mux
}

func get(mux http.Handler, target string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestShellLoadsTheBuildFromTheManifest(t *testing.T) {
	_, mux := newHandler(t, web.Options{InstanceName: named("Example Social")})
	rec := get(mux, "/@alice/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("shell = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<html lang="en" dir="ltr">`,
		`<title>Example Social</title>`,
		`<link rel="manifest" href="/manifest.webmanifest">`,
		`<link rel="icon" href="/icon.svg" type="image/svg+xml">`,
		`<link rel="apple-touch-icon" href="/apple-touch-icon.png">`,
		// The stored color theme applies before the first paint: a classic script, before the stylesheets.
		`<script src="/theme.js"></script>` + "\n" + `<link rel="stylesheet" href="/assets/index-CCCC.css">`,
		`<link rel="stylesheet" href="/assets/index-CCCC.css">` + "\n" + `<link rel="stylesheet" href="/assets/shared-DDDD.css">`,
		`<link rel="modulepreload" href="/assets/shared-BBBB.js">`,
		`<script type="module" src="/assets/index-AAAA.js"></script>`,
		`<div id="root"></div>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "lazy-EEEE") || strings.Contains(body, "vite</title>") || strings.Contains(body, "server-pages") {
		t.Errorf("shell preloads a lazy chunk, loads the server pages' stylesheet, or is Vite's index.html:\n%s", body)
	}
	h := rec.Header()
	if h.Get("Cache-Control") != "public, max-age=60, stale-while-revalidate=300" || h.Get("Vary") != "Accept" || h.Get("ETag") == "" {
		t.Errorf("headers = %v, want short public caching, Vary: Accept and an ETag", h)
	}

	// Every route gets the same shell, so a conditional request revalidates it.
	again := get(mux, "/home", map[string]string{"If-None-Match": h.Get("ETag")})
	if again.Code != http.StatusNotModified {
		t.Errorf("conditional shell = %d, want 304", again.Code)
	}
}

func TestStylesheetIsTheServerPagesEntry(t *testing.T) {
	h, mux := newHandler(t, web.Options{})
	if got := h.Stylesheet(); got != "/assets/server-pages-FFFF.css" {
		t.Fatalf("Stylesheet() = %q, want the server pages' entry", got)
	}
	if got := h.PasskeySignInScript(); got != "/assets/server-pages-passkey-GGGG.js" {
		t.Errorf("PasskeySignInScript() = %q, want the passkey entry", got)
	}
	if rec := get(mux, h.Stylesheet(), nil); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Errorf("GET %s = %d %q", h.Stylesheet(), rec.Code, rec.Header().Get("Content-Type"))
	}

	build := fakeBuild()
	build[".vite/manifest.json"] = &fstest.MapFile{Data: []byte(`{"index.html": {"file": "assets/index-AAAA.js", "isEntry": true}}`)}
	without, _ := newHandler(t, web.Options{Build: build})
	if got := without.Stylesheet(); got != "" {
		t.Errorf("Stylesheet() without the entry = %q, want empty", got)
	}
}

// TestShellCSP checks that the shell runs under the nonce-free policy of ADR-0019 §9: every
// script is an external file from the server's own origin, apart from the JSON data block,
// which is not executed.
func TestShellCSP(t *testing.T) {
	_, mux := newHandler(t, web.Options{})
	rec := get(mux, "/", nil)
	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'none'", "script-src 'self';", "style-src 'self';", "object-src 'none'", "base-uri 'none'", "frame-ancestors 'none'", "require-trusted-types-for 'script'",
		// Only the client's own Trusted Types policies, each created once (web-client/09-app-shell.md §9).
		"trusted-types my-server-sanitized-html my-service-worker;"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %s", csp, want)
		}
	}
	for _, bad := range []string{"nonce-", "'unsafe-inline'", "'unsafe-eval'", "sha256-", "http:", "https:", "'allow-duplicates'"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP %q contains %s", csp, bad)
		}
	}
	body := rec.Body.String()
	scripts := regexp.MustCompile(`<script([^>]*)>([^<]*)</script>`).FindAllStringSubmatch(body, -1)
	if len(scripts) != 3 {
		t.Fatalf("shell has %d scripts, want the theme script, the data block and the entry:\n%s", len(scripts), body)
	}
	for _, s := range scripts {
		attrs, content := s[1], s[2]
		switch {
		case attrs == ` type="application/json" id="config"`:
		case attrs == ` src="/theme.js"` && content == "":
		case strings.HasPrefix(attrs, ` type="module" src="/`) && !strings.HasPrefix(attrs, ` type="module" src="//`) && content == "":
		default:
			t.Errorf("script %q would need a nonce or a remote origin under %q", s[0], csp)
		}
	}
	if strings.Contains(body, " style=") || strings.Contains(body, "<style") || regexp.MustCompile(` on[a-z]+=`).MatchString(body) {
		t.Errorf("shell has inline styles or event handlers:\n%s", body)
	}
}

// TestRuntimeConfigIsEscaped is a regression test: instance settings reach the shell's JSON
// data block through html/template, which must keep them from ending the script element.
func TestRuntimeConfigIsEscaped(t *testing.T) {
	name := `</script><script>alert(1)</script><!-- "quoted" & 'single' \ ` + "\u2028"
	_, mux := newHandler(t, web.Options{InstanceName: named(name), Host: "social.example", AccountDomain: "example.org", DefaultLocale: locale("ja")})
	body := get(mux, "/", nil).Body.String()
	if strings.Count(body, "</script>") != 3 || strings.Contains(body, "<script>alert") || strings.Contains(body, "<!--") {
		t.Fatalf("the instance name escaped the data block:\n%s", body)
	}
	m := regexp.MustCompile(`<script type="application/json" id="config">([^<]*)</script>`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no data block:\n%s", body)
	}
	var cfg map[string]string
	if err := json.Unmarshal([]byte(m[1]), &cfg); err != nil {
		t.Fatalf("data block %q is not JSON: %v", m[1], err)
	}
	want := map[string]string{"apiOrigin": "https://social.example", "instanceName": name, "accountDomain": "example.org", "defaultLocale": "ja"}
	for k, v := range want {
		if cfg[k] != v {
			t.Errorf("config[%q] = %q, want %q", k, cfg[k], v)
		}
	}
	if !strings.Contains(body, "<title>&lt;/script&gt;&lt;script&gt;alert(1)") {
		t.Errorf("title is not HTML-escaped:\n%s", body)
	}
}

// TestShellAPIOrigin checks that the shell's client calls the API at the origin the server is served
// at: the web origin when it is on the server's host, https://host otherwise.
func TestShellAPIOrigin(t *testing.T) {
	for _, c := range []struct{ host, webOrigin, want string }{
		{"social.example", "", "https://social.example"},
		{"social.example", "https://social.example", "https://social.example"},
		{"localhost:8080", "http://localhost:8080", "http://localhost:8080"},
		{"localhost:8080", "http://LOCALHOST:8080", "http://localhost:8080"},
		// A client served from elsewhere says nothing about the server's scheme.
		{"social.example", "http://localhost:5173", "https://social.example"},
		{"localhost:8080", "http://localhost:5173", "https://localhost:8080"},
	} {
		_, mux := newHandler(t, web.Options{Host: c.host, WebOrigin: c.webOrigin})
		if body := get(mux, "/", nil).Body.String(); !strings.Contains(body, `"apiOrigin":"`+c.want+`"`) {
			t.Errorf("host %q, web origin %q: shell lacks apiOrigin %q:\n%s", c.host, c.webOrigin, c.want, body)
		}
	}
	// Requests are upgraded to HTTPS only when the server is served over HTTPS: WebKit upgrades
	// http://localhost too, which breaks a plain-HTTP setup on localhost.
	for _, c := range []struct {
		host, webOrigin string
		upgrade         bool
	}{
		{"social.example", "", true},
		{"localhost:8080", "http://localhost:5173", true},
		{"localhost:8080", "http://localhost:8080", false},
	} {
		_, mux := newHandler(t, web.Options{Host: c.host, WebOrigin: c.webOrigin})
		csp := get(mux, "/", nil).Header().Get("Content-Security-Policy")
		if got := strings.HasSuffix(csp, "; upgrade-insecure-requests"); got != c.upgrade {
			t.Errorf("host %q, web origin %q: CSP %q upgrades insecure requests: %v, want %v", c.host, c.webOrigin, csp, got, c.upgrade)
		}
	}
	for _, bad := range []string{"localhost:8080", "ftp://localhost:8080", "http://"} {
		if _, err := web.New(web.Options{Host: "localhost:8080", Build: fakeBuild(), WebOrigin: bad}); err == nil {
			t.Errorf("New accepted the web origin %q", bad)
		}
	}
}

func TestShellLanguageAndDirection(t *testing.T) {
	current := "ar"
	_, mux := newHandler(t, web.Options{DefaultLocale: func(context.Context) string { return current }})
	ar := get(mux, "/", map[string]string{"Accept-Language": "ja"})
	if body := ar.Body.String(); !strings.Contains(body, `<html lang="ar" dir="rtl">`) {
		t.Errorf("shell for an Arabic instance:\n%s", body)
	}
	// A change of the default locale applies to the next request, with a new ETag.
	current = "ja"
	ja := get(mux, "/", nil)
	if body := ja.Body.String(); !strings.Contains(body, `<html lang="ja" dir="ltr">`) || !strings.Contains(body, `"defaultLocale":"ja"`) {
		t.Errorf("shell after the default locale changed:\n%s", body)
	}
	if ja.Header().Get("ETag") == ar.Header().Get("ETag") {
		t.Errorf("the ETag did not change with the locale")
	}
}

func TestReservedPathsAreNotTheShell(t *testing.T) {
	var notFound []string
	_, mux := newHandler(t, web.Options{NotFound: func(w http.ResponseWriter, r *http.Request) {
		notFound = append(notFound, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}})
	reserved := []string{"/api", "/api/v1/nope", "/oauth/authorize", "/auth/login", "/.well-known/nope", "/nodeinfo", "/nodeinfo/2.0", "/ap/nope", "/media/x.png", "/uploads/x", "/licenses", "/licenses/a/b.txt"}
	for _, p := range reserved {
		if rec := get(mux, p, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	if len(notFound) != len(reserved) {
		t.Errorf("NotFound served %q, want %q", notFound, reserved)
	}
	for _, p := range []string{"/apple", "/application/x", "/authors", "/mediathek", "/settings/profile", "/tags/go"} {
		if rec := get(mux, p, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="root"`) {
			t.Errorf("GET %s = %d, want the shell", p, rec.Code)
		}
	}
}

func TestStaticFiles(t *testing.T) {
	_, mux := newHandler(t, web.Options{})
	tests := []struct {
		path, cache, contentType, body string
	}{
		{"/assets/index-AAAA.js", "public, max-age=31536000, immutable", "text/javascript; charset=utf-8", "console.log('entry')"},
		{"/assets/index-CCCC.css", "public, max-age=31536000, immutable", "text/css; charset=utf-8", "body{}"},
		{"/sw.js", "no-cache", "text/javascript; charset=utf-8", "self.addEventListener('fetch', () => {})"},
		{"/icon.svg", "no-cache", "image/svg+xml", "<svg xmlns='http://www.w3.org/2000/svg'/>"},
		{"/theme.js", "no-cache", "text/javascript; charset=utf-8", "document.documentElement.dataset.theme = 'dark'"},
		// License notices are plain text whatever their extension, so a browser shows Markdown as it is.
		{"/licenses/npm-packages.md", "no-cache", "text/plain; charset=utf-8", "# Licenses\n\n## react - 19.3.0 (MIT)\n"},
		{"/licenses/LICENSE-Unicode.txt", "no-cache", "text/plain; charset=utf-8", "UNICODE LICENSE V3\n"},
	}
	for _, tt := range tests {
		rec := get(mux, tt.path, nil)
		if rec.Code != http.StatusOK || rec.Body.String() != tt.body {
			t.Errorf("GET %s = %d %q, want %q", tt.path, rec.Code, rec.Body, tt.body)
			continue
		}
		h := rec.Header()
		if h.Get("Cache-Control") != tt.cache || h.Get("Content-Type") != tt.contentType || h.Get("ETag") == "" {
			t.Errorf("GET %s headers = %v, want Cache-Control %q, Content-Type %q and an ETag", tt.path, h, tt.cache, tt.contentType)
		}
		if rec := get(mux, tt.path, map[string]string{"If-None-Match": h.Get("ETag")}); rec.Code != http.StatusNotModified {
			t.Errorf("conditional GET %s = %d, want 304", tt.path, rec.Code)
		}
	}
	// The Service Worker runs under its own response's policy, which must let it fetch from the origin.
	if csp := get(mux, "/sw.js", nil).Header().Get("Content-Security-Policy"); csp != "default-src 'none'; connect-src 'self'" {
		t.Errorf("sw.js Content-Security-Policy = %q, want fetches from the origin only", csp)
	}
	for _, p := range []string{"/assets/missing.js", "/assets/", "/assets/.vite/manifest.json", "/licenses", "/licenses/", "/licenses/missing.txt", "/licenses/sw.js"} {
		if rec := get(mux, p, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	// Vite's index.html and the build manifest are never served as files.
	for _, p := range []string{"/index.html", "/.vite/manifest.json"} {
		if rec := get(mux, p, nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="config"`) {
			t.Errorf("GET %s = %d %q, want the shell", p, rec.Code, rec.Body)
		}
	}
}

func TestWebManifest(t *testing.T) {
	_, mux := newHandler(t, web.Options{InstanceName: named("Example Social"), DefaultLocale: locale("ar")})
	rec := get(mux, web.PathWebManifest, nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/manifest+json" || rec.Header().Get("ETag") == "" {
		t.Fatalf("manifest = %d %v", rec.Code, rec.Header())
	}
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") || cc == "" {
		t.Errorf("manifest Cache-Control = %q, want short caching", cc)
	}
	var m struct {
		ID, Name, Lang, Dir, Display string
		ShortName                    string `json:"short_name"`
		StartURL                     string `json:"start_url"`
		Scope                        string
		ThemeColor                   string `json:"theme_color"`
		BackgroundColor              string `json:"background_color"`
		Icons                        []struct{ Src, Sizes, Type, Purpose string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "Example Social" || m.ShortName != "Example Social" || m.Lang != "ar" || m.Dir != "rtl" ||
		m.StartURL != "/" || m.Scope != "/" || m.ID != "/" || m.Display != "standalone" ||
		m.ThemeColor != defaultSurface.Light || m.BackgroundColor != defaultSurface.Light {
		t.Errorf("manifest = %+v", m)
	}
	type icon = struct{ Src, Sizes, Type, Purpose string }
	want := []icon{
		{"/icon.svg", "any", "image/svg+xml", "any"},
		{"/icon-192.png", "192x192", "image/png", "any"},
		{"/icon-maskable-512.png", "512x512", "image/png", "maskable"},
	}
	if !slices.Equal(m.Icons, want) {
		t.Errorf("icons = %+v, want the icons the build has: %+v", m.Icons, want)
	}
}

// TestInstanceName checks that the shell and the manifest name the instance by its display name,
// read per request so that a change applies at once, and by its host when it has none; the
// handle domain stays in the runtime config either way.
func TestInstanceName(t *testing.T) {
	var name string
	_, mux := newHandler(t, web.Options{
		Host: "social.example", AccountDomain: "example.org", DefaultLocale: locale("ja"),
		InstanceName: func(context.Context) string { return name },
	})
	check := func(want string) {
		t.Helper()
		body := get(mux, "/", nil).Body.String()
		if !strings.Contains(body, "<title>"+want+"</title>") {
			t.Errorf("shell title, want %q:\n%s", want, body)
		}
		m := regexp.MustCompile(`<script type="application/json" id="config">([^<]*)</script>`).FindStringSubmatch(body)
		var cfg map[string]string
		if m == nil || json.Unmarshal([]byte(m[1]), &cfg) != nil || cfg["instanceName"] != want || cfg["accountDomain"] != "example.org" {
			t.Errorf("runtime config = %v, want the name %q and the account domain", cfg, want)
		}
		var manifest struct {
			Name      string
			ShortName string `json:"short_name"`
		}
		if err := json.Unmarshal(get(mux, web.PathWebManifest, nil).Body.Bytes(), &manifest); err != nil || manifest.Name != want || manifest.ShortName != want {
			t.Errorf("manifest = %+v (%v), want the name %q", manifest, err, want)
		}
	}
	check("social.example")
	name = "Example Social"
	check("Example Social")
	name = "例のサーバー"
	check("例のサーバー")
}

// TestWithoutBuild checks the binary that Go builds without `scripts/task.sh web:build`: it starts,
// warns, and serves a shell that loads nothing.
func TestWithoutBuild(t *testing.T) {
	var logs bytes.Buffer
	build := fstest.MapFS{".gitkeep": {}}
	_, mux := newHandler(t, web.Options{Build: build, Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	rec := get(mux, "/", nil)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `type="module"`) || !strings.Contains(rec.Body.String(), `id="config"`) {
		t.Errorf("shell without a build = %d:\n%s", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), "not built") {
		t.Errorf("no warning about the missing build: %q", logs.String())
	}
	if rec := get(mux, "/.gitkeep", nil); !strings.Contains(rec.Body.String(), `id="config"`) {
		t.Errorf("GET /.gitkeep = %q, want the shell", rec.Body)
	}

	// The embedded build is the placeholder, or the build of `scripts/task.sh web:build`.
	if _, err := web.New(web.Options{Host: "social.example", Build: web.Embedded()}); err != nil {
		t.Errorf("New on the embedded build: %v", err)
	}
}

func TestBrokenManifest(t *testing.T) {
	for name, manifest := range map[string]string{
		"not JSON":      `{`,
		"no entry":      `{"src/main.tsx": {"file": "assets/main.js", "isEntry": true}}`,
		"missing chunk": `{"index.html": {"file": "assets/index.js", "isEntry": true, "imports": ["_gone.js"]}}`,
	} {
		_, err := web.New(web.Options{Host: "social.example", Build: fstest.MapFS{".vite/manifest.json": {Data: []byte(manifest)}}})
		if err == nil {
			t.Errorf("%s: New succeeded", name)
		}
	}
}

func TestDevViteOrigin(t *testing.T) {
	_, mux := newHandler(t, web.Options{DevViteOrigin: "http://localhost:5173", Host: "localhost:8080"})
	rec := get(mux, "/", nil)
	body := rec.Body.String()
	for _, want := range []string{
		`<script type="module" src="http://localhost:5173/@vite/client"></script>` + "\n" + `<script type="module" src="http://localhost:5173/src/main.tsx"></script>`,
		`"apiOrigin":"http://localhost:8080"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dev shell lacks %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "/assets/") {
		t.Errorf("dev shell loads the embedded build:\n%s", body)
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self' http://localhost:5173;") || !strings.Contains(csp, "ws://localhost:5173") {
		t.Errorf("dev CSP = %q, want the dev server's scripts and WebSocket", csp)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("dev shell Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	for _, bad := range []string{"localhost:5173/", "ftp://localhost", "localhost:5173", "http://user@localhost:5173"} {
		if _, err := web.New(web.Options{Host: "localhost:8080", Build: fakeBuild(), DevViteOrigin: bad}); err == nil {
			t.Errorf("dev origin %q accepted", bad)
		}
	}
}

// TestShellCSPMediaOrigin checks that the policy lets images and media load from the CDN origin of
// the media files, and from nowhere else, and that it is unchanged when they are on the server.
func TestShellCSPMediaOrigin(t *testing.T) {
	csp := func(o web.Options) string {
		_, mux := newHandler(t, o)
		return get(mux, "/", nil).Header().Get("Content-Security-Policy")
	}
	base := csp(web.Options{})
	for _, same := range []string{"", "https://social.example"} {
		if got := csp(web.Options{MediaOrigin: same}); got != base {
			t.Errorf("media origin %q: CSP = %q, want it unchanged %q", same, got, base)
		}
	}
	got := csp(web.Options{MediaOrigin: "https://media.example"})
	for _, want := range []string{"img-src 'self' blob: https://media.example;", "media-src 'self' blob: https://media.example;"} {
		if !strings.Contains(got, want) {
			t.Errorf("CSP %q lacks %s", got, want)
		}
	}
	if strings.Count(got, "media.example") != 2 {
		t.Errorf("CSP %q names the media origin outside img-src and media-src", got)
	}
	if _, err := web.New(web.Options{Host: "social.example", Build: fakeBuild(), MediaOrigin: "media.example"}); err == nil {
		t.Error("New accepted a media origin without a scheme")
	}
}
