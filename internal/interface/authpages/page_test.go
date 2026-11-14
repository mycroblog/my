// SPDX-License-Identifier: AGPL-3.0-only

package authpages_test

import (
	"context"
	"encoding/json"
	"html"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/authpages"
)

// dataBlock is the JSON data block of a screen's document.
var dataBlock = regexp.MustCompile(`<script type="application/json" id="auth">([^<]*)</script>`)

// screenData returns the data of the screen that w answered with, the JSON that the entry reads.
func screenData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	m := dataBlock.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("the answer has no data block:\n%s", w.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		t.Fatalf("the data block is not JSON: %v\n%s", err, m[1])
	}
	return data
}

// at returns the value at the dotted path of data ("signUp.errors.username"), with numbers for
// the elements of an array ("consent.scopeGroups.0.risk"), or nil when there is none.
func at(data map[string]any, path string) any {
	var v any = data
	for _, key := range strings.Split(path, ".") {
		switch c := v.(type) {
		case map[string]any:
			v = c[key]
		case []any:
			i := 0
			for _, r := range key {
				if r < '0' || r > '9' {
					return nil
				}
				i = i*10 + int(r-'0')
			}
			if i >= len(c) {
				return nil
			}
			v = c[i]
		default:
			return nil
		}
	}
	return v
}

// text returns the string at path, "" when there is none.
func text(data map[string]any, path string) string {
	s, _ := at(data, path).(string)
	return s
}

func newEntryServer(t *testing.T, o authpages.Options) http.Handler {
	t.Helper()
	b, err := i18n.Load(i18n.WithPseudoLocales())
	if err != nil {
		t.Fatal(err)
	}
	if o.Identity == nil {
		o.Identity = newFakeIdentity()
	}
	o.Locales = &i18n.Resolver{Bundle: b}
	m := mux{http.NewServeMux()}
	authpages.New(o).Register(m)
	return m
}

var entry = authpages.Entry{
	Script: "/assets/auth-AAAA.js", Stylesheets: []string{"/assets/auth-BBBB.css"}, Preloads: []string{"/assets/shared-CCCC.js"},
}

