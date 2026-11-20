// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"path"
	"slices"
	"strings"
)

// Paths inside the build.
const (
	// manifestPath is the Vite build manifest (build.manifest: true).
	manifestPath = ".vite/manifest.json"
	// ETagsPath is the list of the ETags of the build's files, which the web build writes once every file is in
	// place (web/app/scripts/etags.ts), so that the server does not read the files when it starts.
	ETagsPath = ".vite/etags.json"
	// entryKey is the manifest key of the SPA's entry: the HTML input of the Vite build.
	entryKey = "index.html"
	// authEntryKey is the manifest key of the auth entry (ADR-0028 §4, ADR-0090 §16): the HTML
	// input of the sign-in, registration, and consent screens, which a full page load opens.
	authEntryKey = "auth.html"
	// payEntryKey is the manifest key of the payment entry (ADR-0074 §11): the HTML input of the
	// checkout and portal pages of a provider without hosted ones, which a full page load opens.
	payEntryKey = "pay.html"
	// PaymentScriptsPath is the inventory of the scripts that the payment entry loads (PCI DSS
	// 6.4.3), which the web build writes from the manifest (web/app/scripts/payment-scripts.ts) and
	// each release publishes. The server takes the integrity of the entry's scripts from it and
	// refuses a build whose inventory does not list the entry's chunks.
	PaymentScriptsPath = ".vite/payment-scripts.json"
	// assetsDir holds the content-hashed files of the build (Vite's build.assetsDir).
	assetsDir = "assets"
	// licensesDir holds the license notices that the build carries: those of the bundled npm packages
	// (Vite's build.license) and of the bundled data that is no npm package.
	licensesDir = "licenses"
	// iconPath is the instance-independent app icon (web/brand/icon.svg).
	iconPath = "icon.svg"
	// serviceWorkerPath is the Service Worker of the web client (ADR-0028).
	serviceWorkerPath = "sw.js"
	// themeScriptPath is the classic script that applies the color theme stored on the browser before the
	// first paint. The shell loads it render-blocking, before its stylesheets.
	themeScriptPath = "theme.js"
	// appleTouchIconPath is the home screen icon of iOS, which takes no SVG and applies its own mask.
	appleTouchIconPath = "apple-touch-icon.png"
)

// chunk is one entry of the Vite build manifest
// (https://vite.dev/guide/backend-integration.html).
type chunk struct {
	File           string   `json:"file"`
	IsEntry        bool     `json:"isEntry"`
	Imports        []string `json:"imports"`
	DynamicImports []string `json:"dynamicImports"`
	CSS            []string `json:"css"`
}

// entryTags are the URLs a document loads to start an entry of the build.
type entryTags struct {
	Script     string   // the entry module
	Stylesheet []string // CSS of the entry and of every chunk it imports statically
	Preload    []string // chunks the entry imports statically, for <link rel="modulepreload">
	// Integrity is the Subresource Integrity of Script and of each of Preload, by URL; nil for an
	// entry whose document loads its scripts without it.
	Integrity map[string]string
}

// builtEntries are the tags of the entries of the build: the SPA's, the auth entry's, and the
// payment entry's, the last two empty when the build lacks them.
type builtEntries struct {
	app, auth, pay entryTags
}

// errNoBuild means the build has no Vite manifest: dist/ holds no build of the web client.
var errNoBuild = errors.New("web: the build has no " + manifestPath + "; run scripts/task.sh web:build")

// readEntries builds the tags of the entries from the build manifest, and the integrity of the
// payment entry's scripts from its inventory.
func readEntries(build fs.FS) (builtEntries, error) {
	data, err := readBuildFile(build, manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return builtEntries{}, errNoBuild
	}
	if err != nil {
		return builtEntries{}, fmt.Errorf("web: read %s: %w", manifestPath, err)
	}
	var manifest map[string]chunk
	if err := json.Unmarshal(data, &manifest); err != nil {
		return builtEntries{}, fmt.Errorf("web: parse %s: %w", manifestPath, err)
	}
	var e builtEntries
	if e.app, err = entryOf(manifest, entryKey); err != nil {
		return builtEntries{}, err
	}
	if _, ok := manifest[authEntryKey]; ok {
		if e.auth, err = entryOf(manifest, authEntryKey); err != nil {
			return builtEntries{}, err
		}
	}
	if _, ok := manifest[payEntryKey]; ok {
		if e.pay, err = entryOf(manifest, payEntryKey); err != nil {
			return builtEntries{}, err
		}
		if e.pay.Integrity, err = paymentScripts(build, manifest); err != nil {
			return builtEntries{}, err
		}
	}
	return e, nil
}

