// SPDX-License-Identifier: AGPL-3.0-only

package payments

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/supporters"
)

// render answers a request with the document of data through a shell of entry.
func render(t *testing.T, s Shell, data supporters.PageData) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Render(rec, httptest.NewRequest(http.MethodGet, "/payments/pages/payjp/checkout", nil), http.StatusPaymentRequired,
		supporters.PageDocument{Lang: "ja", Dir: "ltr", Title: "Supporter に加入する", Data: data})
	return rec
}

var payEntry = PageEntry{
	Script: "/assets/pay-A.js", Stylesheets: []string{"/assets/pay-B.css"}, Preloads: []string{"/assets/shared-C.js"},
	Integrity: map[string]string{"/assets/pay-A.js": "sha384-AAAA", "/assets/shared-C.js": "sha384-CCCC"},
}

// The document is the entry's shell with the page's data, under the pages' policy and headers.
func TestShellRender(t *testing.T) {
	data := supporters.PageData{
		Screen: supporters.PageScreenCheckout, Title: "Supporter に加入する", TestMode: true, Price: "月額 1,200 円",
		Error: "</script><script>alert(1)</script>", Back: "https://my.example/supporter",
		Card: &supporters.PageCardForm{Action: "/payments/pages/payjp/checkout", State: "s1", PublicKey: "pk_test", Email: "alice@example.com"},
	}
	s := Shell{Entry: payEntry, ThemeStylesheet: func(context.Context) string { return "/theme/r1.css" }}
	rec := render(t, s, data)
	if rec.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want the page's", rec.Code)
	}
	for k, want := range map[string]string{
		"Content-Type": "text/html; charset=utf-8", "Content-Language": "ja", "Cache-Control": "no-store",
		"Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff", "Content-Security-Policy": PageContentSecurityPolicy(""),
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<html lang="ja" dir="ltr">`, `<title>Supporter に加入する</title>`, `<meta name="robots" content="noindex, nofollow">`,
		`<link rel="stylesheet" href="/assets/pay-B.css">`, `<link rel="stylesheet" href="/theme/r1.css">`,
		`<link rel="modulepreload" href="/assets/shared-C.js" integrity="sha384-CCCC">`,
		`<script type="module" src="/assets/pay-A.js" integrity="sha384-AAAA"></script>`,
		`<!--email_off--><script type="application/json" id="pay">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the document lacks %s:\n%s", want, body)
		}
	}
	if n := strings.Count(body, "<script"); n != 2 {
		t.Errorf("the document has %d script elements, want the entry's and the data block:\n%s", n, body)
	}
	block := regexp.MustCompile(`(?s)<script type="application/json" id="pay">(.*?)</script>`).FindStringSubmatch(body)
	if block == nil {
		t.Fatalf("no data block:\n%s", body)
	}
	var got supporters.PageData
	if err := json.Unmarshal([]byte(html.UnescapeString(block[1])), &got); err != nil {
		t.Fatalf("the data block %s: %v", block[1], err)
	}
	if got.Error != data.Error || got.Card == nil || *got.Card != *data.Card || got.Price != data.Price || !got.TestMode {
		t.Errorf("the data block = %+v, want %+v", got, data)
	}
}

// The policy allows no script but this site's and payjp.js, PAY.JP's frames and API, forms to this
// site only, and no framing; a dev server adds its own origin.
func TestPageContentSecurityPolicy(t *testing.T) {
	csp := PageContentSecurityPolicy("")
	directives := map[string]string{}
	for d := range strings.SplitSeq(csp, "; ") {
		name, value, _ := strings.Cut(d, " ")
		directives[name] = value
	}
	for name, want := range map[string]string{
		"default-src": "'none'", "script-src": "'self' https://js.pay.jp", "style-src": "'self'",
		"frame-src": "https://js.pay.jp https://api.pay.jp", "connect-src": "https://api.pay.jp",
		"img-src": "'self' data:", "font-src": "'self'", "form-action": "'self'", "frame-ancestors": "'none'",
		"base-uri": "'none'", "object-src": "'none'",
	} {
		if got := directives[name]; got != want {
			t.Errorf("%s = %q, want %q (%s)", name, got, want, csp)
		}
	}
	if len(directives) != 11 {
		t.Errorf("the policy has %d directives, want 11: %s", len(directives), csp)
	}
	dev := PageContentSecurityPolicy("http://localhost:5173")
	for _, want := range []string{
		"script-src 'self' http://localhost:5173 https://js.pay.jp;", "style-src 'self' 'unsafe-inline' http://localhost:5173;",
		"connect-src https://api.pay.jp 'self' http://localhost:5173 ws://localhost:5173;",
	} {
		if !strings.Contains(dev, want) {
			t.Errorf("the dev policy %q lacks %q", dev, want)
		}
	}
}
