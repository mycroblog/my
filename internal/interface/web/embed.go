// SPDX-License-Identifier: AGPL-3.0-only

package web

import (
	"embed"
	"io/fs"
)

// dist is the Vite build of the web client, including .vite/manifest.json (hence all:).
//
//go:embed all:dist
var dist embed.FS

// Embedded returns the build of the web client embedded in the binary. Without a build (only
// dist/.gitkeep) it has no .vite/manifest.json.
func Embedded() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path, so fs.Sub cannot fail
	}
	return sub
}
