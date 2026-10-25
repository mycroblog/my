// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminClosureExportHTML(t *testing.T) {
	newAdminEnv(t)
	t.Setenv("MY_MAINTENANCE_SERVER_NAME", "")
	t.Setenv("MY_STATUS_PAGE_URL", "https://status.example.org/")
	mustInstance(t, "", "set", "name", "Example Social")
	mustInstance(t, "", "set", "default_locale", "ja")
	dir := filepath.Join(t.TempDir(), "closure")

	// Before the closure, nothing is written.
	if code, _, stderr := runCmd(t, adminCmd, "closure", "export-html", "-out", dir); code != 1 || !strings.Contains(stderr, "the server is not closed (state: none)") {
		t.Errorf("export-html before the closure = %d %q", code, stderr)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("export-html before the closure wrote the directory")
	}
	closesAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	if code, _, stderr := runCmd(t, adminCmd, "closure", "announce", "-closes-at", closesAt.Format(time.RFC3339), "-message", "Thank you <all>."); code != 0 {
		t.Fatalf("announce = %d %q", code, stderr)
	}
	if code, _, stderr := runCmd(t, adminCmd, "closure", "close", "-confirm-domain", "social.example"); code != 0 {
		t.Fatalf("close = %d %q", code, stderr)
	}

	code, stdout, stderr := runCmd(t, adminCmd, "closure", "export-html", "-out", dir)
	if code != 0 || !strings.Contains(stderr, "warning: the Deletes of the closure are still being delivered") {
		t.Fatalf("export-html = %d %q %q", code, stdout, stderr)
	}
	for _, name := range []string{"ja.html", "en.html", "index.html", "nginx.conf", "Caddyfile", "cloudfront-function.js"} {
		if !strings.Contains(stdout, "wrote "+filepath.Join(dir, name)+"\n") {
			t.Errorf("stdout lacks %s: %q", name, stdout)
		}
	}
	date := `<time datetime="` + closesAt.Format(time.RFC3339) + `">` + closesAt.Format("2006-01-02 15:04 UTC") + `</time>`
	en, ja, index := readFile(t, filepath.Join(dir, "en.html")), readFile(t, filepath.Join(dir, "ja.html")), readFile(t, filepath.Join(dir, "index.html"))
	for name, tc := range map[string]struct {
		page string
		want []string
	}{
		"en.html":    {en, []string{`<html lang="en" dir="ltr">`, "This server has closed", "Closing date"}},
		"ja.html":    {ja, []string{`<html lang="ja" dir="ltr">`, "このサーバーは閉鎖しました", "閉鎖日"}},
		"index.html": {index, []string{`<html lang="ja" dir="ltr">`, "このサーバーは閉鎖しました", "This server has closed"}},
	} {
		for _, want := range append(tc.want, `<p class="server" dir="auto">Example Social</p>`, "Thank you &lt;all&gt;.", date,
			`href="https://status.example.org/"`, `<meta http-equiv="Content-Security-Policy"`) {
			if !strings.Contains(tc.page, want) {
				t.Errorf("%s lacks %s:\n%s", name, want, tc.page)
			}
		}
		if m := externalResource.FindString(tc.page); m != "" {
			t.Errorf("%s loads a resource (%s)", name, m)
		}
	}
	// Every locale, the instance's default first.
	if i, j := strings.Index(index, `<section lang="ja"`), strings.Index(index, `<section lang="en"`); i < 0 || j < i {
		t.Errorf("index.html lacks ja then en:\n%s", index)
	}
	nginx := readFile(t, filepath.Join(dir, "nginx.conf"))
	for _, want := range []string{
		"server_name social.example;", "location ^~ /ap/ { return 410; }", "location ^~ /@ { return 410; }",
		"location = /.well-known/webfinger { return 410; }", `"~*^ja(-|,|;|$)" /ja.html;`, `location ~ ^/(index|ja|en)\.html$ {`,
		"error_page 410 $closure_page;",
	} {
		if !strings.Contains(nginx, want) {
			t.Errorf("nginx.conf lacks %s:\n%s", want, nginx)
		}
	}
	caddy := readFile(t, filepath.Join(dir, "Caddyfile"))
	for _, want := range []string{"social.example {", "@gone path /inbox /.well-known/webfinger", "/ap/* /nodeinfo/* /@*", "~(?i)^en(-|,|;|$) /en.html", "status 410"} {
		if !strings.Contains(caddy, want) {
			t.Errorf("Caddyfile lacks %s:\n%s", want, caddy)
		}
	}
	cloudfront := readFile(t, filepath.Join(dir, "cloudfront-function.js"))
	for _, want := range []string{`var gonePrefixes = ["/ap/","/nodeinfo/","/@"];`, `var locales = ["ja","en"];`, "statusCode: 410"} {
		if !strings.Contains(cloudfront, want) {
			t.Errorf("cloudfront-function.js lacks %s:\n%s", want, cloudfront)
		}
	}

	// -lang picks the locales and their order.
	dir2 := t.TempDir()
	if code, _, stderr := runCmd(t, adminCmd, "closure", "export-html", "-out", dir2, "-lang", "en"); code != 0 {
		t.Fatalf("export-html -lang en = %d %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir2, "ja.html")); err == nil {
		t.Error("export-html -lang en wrote ja.html")
	}
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"closure", "export-html"}, 2, "-out is required"},
		{[]string{"closure", "export-html", "-out", dir2, "-lang", "xx"}, 2, `unknown locale "xx"`},
	} {
		if code, _, stderr := runCmd(t, adminCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("admin %q = %d %q; want %d with %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}