// scriptInventory is the inventory at PaymentScriptsPath.
type scriptInventory struct {
	Entry   string `json:"entry"`
	Scripts []struct {
		URL       string `json:"url"`
		Integrity string `json:"integrity"`
	} `json:"scripts"`
}

// paymentScripts reads the integrity of the payment entry's own scripts from the inventory at
// PaymentScriptsPath and checks it against the manifest and the files: it lists every chunk that the
// entry loads, with the integrity of its file, and no other script of this site. A build whose
// inventory is missing or stale is refused, so that no release serves a payment page whose scripts
// its inventory does not name, and so is a build whose payment entry imports a chunk on demand:
// such a chunk, and the modulepreload that Vite's helper adds for it, would run unchecked by its
// hash (web/app/scripts/payment-scripts.ts says why no import map checks it). The entry's chunks
// are read once, when the server starts, and gunzipped: they are small, unlike the fonts that the
// ETags spare it.
func paymentScripts(build fs.FS, manifest map[string]chunk) (map[string]string, error) {
	stale := func(why string) error {
		return fmt.Errorf("web: %s %s; run scripts/task.sh web:build", PaymentScriptsPath, why)
	}
	data, err := readBuildFile(build, PaymentScriptsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, stale("is missing")
	}
	if err != nil {
		return nil, fmt.Errorf("web: read %s: %w", PaymentScriptsPath, err)
	}
	var inv scriptInventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, stale("is not an inventory: " + err.Error())
	}
	if inv.Entry != payEntryKey {
		return nil, stale(fmt.Sprintf("is the inventory of %q, not of %s", inv.Entry, payEntryKey))
	}
	loaded := map[string]bool{}
	var lazy []string
	var visit func(key string)
	visit = func(key string) {
		c, ok := manifest[key]
		if !ok || loaded[fileURL(c.File)] {
			return
		}
		loaded[fileURL(c.File)] = true
		if len(c.DynamicImports) > 0 {
			lazy = append(lazy, c.File)
		}
		for _, k := range c.Imports {
			visit(k)
		}
	}
	visit(payEntryKey)
	if len(lazy) > 0 {
		slices.Sort(lazy)
		return nil, fmt.Errorf("web: %s imports chunks on demand from %s, which no integrity checks; keep lazy imports out of the payment pages' modules",
			payEntryKey, strings.Join(lazy, ", "))
	}
	integrity := map[string]string{}
	for _, s := range inv.Scripts {
		if !strings.HasPrefix(s.URL, "/") {
			// A script of another site (payjp.js), which the page's policy names.
			continue
		}
		if !loaded[s.URL] {
			return nil, stale("lists " + s.URL + ", which " + payEntryKey + " does not load")
		}
		data, err := readBuildFile(build, strings.TrimPrefix(s.URL, "/"))
		if err != nil {
			return nil, stale("lists " + s.URL + ", which the build cannot read: " + err.Error())
		}
		sum := sha512.Sum384(data)
		if want := "sha384-" + base64.StdEncoding.EncodeToString(sum[:]); s.Integrity != want {
			return nil, stale("has the integrity " + s.Integrity + " for " + s.URL + ", whose file has " + want)
		}
		integrity[s.URL] = s.Integrity
	}
	for u := range loaded {
		if _, ok := integrity[u]; !ok {
			return nil, stale("does not list " + u + ", which " + payEntryKey + " loads")
		}
	}
	return integrity, nil
}

