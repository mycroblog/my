// SPDX-License-Identifier: AGPL-3.0-only

package config

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "regenerate "+ReferencePath)

// TestReferenceUpToDate fails when the committed reference differs from the schema.
// Regenerate it with `scripts/task.sh go:config-docs`.
func TestReferenceUpToDate(t *testing.T) {
	path := filepath.Join("..", "..", "..", filepath.FromSlash(ReferencePath))
	want := Reference()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path) //nolint:gosec // fixed path inside the repository
	if err != nil {
		t.Fatalf("%v; run `scripts/task.sh go:config-docs`", err)
	}
	if string(got) != want {
		t.Errorf("%s is stale; run `scripts/task.sh go:config-docs`", ReferencePath)
	}
}

// designCitation matches what an operator document must not cite or link: ADRs, requirements, the design documents,
// and the issues and pull requests of the repository (the rules of `agentkit docs lint`, docs/lint.yaml).
var designCitation = regexp.MustCompile(`\bADR-\d{4}\b|\b(?:NFR-[A-Z]+-\d+|NFR-\d{3}|FR-\d{3})\b|(?:^|[\s(,])#\d{1,5}\b|docs/(?:adr|requirements|architecture)/|data-model ?§|github\.com/mycroblog/my/(?:issues|pull)/`)

// TestReferenceCitesNoDesignRecords keeps the descriptions of the schema, which the reference shows to operators,
// free of citations of design records: a description explains the behaviour itself.
func TestReferenceCitesNoDesignRecords(t *testing.T) {
	for i, line := range strings.Split(Reference(), "\n") {
		if m := designCitation.FindString(line); m != "" {
			t.Errorf("%s:%d cites %q; explain the behaviour in the description in schema.go instead", ReferencePath, i+1, m)
		}
	}
}
