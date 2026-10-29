// SPDX-License-Identifier: AGPL-3.0-only

package maintenance_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/maintenance"
)

var now = time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)

func newHandler(t *testing.T, p maintenance.Page) http.Handler {
	t.Helper()
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	return maintenance.New(maintenance.Options{
		Page: p, RetryAfter: 600 * time.Second, Locales: &i18n.Resolver{Bundle: b}, Now: func() time.Time { return now },
	})
}

func testPage() maintenance.Page {
	return maintenance.Page{
		ServerName: "social.example", Message: "Upgrading <the> database.", Until: now.Add(2 * time.Hour),
		StatusPageURL: "https://status.example.org/",
		Theme: maintenance.Theme{
			Light: map[string]string{"color-surface": "#fcf9f6", "color-fg": "#2e2e2e", "font-sans": `"Noto Sans", sans-serif`},
			Dark:  map[string]string{"color-surface": "#241f20", "color-fg": "#f5ebea"},
		},
	}
}

func serve(h http.Handler, method, target string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://social.example"+target, strings.NewReader("{}"))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestResponses checks every row of the table of ADR-0084 §3.
func TestResponses(t *testing.T) {
	h := newHandler(t, testPage())
	const statusBody = `{"status":"maintenance"}` + "\n"
	const shortBody = `{"error":"maintenance"}` + "\n"
	const problem, page = "problem", "page"
	ap := map[string]string{"Accept": "application/activity+json"}
	for _, tt := range []struct {
		name, method, target string
		header               map[string]string
		code                 int
		contentType, body    string
	}{
		{"liveness", http.MethodGet, "/healthz", nil, 200, "application/json", statusBody},
		{"readiness", http.MethodGet, "/readyz", nil, 200, "application/json", statusBody},
		{"instance health", http.MethodGet, "/api/v1/instance/health", nil, 503, "application/json", statusBody},
		{"client API", http.MethodGet, "/api/v1/timelines/home", nil, 503, "application/problem+json", problem},
		{"client API POST", http.MethodPost, "/api/v1/statuses", map[string]string{"Content-Type": "application/json"}, 503, "application/problem+json", problem},
		{"OAuth token", http.MethodPost, "/oauth/token", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, 503, "application/problem+json", problem},
		{"OAuth refresh", http.MethodPost, "/oauth/token/refresh", nil, 503, "application/problem+json", problem},
		{"OAuth metadata", http.MethodGet, "/.well-known/oauth-authorization-server", nil, 503, "application/problem+json", problem},
		{"actor GET by path", http.MethodGet, "/ap/users/01J", ap, 503, "application/json", shortBody},
		{"actor GET by Accept", http.MethodGet, "/@alice", ap, 503, "application/json", shortBody},
		{"object GET by ld+json", http.MethodGet, "/@alice/01J", map[string]string{"Accept": `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`}, 503, "application/json", shortBody},
		{"shared inbox POST", http.MethodPost, "/ap/inbox", map[string]string{"Content-Type": "application/activity+json"}, 503, "application/json", shortBody},
		{"inbox POST", http.MethodPost, "/inbox", map[string]string{"Content-Type": "application/activity+json"}, 503, "application/json", shortBody},
		{"WebFinger", http.MethodGet, "/.well-known/webfinger?resource=acct:alice@social.example", nil, 503, "application/json", shortBody},
		{"host-meta", http.MethodGet, "/.well-known/host-meta", nil, 503, "application/json", shortBody},
		{"NodeInfo index", http.MethodGet, "/.well-known/nodeinfo", nil, 503, "application/json", shortBody},
		{"NodeInfo", http.MethodGet, "/nodeinfo/2.1", nil, 503, "application/json", shortBody},
		{"payment webhook", http.MethodPost, "/payments/webhooks/stripe", nil, 503, "application/json", shortBody},
		{"job signal of the queue", http.MethodPost, "/_internal/jobs", nil, 503, "application/json", shortBody},
		{"tick of the scheduler", http.MethodPost, "/_internal/tick", nil, 503, "application/json", shortBody},
		{"HTML GET", http.MethodGet, "/@alice", map[string]string{"Accept": "text/html,application/xhtml+xml,*/*;q=0.8"}, 503, "text/html; charset=utf-8", page},
		{"sign-in page", http.MethodGet, "/auth/sign_in", nil, 503, "text/html; charset=utf-8", page},
		{"OAuth authorize", http.MethodGet, "/oauth/authorize?client_id=x", nil, 503, "text/html; charset=utf-8", page},
		{"asset", http.MethodGet, "/assets/index-abc.js", nil, 503, "text/html; charset=utf-8", page},
		{"form POST", http.MethodPost, "/auth/sign_in", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, 503, "text/html; charset=utf-8", page},
		{"JSON POST", http.MethodPost, "/somewhere", map[string]string{"Accept": "application/json"}, 503, "application/problem+json", problem},
		{"POST to a probe", http.MethodPost, "/readyz", nil, 503, "text/html; charset=utf-8", page},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := serve(h, tt.method, tt.target, tt.header)
			if w.Code != tt.code || w.Header().Get("Content-Type") != tt.contentType {
				t.Fatalf("= %d %q, want %d %q", w.Code, w.Header().Get("Content-Type"), tt.code, tt.contentType)
			}
			if got := w.Header().Get("Retry-After"); got != "7200" {
				t.Errorf("Retry-After = %q", got)
			}
			if got := w.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q", got)
			}
			switch tt.body {
			case problem:
				var p map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{
					"type": maintenance.ProblemType, "title": "Under maintenance", "status": 503.0,
					"message": "Upgrading <the> database.", "until": "2026-10-24T02:00:00Z", "status_page_url": "https://status.example.org/",
				}
				if len(p) != len(want) {
					t.Errorf("problem = %v", p)
				}
				for k, v := range want {
					if p[k] != v {
						t.Errorf("problem %s = %v, want %v", k, p[k], v)
					}
				}
			case page:
				if body := w.Body.String(); !strings.Contains(body, "<h1") || !strings.Contains(body, "Under maintenance") {
					t.Errorf("body = %s", body)
				}
			default:
				if w.Body.String() != tt.body {
					t.Errorf("body = %q, want %q", w.Body, tt.body)
				}
			}
		})
	}
}

