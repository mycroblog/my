// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestOperatorDocsNameRealSettings keeps the MY_* names of the operators' documents true: each is a
// setting, its _FILE form, or MY_CONFIG, so that copying a document does not give an unknown-variable
// warning and a setting left at its default.
func TestOperatorDocsNameRealSettings(t *testing.T) {
	known := map[string]bool{EnvConfigFile: true}
	for i := range schema {
		known[schema[i].env()] = true
		known[schema[i].env()+"_FILE"] = true
	}
	// Names that the Compose files read, which do not reach the server.
	for _, name := range []string{"MY_HTTP_PORT", "MY_VERSION", "MY_PREVIOUS_IMAGE", "MY_PREVIOUS_SCHEMA_IMAGE"} {
		known[name] = true
	}
	files, err := filepath.Glob("../../../docs/operations/*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("documents = %v, %v", files, err)
	}
	name := regexp.MustCompile(`\bMY_[A-Z0-9_]*[A-Z0-9]\b`)
	for _, file := range files {
		text, err := os.ReadFile(file) //nolint:gosec // G304: a document of this repository
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(text), "\n") {
			for _, found := range name.FindAllString(line, -1) {
				if !known[found] {
					t.Errorf("%s:%d names %s, which is not a setting", filepath.Base(file), n+1, found)
				}
			}
		}
	}
}
