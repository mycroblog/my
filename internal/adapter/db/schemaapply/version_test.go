// SPDX-License-Identifier: AGPL-3.0-only

package schemaapply

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	goModAtlas   = regexp.MustCompile(`(?m)^\s*(?:require\s+)?ariga\.io/atlas\s+v(\S+)`)
	scriptAtlas  = regexp.MustCompile(`(?m)^ATLAS_VERSION=(\S+)\s*$`)
	repoRootPath = filepath.Join("..", "..", "..", "..")
)

// atlasVersions returns the version of ariga.io/atlas that goMod requires and the
// ATLAS_VERSION of the install script, and an error when they differ: `my schema apply` (the
// library) and CI's lint (the CLI) must plan with the same Atlas (ADR-0126 §3).
func atlasVersions(goMod, script string) (lib, cli string, err error) {
	m := goModAtlas.FindStringSubmatch(goMod)
	if m == nil {
		return "", "", fmt.Errorf("go.mod requires no ariga.io/atlas")
	}
	s := scriptAtlas.FindStringSubmatch(script)
	if s == nil {
		return "", "", fmt.Errorf("scripts/install-atlas.sh sets no ATLAS_VERSION")
	}
	lib, cli = m[1], s[1]
	if lib != cli {
		return lib, cli, fmt.Errorf("go.mod requires ariga.io/atlas v%s, but scripts/install-atlas.sh installs the Atlas CLI %s: change both to the same version (the CLI's checksums too)", lib, cli)
	}
	return lib, cli, nil
}

// TestAtlasLibraryMatchesTheCLI fails when the Atlas library of go.mod and the Atlas CLI of
// scripts/install-atlas.sh have different versions, for example after a dependency update of
// only one of them.
func TestAtlasLibraryMatchesTheCLI(t *testing.T) {
	goMod, err := os.ReadFile(filepath.Join(repoRootPath, "go.mod")) //nolint:gosec // G304: a file of the repository
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(repoRootPath, "scripts", "install-atlas.sh")) //nolint:gosec // G304: a file of the repository
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := atlasVersions(string(goMod), string(script)); err != nil {
		t.Error(err)
	}
}

func TestAtlasVersionsReportsADrift(t *testing.T) {
	const script = "#!/usr/bin/env bash\nset -euo pipefail\n\nATLAS_VERSION=1.3.0\n"
	for _, tc := range []struct {
		name, goMod string
		ok          bool
	}{
		{"same", "module m\n\nrequire (\n\tariga.io/atlas v1.3.0\n)\n", true},
		{"single require", "module m\n\nrequire ariga.io/atlas v1.3.0\n", true},
		{"newer library", "module m\n\nrequire (\n\tariga.io/atlas v1.4.0\n)\n", false},
		{"older library", "module m\n\nrequire (\n\tariga.io/atlas v1.2.9\n)\n", false},
		{"no library", "module m\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := atlasVersions(tc.goMod, script)
			if (err == nil) != tc.ok {
				t.Errorf("atlasVersions = %v, want ok %v", err, tc.ok)
			}
		})
	}
	if _, _, err := atlasVersions("require ariga.io/atlas v1.3.0\n", strings.Replace(script, "1.3.0", "1.3.1", 1)); err == nil {
		t.Error("atlasVersions accepts a newer CLI")
	}
}
