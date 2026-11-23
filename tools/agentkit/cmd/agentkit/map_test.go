// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The contexts a map may name are those scripts/changed-contexts.sh selects, read from the script itself.
func TestCheckContextsAreTheScripts(t *testing.T) {
	got, err := checkContexts(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"docs", "go", "web", "shell", "e2e", "stories"} {
		if !slices.Contains(got, c) {
			t.Errorf("checkContexts() = %v, lacks %q", got, c)
		}
	}
	for _, c := range []string{"ui", "mail"} {
		if slices.Contains(got, c) {
			t.Errorf("checkContexts() = %v, has %q, which the script does not select", got, c)
		}
	}
}

func TestCheckContextsOfAScript(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(root, contextsScript), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("#!/usr/bin/env bash\nset -euo pipefail\n\nALL=(docs go web)\n\nfoo() { :; }\n")
	got, err := checkContexts(root)
	if err != nil || !slices.Equal(got, []string{"docs", "go", "web"}) {
		t.Errorf("checkContexts() = %v, %v; want [docs go web]", got, err)
	}

	write("#!/usr/bin/env bash\n# no list\n")
	if got, err := checkContexts(root); err == nil {
		t.Errorf("checkContexts() = %v, want an error when the script lists no contexts", got)
	}
}
