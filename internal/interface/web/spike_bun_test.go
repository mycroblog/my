// SPDX-License-Identifier: AGPL-3.0-only

package web

// Spike #2906, not for main: whether the server reads the manifest that web/app/spike/bun-build.ts writes from
// Bun's metafile as it reads Vite's. SPIKE_BUILD names the build directory.

import (
	"os"
	"testing"
)

func TestSpikeReadEntriesOfBunBuild(t *testing.T) {
	dir := os.Getenv("SPIKE_BUILD")
	if dir == "" {
		t.Skip("SPIKE_BUILD is not set")
	}
	e, err := readEntries(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	for name, tags := range map[string]entryTags{"app": e.app, "auth": e.auth, "pay": e.pay} {
		t.Logf("%s: script %s, %d preloads, %d stylesheets, %d integrities", name, tags.Script, len(tags.Preload), len(tags.Stylesheet), len(tags.Integrity))
	}
}