// TestLambdaEvent checks that an SQS event that the Lambda Web Adapter passes through gets every
// record back as a failure, at any path, in the JSON body that is all that reaches Lambda.
func TestLambdaEvent(t *testing.T) {
	h := newHandler(t, testPage())
	req := httptest.NewRequest(http.MethodPost, "https://social.example/events",
		strings.NewReader(`{"Records":[{"messageId":"m1","body":"01J"},{"messageId":"m2","body":"{\"type\":\"tick\"}"}]}`))
	req.Header.Set("X-Amzn-Request-Context", "null")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("= %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if got, want := strings.TrimSpace(w.Body.String()), `{"batchItemFailures":[{"itemIdentifier":"m1"},{"itemIdentifier":"m2"}]}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	// A Function URL request to the same path is an ordinary request.
	w = serve(h, http.MethodPost, "/events", map[string]string{"X-Amzn-Request-Context": `{"http":{"method":"POST"}}`})
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Function URL request = %d %q, want the page", w.Code, w.Header().Get("Content-Type"))
	}
}

// The problem leaves out the members that are not configured.
func TestProblemWithoutOptionalMembers(t *testing.T) {
	w := serve(newHandler(t, maintenance.Page{ServerName: "social.example"}), http.MethodGet, "/api/v1/instance", map[string]string{"Accept-Language": "ja"})
	var p map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || len(p) != 3 || p["title"] != "メンテナンス中" {
		t.Errorf("problem = %s (%v)", w.Body, err)
	}
}