// entryOf builds the tags of the entry key, following Vite's backend integration guide: the
// entry's file, the CSS of the entry and of its static imports, and the static imports as module
// preloads, each once and in import order. URLs are absolute paths, since a document of an entry
// is served at many paths.
func entryOf(manifest map[string]chunk, key string) (entryTags, error) {
	entry, ok := manifest[key]
	if !ok || !entry.IsEntry || entry.File == "" {
		return entryTags{}, fmt.Errorf("web: %s has no entry %q", manifestPath, key)
	}
	tags := entryTags{Script: fileURL(entry.File)}
	addCSS := func(c chunk) {
		for _, css := range c.CSS {
			if u := fileURL(css); !slices.Contains(tags.Stylesheet, u) {
				tags.Stylesheet = append(tags.Stylesheet, u)
			}
		}
	}
	addCSS(entry)
	seen := map[string]bool{key: true}
	var visit func(keys []string) error
	visit = func(keys []string) error {
		for _, k := range keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			c, ok := manifest[k]
			if !ok {
				return fmt.Errorf("web: %s imports the missing chunk %q", manifestPath, k)
			}
			tags.Preload = append(tags.Preload, fileURL(c.File))
			addCSS(c)
			if err := visit(c.Imports); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(entry.Imports); err != nil {
		return entryTags{}, err
	}
	return tags, nil
}

// links returns the Link header values (RFC 8288) that hint the files a document of the entry needs
// first: each stylesheet as rel=preload; as=style, then the entry module as rel=preload; as=script;
// crossorigin. Browsers take no rel=modulepreload from a 103 Early Hints response; a preload in
// CORS mode with the same-origin credentials is the fetch of a module script of this site, so the
// module reuses its response, as a stylesheet does that of its no-cors preload. The chunks the entry
// imports are left to the document's modulepreload tags, which keeps the headers small. An entry
// without a script hints nothing.
func (t entryTags) links() []string {
	if t.Script == "" {
		return nil
	}
	links := make([]string, 0, len(t.Stylesheet)+1)
	for _, u := range t.Stylesheet {
		links = append(links, "<"+u+">; rel=preload; as=style")
	}
	return append(links, "<"+t.Script+">; rel=preload; as=script; crossorigin")
}

// WriteLinks adds links to the response's Link headers, which a CDN can also make early hints of.
// With early, for a GET request over HTTP/1.1 or later, it first sends them in a 103 Early Hints
// response (RFC 8297), so that the browser fetches the files while the server prepares the
// document; the headers stay for the document's response. Call it before setting the document's
// headers. With no links it does nothing.
func WriteLinks(w http.ResponseWriter, r *http.Request, links []string, early bool) {
	if len(links) == 0 {
		return
	}
	hd := w.Header()
	// An HTTP/1.0 client knows no 1xx (RFC 9110 §15.2), and a HEAD request loads nothing.
	if !early || r.Method != http.MethodGet || !r.ProtoAtLeast(1, 1) {
		for _, l := range links {
			hd.Add("Link", l)
		}
		return
	}
	// The 103 carries the header map as it is, so it holds the links alone: the headers set so far
	// are the server's defaults for a response that is no document, and a browser applies a
	// Content-Security-Policy of the 103 to the hinted fetches.
	set := hd.Clone()
	clear(hd)
	for _, l := range links {
		hd.Add("Link", l)
	}
	w.WriteHeader(http.StatusEarlyHints)
	maps.Copy(hd, set)
}

// fileURL is the absolute URL path of a file of the build.
func fileURL(file string) string { return path.Join("/", file) }

// appIcons are the icons of the Web App Manifest, in its order, each listed when the build has its file
// (web/brand, rendered by scripts/web-icons.sh). Chrome's install criteria ask for 192 and 512 px
// icons; the maskable one fills the shapes that Android masks icons into.
var appIcons = []manifestIcon{
	{Src: "/" + iconPath, Sizes: "any", Type: "image/svg+xml", Purpose: "any"},
	{Src: "/icon-192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
	{Src: "/icon-512.png", Sizes: "512x512", Type: "image/png", Purpose: "any"},
	{Src: "/icon-maskable-512.png", Sizes: "512x512", Type: "image/png", Purpose: "maskable"},
}
