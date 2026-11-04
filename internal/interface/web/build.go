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
	// serverPagesKey is the manifest key of the stylesheet entry of the pages the server renders
	// without JavaScript (app/web/src/server-pages.css, ADR-0090).
	serverPagesKey = "src/server-pages.css"
	// passkeySignInKey is the manifest key of the script entry that signs in with a passkey on the
	// server-rendered sign-in page (app/web/src/server-pages/passkey-sign-in.ts, FR-004).
	passkeySignInKey = "src/server-pages/passkey-sign-in.ts" //nolint:gosec // G101: a file name, not a credential
	// assetsDir holds the content-hashed files of the build (Vite's build.assetsDir).
	assetsDir = "assets"
	// licensesDir holds the license notices that the build carries: those of the bundled npm packages
	// (Vite's build.license) and of the bundled data that is no npm package (app/web/vite-plugins/licenses.ts).
	licensesDir = "licenses"
	// iconPath is the instance-independent app icon (app/web/public/icon.svg).
	iconPath = "icon.svg"
	// serviceWorkerPath is the Service Worker that vite-plugin-pwa builds from app/web/sw/sw.ts (ADR-0028).
	serviceWorkerPath = "sw.js"
	// themeScriptPath is the classic script that applies the color theme stored on the browser before the
	// first paint (app/web/public/theme.js). The shell loads it render-blocking, before its stylesheets.
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

// entryTags are the URLs the shell loads to start the SPA.
type entryTags struct {
	Script     string   // the entry module
	Stylesheet []string // CSS of the entry and of every chunk it imports statically
	Preload    []string // chunks the entry imports statically, for <link rel="modulepreload">
	// ServerPages is the stylesheet of the server-rendered pages; the shell does not load it.
	ServerPages string
	// PasskeySignIn is the module script of the sign-in page's passkey form; the shell does not
	// load it.
	PasskeySignIn string
}

// errNoBuild means the build has no Vite manifest: app/web was not built into dist/.
var errNoBuild = errors.New("web: the build has no " + manifestPath + "; run scripts/task.sh web:build")

// readEntry builds the tags of the SPA entry from the build manifest, following Vite's
// backend integration guide: the entry's file, the CSS of the entry and of its static
// imports, and the static imports as module preloads, each once and in import order.
// URLs are absolute paths, since the shell is served at every SPA route. It also reads the
// stylesheet and the passkey script entries of the server-rendered pages, which a build may lack.
func readEntry(build fs.FS) (entryTags, error) {
	data, err := fs.ReadFile(build, manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return entryTags{}, errNoBuild
	}
	if err != nil {
		return entryTags{}, fmt.Errorf("web: read %s: %w", manifestPath, err)
	}
	var manifest map[string]chunk
	if err := json.Unmarshal(data, &manifest); err != nil {
		return entryTags{}, fmt.Errorf("web: parse %s: %w", manifestPath, err)
	}
	entry, ok := manifest[entryKey]
	if !ok || !entry.IsEntry || entry.File == "" {
		return entryTags{}, fmt.Errorf("web: %s has no entry %q", manifestPath, entryKey)
	}

	tags := entryTags{Script: fileURL(entry.File)}
	if c, ok := manifest[serverPagesKey]; ok && c.IsEntry && c.File != "" {
		tags.ServerPages = fileURL(c.File)
	}
	if c, ok := manifest[passkeySignInKey]; ok && c.IsEntry && c.File != "" {
		tags.PasskeySignIn = fileURL(c.File)
	}
	addCSS := func(c chunk) {
		for _, css := range c.CSS {
			if u := fileURL(css); !slices.Contains(tags.Stylesheet, u) {
				tags.Stylesheet = append(tags.Stylesheet, u)
			}
		}
	}
	addCSS(entry)
	seen := map[string]bool{entryKey: true}
	var visit func(keys []string) error
	visit = func(keys []string) error {
		for _, key := range keys {
			if seen[key] {
				continue
			}
			seen[key] = true
			c, ok := manifest[key]
			if !ok {
				return fmt.Errorf("web: %s imports the missing chunk %q", manifestPath, key)
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
// (app/web/public, rendered by scripts/web-icons.sh). Chrome's install criteria ask for 192 and 512 px
// icons; the maskable one fills the shapes that Android masks icons into.
var appIcons = []manifestIcon{
	{Src: "/" + iconPath, Sizes: "any", Type: "image/svg+xml", Purpose: "any"},
	{Src: "/icon-192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
	{Src: "/icon-512.png", Sizes: "512x512", Type: "image/png", Purpose: "any"},
	{Src: "/icon-maskable-512.png", Sizes: "512x512", Type: "image/png", Purpose: "maskable"},
}