func TestRetryAfter(t *testing.T) {
	for _, tt := range []struct {
		name  string
		until time.Time
		want  string
	}{
		{"no end", time.Time{}, "600"},
		{"past", now.Add(-time.Minute), "600"},
		{"now", now, "600"},
		{"future", now.Add(90*time.Minute + 500*time.Millisecond), "5401"},
		{"over a day", now.Add(30 * time.Hour), "86400"},
	} {
		w := serve(newHandler(t, maintenance.Page{ServerName: "x", Until: tt.until}), http.MethodGet, "/healthz", nil)
		if got := w.Header().Get("Retry-After"); got != tt.want {
			t.Errorf("%s: Retry-After = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestPageLocales(t *testing.T) {
	h := newHandler(t, testPage())
	for _, tt := range []struct{ lang, root, title, until string }{
		{"en", `<html lang="en" dir="ltr">`, "Under maintenance", "Expected to end"},
		{"ja", `<html lang="ja" dir="ltr">`, "メンテナンス中", "終了予定"},
		{"fr", `<html lang="en" dir="ltr">`, "Under maintenance", "Expected to end"},
	} {
		w := serve(h, http.MethodGet, "/", map[string]string{"Accept-Language": tt.lang})
		body := w.Body.String()
		for _, want := range []string{
			tt.root, tt.title, tt.until,
			`<p class="server" dir="auto">social.example</p>`,
			`<p class="message" dir="auto" lang="">Upgrading &lt;the&gt; database.</p>`,
			`<time datetime="2026-10-24T02:00:00Z">2026-10-24 02:00 UTC</time>`,
			`<a href="https://status.example.org/" rel="noopener">`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: page lacks %s:\n%s", tt.lang, want, body)
			}
		}
		if strings.Contains(body, "<script") || strings.Contains(body, "<link") {
			t.Errorf("%s: page loads resources:\n%s", tt.lang, body)
		}
	}
	// Without the optional values, their parts are left out.
	body := serve(newHandler(t, maintenance.Page{ServerName: "social.example"}), http.MethodGet, "/", nil).Body.String()
	for _, part := range []string{"<h2", "<time", "<a "} {
		if strings.Contains(body, part) {
			t.Errorf("page has %s:\n%s", part, body)
		}
	}
}

// The page's CSP admits its inline CSS by hash and never 'unsafe-inline' (ADR-0084 §4).
func TestPageCSP(t *testing.T) {
	w := serve(newHandler(t, testPage()), http.MethodGet, "/", nil)
	csp := w.Header().Get("Content-Security-Policy")
	m := regexp.MustCompile(`(?s)<style>(.*?)</style>`).FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("no <style>:\n%s", w.Body)
	}
	sum := sha256.Sum256([]byte(m[1]))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "style-src "+hash) ||
		!strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP = %q, want style-src %s", csp, hash)
	}
	for _, want := range []string{"--color-surface:#fcf9f6;", "--color-surface:#241f20;", `--font-sans:"Noto Sans", sans-serif;`, "prefers-color-scheme: dark"} {
		if !strings.Contains(m[1], want) {
			t.Errorf("CSS lacks %s: %s", want, m[1])
		}
	}
	if physical := regexp.MustCompile(`[{;](width|height|min-height|max-width|(margin|padding|border)-(left|right|top|bottom)|left|right|top|bottom|text-align:(left|right)):`).FindString(m[1]); physical != "" {
		t.Errorf("CSS uses the physical property %s: %s", physical, m[1])
	}
}

// Render puts the CSP in a <meta> for the exported pages, without frame-ancestors, and renders
// one section per locale.
func TestRenderExport(t *testing.T) {
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	page, csp, err := maintenance.Render(testPage(), []*i18n.Localizer{b.Localizer("en"), b.Localizer("ja")}, maintenance.RenderOptions{MetaCSP: true})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page)
	meta := strings.TrimSuffix(csp, "; frame-ancestors 'none'")
	if !strings.Contains(body, `<meta http-equiv="Content-Security-Policy" content="`+html.EscapeString(meta)+`">`) {
		t.Errorf("no meta CSP %q:\n%s", meta, body)
	}
	if !strings.Contains(body, `<section lang="en" dir="ltr"`) || !strings.Contains(body, `<section lang="ja" dir="ltr"`) {
		t.Errorf("sections:\n%s", body)
	}
}

// Token values that could escape the declaration are refused.
func TestUnsafeTheme(t *testing.T) {
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"red;}body{display:none", "url(https://example.org/x.png)", "</style><script>", `red\;`} {
		p := maintenance.Page{ServerName: "x", Theme: maintenance.Theme{Light: map[string]string{"color-fg": v}}}
		if _, _, err := maintenance.Render(p, []*i18n.Localizer{b.Localizer("en")}, maintenance.RenderOptions{}); err == nil {
			t.Errorf("%q: Render accepted it", v)
		}
	}
}
