// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mycroblog/my/internal/interface/web"
)

// sri is the Subresource Integrity of data, as the web build's inventory writes it.
func sri(data []byte) string {
	sum := sha512.Sum384(data)
	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
}

// payBuild is fakeBuild with the payment entry, which shares the app's chunk (and with lazy imports
// the app's lazy chunk on demand), and the inventory of its scripts that edit leaves as the web
// build writes it or changes.
func payBuild(t *testing.T, lazy bool, edit func(scripts []map[string]string) []map[string]string) fstest.MapFS {
	t.Helper()
	b := fakeBuild()
	dynamic, files := "", []string{"assets/pay-HHHH.js", "assets/shared-BBBB.js"}
	if lazy {
		dynamic, files = `, "dynamicImports": ["src/lazy.tsx"]`, append(files, "assets/lazy-EEEE.js")
	}
	b[".vite/manifest.json"] = &fstest.MapFile{Data: []byte(`{
		"index.html": {"file": "assets/index-AAAA.js", "src": "index.html", "isEntry": true,
			"imports": ["_shared-BBBB.js"], "dynamicImports": ["src/lazy.tsx"], "css": ["assets/index-CCCC.css"]},
		"_shared-BBBB.js": {"file": "assets/shared-BBBB.js", "css": ["assets/shared-DDDD.css"]},
		"src/lazy.tsx": {"file": "assets/lazy-EEEE.js", "src": "src/lazy.tsx", "isDynamicEntry": true},
		"pay.html": {"file": "assets/pay-HHHH.js", "src": "pay.html", "isEntry": true,
			"imports": ["_shared-BBBB.js"]` + dynamic + `, "css": ["assets/pay-IIII.css"]}
	}`)}
	b["pay.html"] = &fstest.MapFile{Data: []byte("<!doctype html><title>vite pay</title>")}
	b["assets/pay-HHHH.js"] = &fstest.MapFile{Data: []byte("console.log('pay')")}
	b["assets/pay-IIII.css"] = &fstest.MapFile{Data: []byte("form{}")}
	delete(b, web.ETagsPath)
	scripts := []map[string]string{}
	for _, f := range files {
		scripts = append(scripts, map[string]string{"url": "/" + f, "integrity": sri(b[f].Data)})
	}
	scripts = append(scripts, map[string]string{"url": "https://js.pay.jp/v2/pay.js", "loads": "third party"})
	if edit != nil {
		scripts = edit(scripts)
	}
	inv, err := json.Marshal(map[string]any{"entry": "pay.html", "scripts": scripts})
	if err != nil {
		t.Fatal(err)
	}
	b[web.PaymentScriptsPath] = &fstest.MapFile{Data: inv}
	return listed(b)
}

// TestPayEntry checks that the handler reads the payment entry of the build with the integrity of
// its scripts from the inventory, and never serves Vite's pay.html.
func TestPayEntry(t *testing.T) {
	build := payBuild(t, false, nil)
	h, mux := newHandler(t, web.Options{Build: build})
	got := h.PayEntry()
	want := web.Entry{
		Script: "/assets/pay-HHHH.js", Stylesheets: []string{"/assets/pay-IIII.css", "/assets/shared-DDDD.css"},
		Preloads: []string{"/assets/shared-BBBB.js"},
		Integrity: map[string]string{
			"/assets/pay-HHHH.js":    sri(build["assets/pay-HHHH.js"].Data),
			"/assets/shared-BBBB.js": sri(build["assets/shared-BBBB.js"].Data),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PayEntry() = %+v, want %+v", got, want)
	}
	if body := get(mux, "/pay.html", nil).Body.String(); strings.Contains(body, "vite pay") {
		t.Errorf("GET /pay.html serves Vite's pay.html:\n%s", body)
	}
	if rec := get(mux, "/payments/pages/payjp/checkout", nil); rec.Code != http.StatusNotFound {
		t.Errorf("GET a payment page from the web handler = %d, want the server's 404, never the shell", rec.Code)
	}

	without, _ := newHandler(t, web.Options{})
	if got := without.PayEntry(); got.Script != "" || got.Integrity != nil {
		t.Errorf("PayEntry() without the entry = %+v, want none", got)
	}
	dev, _ := newHandler(t, web.Options{DevViteOrigin: "http://localhost:5173"})
	if got := dev.PayEntry(); got.Script != "http://localhost:5173/src/pay.tsx" || got.Integrity != nil {
		t.Errorf("PayEntry() with a dev server = %+v", got)
	}
}

// TestPayEntryRefusesAStaleInventory checks that a build whose inventory is missing, misses a chunk
// of the entry, lists another script of this site, or names another integrity is refused.
func TestPayEntryRefusesAStaleInventory(t *testing.T) {
	for name, edit := range map[string]func([]map[string]string) []map[string]string{
		"a missing chunk": func(s []map[string]string) []map[string]string { return s[1:] },
		"another chunk": func(s []map[string]string) []map[string]string {
			return append(s, map[string]string{"url": "/assets/index-AAAA.js", "integrity": sri([]byte("console.log('entry')"))})
		},
		"another integrity": func(s []map[string]string) []map[string]string {
			s[0]["integrity"] = sri([]byte("changed"))
			return s
		},
	} {
		if _, err := web.New(web.Options{Build: payBuild(t, false, edit), Host: "my.example"}); err == nil || !strings.Contains(err.Error(), web.PaymentScriptsPath) {
			t.Errorf("%s: New = %v, want a refusal of the inventory", name, err)
		}
	}
	build := payBuild(t, false, nil)
	delete(build, web.PaymentScriptsPath)
	delete(build, web.ETagsPath)
	if _, err := web.New(web.Options{Build: listed(build), Host: "my.example"}); err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Errorf("no inventory: New = %v, want a refusal", err)
	}
}

// TestPayEntryRefusesLazyImports checks that a build whose payment entry imports a chunk on demand
// is refused even with that chunk in its inventory: neither the chunk nor the modulepreload that
// Vite adds for it would carry its integrity.
func TestPayEntryRefusesLazyImports(t *testing.T) {
	_, err := web.New(web.Options{Build: payBuild(t, true, nil), Host: "my.example"})
	if err == nil || !strings.Contains(err.Error(), "imports chunks on demand from assets/pay-HHHH.js") {
		t.Errorf("New = %v, want a refusal of the lazy import", err)
	}
}
