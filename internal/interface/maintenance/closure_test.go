// SPDX-License-Identifier: AGPL-3.0-only

package maintenance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/maintenance"
)

// The closure page is the maintenance page's template with the closure's words, its CSP in a
// <meta>, and the optional parts left out when they are empty.
func TestRenderClosure(t *testing.T) {
	b, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := maintenance.ClosurePage{
		ServerName: "social.example", Message: "Goodbye.", ClosesAt: time.Date(2027, 1, 31, 15, 0, 0, 0, time.UTC),
		StatusPageURL: "https://status.example.org/", Theme: testPage().Theme,
	}
	page, err := maintenance.RenderClosure(p, []*i18n.Localizer{b.Localizer("en"), b.Localizer("ja")})
	if err != nil {
		t.Fatal(err)
	}
	body := string(page)
	for _, want := range []string{
		`<html lang="en" dir="ltr">`, "<title>Closed – social.example</title>", `<meta http-equiv="Content-Security-Policy"`,
		`<section lang="en" dir="ltr" aria-labelledby="closure-en">`, `<h1 id="closure-ja">このサーバーは閉鎖しました</h1>`,
		`<p class="message" dir="auto" lang="">Goodbye.</p>`, `<time datetime="2027-01-31T15:00:00Z">2027-01-31 15:00 UTC</time>`,
		`<a href="https://status.example.org/" rel="noopener">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("closure page lacks %s:\n%s", want, body)
		}
	}
	page, err = maintenance.RenderClosure(maintenance.ClosurePage{ServerName: "social.example"}, []*i18n.Localizer{b.Localizer("en")})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"<h2", "<time", "<a "} {
		if strings.Contains(string(page), part) {
			t.Errorf("closure page has %s:\n%s", part, page)
		}
	}
	if _, err := maintenance.RenderClosure(p, nil); err == nil {
		t.Error("RenderClosure without a locale succeeded")
	}
}

// Every example of configuration answers each gone path with 410 and names every locale's page.
func TestClosureConfig(t *testing.T) {
	for _, name := range maintenance.ClosureConfigs {
		b, err := maintenance.ClosureConfig(name, "social.example", []string{"en", "ja"})
		if err != nil {
			t.Fatal(err)
		}
		conf := string(b)
		for _, want := range append([]string{"social.example", "en", "ja", "/index.html", "410"},
			append(maintenance.ClosureGonePaths, maintenance.ClosureGonePrefixes...)...) {
			if !strings.Contains(conf, want) {
				t.Errorf("%s lacks %s:\n%s", name, want, conf)
			}
		}
	}
	if _, err := maintenance.ClosureConfig("apache.conf", "social.example", []string{"en"}); err == nil {
		t.Error("ClosureConfig wrote an unknown example")
	}
}
