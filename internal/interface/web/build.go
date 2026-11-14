// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
)

// Paths inside the build.
const (
	// manifestPath is the Vite build manifest (build.manifest: true).
	manifestPath = ".vite/manifest.json"
	// entryKey is the manifest key of the SPA's entry: the HTML input of the Vite build.
	entryKey = "index.html"
	// authEntryKey is the manifest key of the auth entry (ADR-0028 §4, ADR-0090 §16): the HTML
	// input of the sign-in, registration, and consent screens, which a full page load opens.
	authEntryKey = "auth.html"
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
	File    string   `json:"file"`
	IsEntry bool     `json:"isEntry"`
	Imports []string `json:"imports"`
	CSS     []string `json:"css"`
}

// entryTags are the URLs a document loads to start an entry of the build.
type entryTags struct {
	Script     string   // the entry module
	Stylesheet []string // CSS of the entry and of every chunk it imports statically
	Preload    []string // chunks the entry imports statically, for <link rel="modulepreload">
}

// errNoBuild means the build has no Vite manifest: dist/ holds no build of the web client.
var errNoBuild = errors.New("web: the build has no " + manifestPath + "; run scripts/task.sh web:build")

// readEntries builds the tags of the SPA entry and of the auth entry from the build manifest; a
// build without the auth entry leaves its tags empty.
func readEntries(build fs.FS) (app, auth entryTags, err error) {
	data, err := fs.ReadFile(build, manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return entryTags{}, entryTags{}, errNoBuild
	}
	if err != nil {
		return entryTags{}, entryTags{}, fmt.Errorf("web: read %s: %w", manifestPath, err)
	}
	var manifest map[string]chunk
	if err := json.Unmarshal(data, &manifest); err != nil {
		return entryTags{}, entryTags{}, fmt.Errorf("web: parse %s: %w", manifestPath, err)
	}
	if app, err = entryOf(manifest, entryKey); err != nil {
		return entryTags{}, entryTags{}, err
	}
	if _, ok := manifest[authEntryKey]; ok {
		if auth, err = entryOf(manifest, authEntryKey); err != nil {
			return entryTags{}, entryTags{}, err
		}
	}
	return app, auth, nil
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