// TestScreenIsADocumentOfTheEntry checks that a screen is the auth entry's shell: its tags, the
// screen's data in the JSON data block, the locale on <html>, the server's name as its title,
// and no markup of the screen itself, which the entry renders.
func TestScreenIsADocumentOfTheEntry(t *testing.T) {
	h := newEntryServer(t, authpages.Options{
		Entry: entry, InstanceName: func(context.Context) string { return "例の<サーバー>" },
		ThemeStylesheet: func(context.Context) string { return "/theme/1.css" },
	})
	w := newBrowser(t, h).do(http.MethodGet, authpages.PathSignIn, nil, map[string]string{"Accept-Language": "ar-XB"})
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
		w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Language") != "ar-XB" {
		t.Fatalf("GET sign_in = %d %v", w.Code, w.Header())
	}
	body := w.Body.String()
	for _, want := range []string{
		`<html lang="ar-XB" dir="rtl">`,
		`<title>例の&lt;サーバー&gt;</title>`,
		// The entry's stylesheets, then the theme's, which overrides their tokens.
		`<link rel="stylesheet" href="/assets/auth-BBBB.css">` + "\n" + `<link rel="stylesheet" href="/theme/1.css">`,
		`<link rel="modulepreload" href="/assets/shared-CCCC.js">`,
		`<script type="module" src="/assets/auth-AAAA.js"></script>`,
		`<!--email_off--><script type="application/json" id="auth">`,
		`<div id="root"></div>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the screen lacks %s:\n%s", want, body)
		}
	}
	for _, bad := range []string{"<form", "<input", "<label", "<h1", " style=", "<style", "<main"} {
		if strings.Contains(body, bad) {
			t.Errorf("the screen is rendered by the server (%s):\n%s", bad, body)
		}
	}
	data := screenData(t, w)
	if text(data, "screen") != "sign_in" || text(data, "server.name") != "例の<サーバー>" {
		t.Errorf("data = %v, want the sign-in screen of the server", data)
	}
	if csrf := text(data, "csrfToken"); csrf == "" || csrf != newBrowserCookie(w, authpages.CookieCSRF) {
		t.Errorf("csrfToken = %q, want the CSRF cookie's token", csrf)
	}
}

// newBrowserCookie returns the value of the cookie name that w set.
func newBrowserCookie(w *httptest.ResponseRecorder, name string) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// TestNoScreenTemplates checks that the package renders no screen with html/template: it has no
// templates directory, and the shell holds no element of a screen whatever the screen.
func TestNoScreenTemplates(t *testing.T) {
	if _, err := os.Stat("templates"); !os.IsNotExist(err) {
		t.Errorf("the package still has its templates directory (%v)", err)
	}
	entries, err := fs.Glob(os.DirFS("."), "*.html")
	if err != nil || len(entries) > 0 {
		t.Errorf("the package has HTML files %v (%v)", entries, err)
	}
}

// TestDataBlockIsEscaped is a regression test: what users and administrators write (an instance
// name, an email address typed in a form) reaches the data block as JSON that cannot end the
// script element.
func TestDataBlockIsEscaped(t *testing.T) {
	h := newEntryServer(t, authpages.Options{
		Entry: entry, InstanceName: func(context.Context) string { return `</script><script>alert(1)</script>` },
	})
	b := newBrowser(t, h)
	page := b.do(http.MethodGet, authpages.PathSignIn, nil, nil)
	w := b.do(http.MethodPost, authpages.PathSignIn, b.form(page, "email", `x</script><img src=x onerror=alert(1)>@example.com`, "password", "wrong"), nil)
	body := w.Body.String()
	if strings.Count(body, "</script>") != 2 || strings.Contains(body, "<img") {
		t.Errorf("the data block is not escaped:\n%s", body)
	}
	data := screenData(t, w)
	if text(data, "server.name") != `</script><script>alert(1)</script>` || !strings.HasPrefix(text(data, "email"), "x</script>") {
		t.Errorf("data = %v, want the values as they were", data)
	}
	if html.UnescapeString(text(data, "server.name")) != text(data, "server.name") {
		t.Error("the data block's strings are HTML-escaped twice")
	}
}

// TestScreenCSP checks the policy of a screen (ADR-0028 §4): the entry from this site and nothing
// else, no inline script or style, and what a dev server, the media origin, the CAPTCHA, and the
// consent's redirect add.
func TestScreenCSP(t *testing.T) {
	csp := func(o authpages.Options) string {
		o.Entry.Script = "/assets/auth.js"
		return newBrowser(t, newEntryServer(t, o)).do(http.MethodGet, authpages.PathSignIn, nil, nil).Header().Get("Content-Security-Policy")
	}
	if got, want := csp(authpages.Options{FormActionOrigins: []string{"https://web.example"}}),
		"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; font-src 'self'; "+
			"form-action 'self' https://web.example; frame-ancestors 'none'; base-uri 'none'; object-src 'none'"; got != want {
		t.Errorf("CSP = %q, want %q", got, want)
	}
	if got := csp(authpages.Options{MediaOrigin: "https://media.example"}); !strings.Contains(got, "img-src 'self' https://media.example;") {
		t.Errorf("CSP with a media origin = %q", got)
	}
	dev := csp(authpages.Options{Entry: authpages.Entry{DevOrigin: "http://localhost:5173"}})
	for _, want := range []string{
		"script-src 'self' http://localhost:5173;", "style-src 'self' 'unsafe-inline' http://localhost:5173;",
		"img-src 'self' http://localhost:5173;", "connect-src 'self' http://localhost:5173 ws://localhost:5173;",
	} {
		if !strings.Contains(dev, want) {
			t.Errorf("dev CSP %q lacks %s", dev, want)
		}
	}
	widget := csp(authpages.Options{HumanCheck: &kernel.HumanCheckWidget{
		Service: kernel.HumanCheckTurnstile, Name: "Cloudflare Turnstile", SiteKey: "k",
		CSP: kernel.HumanCheckCSP{ScriptSrc: []string{"https://challenges.cloudflare.com"}, FrameSrc: []string{"https://challenges.cloudflare.com"}},
	}})
	for _, want := range []string{"script-src 'self' https://challenges.cloudflare.com;", "frame-src https://challenges.cloudflare.com;"} {
		if !strings.Contains(widget, want) {
			t.Errorf("CSP with the widget %q lacks %s", widget, want)
		}
	}
	for _, p := range []string{dev, widget} {
		if strings.Contains(p, "unsafe-eval") || strings.Contains(p, "nonce-") {
			t.Errorf("CSP %q allows eval or a nonce", p)
		}
	}
}

// TestScreenShowsTheServer checks that every screen carries what it shows of the server: its name,
// its logos, its summary and its banner, and the footer's documents and status page.
func TestScreenShowsTheServer(t *testing.T) {
	h := newEntryServer(t, authpages.Options{
		Entry:               entry,
		InstanceName:        func(context.Context) string { return "Example" },
		Logos:               func(context.Context) (string, string) { return "/brand/1/logo.svg", "/brand/2/logo.svg" },
		InstanceDescription: func(context.Context) string { return "  A small server.\n" },
		Banner:              func(context.Context) string { return "https://media.example/banner.png" },
		Documents:           &fakeDocuments{docs: map[instance.TextKey]appinstance.Document{instance.TextTerms: {}}},
		StatusPageURL:       "https://status.example",
	})
	for _, path := range []string{authpages.PathSignIn, authpages.PathSignUp} {
		data := screenData(t, newBrowser(t, h).do(http.MethodGet, path, nil, nil))
		for key, want := range map[string]string{
			"server.name": "Example", "server.logo": "/brand/1/logo.svg", "server.logoDark": "/brand/2/logo.svg",
			"server.description": "A small server.", "server.banner": "https://media.example/banner.png",
			"documents.0.path": authpages.PathTerms, "statusPageUrl": "https://status.example",
		} {
			if got := text(data, key); got != want {
				t.Errorf("GET %s: %s = %q, want %q", path, key, got, want)
			}
		}
	}
}
