// SPDX-License-Identifier: AGPL-3.0-only

package ctxmap

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		pat, p string
		want   bool
	}{
		{"internal/**", "internal/app/content/x.go", true},
		{"internal/*/content/**", "internal/app/content/x.go", true},
		{"internal/*/content/**", "internal/app/contents/x.go", false},
		{"**/*.gen.go", "internal/interface/clientapi/api.gen.go", true},
		{"**/*.gen.go", "x.gen.go", true},
		{"*.md", "docs/a.md", false},
		{"docs/**/*.md", "docs/a.md", true},
		{"app/web/src/features/*/**", "app/web/src/features/admin/a/b.tsx", true},
	}
	for _, c := range cases {
		if got := Match(c.pat, c.p); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pat, c.p, got, c.want)
		}
	}
}

// write creates the files of a repository under a new directory and returns it with their sorted paths.
func write(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var paths []string
	for p, c := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return root, paths
}

const rootMap = `ignore: ["**/*.gen.go"]
userdocs:
  cover: ["ops/*.md"]
entries:
  - paths: [".github/**"]
    checks: [go]
    shared: true
  - paths: ["*.md", ".*"]
`

func TestCheck(t *testing.T) {
	root, files := write(t, map[string]string{
		"context-map.yaml":                      rootMap,
		"internal/context-map.yaml":             "docs: [\"doc.md#1-rules\"]\nuserdocs: [\"ops/a.md#setup\"]\nchecks: [go, nope]\n",
		"internal/app/content/context-map.yaml": "docs: [\"doc.md#missing\", \"doc.md#1-rules\"]\nuserdocs: [\"ops/a.md#teardown\", \"ops/gone.md\"]\nchecks: [go]\nshared: true\n",
		"ops/context-map.yaml":                  "checks: [go]\n",
		"doc.md":                                "# Title\n\n## 1. Rules\n",
		"ops/a.md":                              "# A\n\n## Setup\n",
		"ops/b.md":                              "# B\n",
		"internal/app/content/a.go":             "",
		"internal/app/graph/b.go":               "",
		"internal/app/graph/c.gen.go":           "",
		"new/deep/x.go":                         "",
		"new/y.gen.go":                          "",
		"README.txt":                            "",
		".editorconfig":                         "",
		".github/workflows/ci.yml":              "",
		".hidden/x.txt":                         "",
	})
	m, err := Load(root, files)
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]Entry{
		"internal/app/content/a.go": {
			File: "internal/app/content/context-map.yaml", Name: "internal/app/content", Shared: true,
			Docs: []string{"doc.md#missing", "doc.md#1-rules"}, Userdocs: []string{"ops/a.md#teardown", "ops/gone.md", "ops/a.md#setup"}, Checks: []string{"go", "nope"},
		},
		"internal/app/content": {
			File: "internal/app/content/context-map.yaml", Name: "internal/app/content", Shared: true,
			Docs: []string{"doc.md#missing", "doc.md#1-rules"}, Userdocs: []string{"ops/a.md#teardown", "ops/gone.md", "ops/a.md#setup"}, Checks: []string{"go", "nope"},
		},
		"internal/app/graph/b.go": {
			File: "internal/context-map.yaml", Name: "internal",
			Docs: []string{"doc.md#1-rules"}, Userdocs: []string{"ops/a.md#setup"}, Checks: []string{"go", "nope"},
		},
		".github/workflows/ci.yml": {File: "context-map.yaml", Name: ".github/**", Paths: []string{".github/**"}, Checks: []string{"go"}, Shared: true},
		".editorconfig":            {File: "context-map.yaml", Name: "*.md", Paths: []string{"*.md", ".*"}},
	} {
		if got, ok := m.For(p); !ok || fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Errorf("For(%q) = %+v, %v, want %+v", p, got, ok, want)
		}
	}
	if got, ok := m.For("new/deep/x.go"); ok {
		t.Errorf("For of a path under no map = %+v", got)
	}
	var got []string
	for _, p := range m.Check(root, files, []string{"go"}) {
		got = append(got, p.Path+" "+p.Msg)
	}
	want := []string{
		"./ no map describes README.txt; name them in an entry of the root context-map.yaml",
		".hidden/ no map describes x.txt; name them in an entry of the root context-map.yaml",
		"new/deep/ no map describes x.go; add new/context-map.yaml with what holds for the whole directory",
		"internal/app/content/context-map.yaml repeats \"doc.md#1-rules\", which a context-map.yaml above it says for this directory too: delete it here",
		"internal/app/content/context-map.yaml repeats \"go\"",
		"internal/context-map.yaml internal: unknown check context \"nope\"",
		"internal/app/content/context-map.yaml internal/app/content: doc.md has no heading #missing",
		"internal/app/content/context-map.yaml internal/app/content: userdocs: ops/a.md has no heading #teardown",
		"internal/app/content/context-map.yaml internal/app/content: userdocs: ops/gone.md does not exist",
		"ops/b.md is the userdocs of no map",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d problems:\n%s", len(got), strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.Contains(got[i], want[i]) {
			t.Errorf("problem %d = %q, want it to contain %q", i, got[i], want[i])
		}
	}
}

func TestLoadRejects(t *testing.T) {
	const perFile = "what governs one file is said by the code and the check that enforces it"
	for name, c := range map[string]struct {
		files map[string]string
		want  string
	}{
		"entries":      {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "entries:\n  - paths: [\"*\"]\n"}, perFile},
		"paths":        {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "paths: [\"x.go\"]\nchecks: [go]\n"}, perFile},
		"notes":        {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "checks: [go]\nnotes: [\"Never do it.\"]\n"}, perFile},
		"settings":     {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "ignore: [\"*\"]\nchecks: [go]\n"}, "field ignore not found"},
		"nothing":      {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "# SPDX-License-Identifier: AGPL-3.0-only\n"}, "says nothing: delete the file"},
		"no root":      {map[string]string{"a/context-map.yaml": "checks: [go]\n"}, "the repository root has no map file"},
		"root strict":  {map[string]string{"context-map.yaml": "strict: [\"*\"]\n"}, "field strict not found"},
		"root pattern": {map[string]string{"context-map.yaml": "entries:\n  - paths: [\"internal/platform/serve.go\"]\n"}, "the root's files and dot-directories only"},
		"empty paths":  {map[string]string{"context-map.yaml": "entries:\n  - docs: [\"a.md\"]\n"}, "no paths"},
	} {
		root, files := write(t, c.files)
		if _, err := Load(root, files); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Load = %v, want an error with %q", name, err, c.want)
		}
	}
}

func TestImpact(t *testing.T) {
	m := &Map{
		Ignore:   []string{"**/*.gen.go"},
		Userdocs: Userdocs{Skip: []string{"**/*_test.go"}},
		Dirs: map[string]Entry{
			"internal":         {Userdocs: []string{"ops/running.md"}},
			"internal/mail":    {Userdocs: []string{"ops/email.md#smtp", "ops/email.md#bounces"}},
			"internal/webhook": {Userdocs: []string{"ref/webhooks.md"}},
			"ops":              {Userdocs: []string{"ops/email.md"}},
		},
	}
	got := m.Impact([]string{
		"internal/webhook/context-map.yaml",
		"internal/mail/smtp.go", "internal/mail/smtp_test.go", "internal/webhook/x.gen.go",
		"internal/other.go", "ops/running.md", "ops/email.md",
	})
	want := []Impact{
		{Doc: "ops/email.md", Refs: []string{"ops/email.md", "ops/email.md#bounces", "ops/email.md#smtp"}, Paths: []string{"internal/mail/smtp.go", "ops/running.md"}, Changed: true},
		{Doc: "ops/running.md", Refs: []string{"ops/running.md"}, Paths: []string{"internal/mail/smtp.go", "internal/other.go"}, Changed: true},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Impact =\n%v\nwant\n%v", got, want)
	}
	if got := m.Impact([]string{"internal/webhook/hook.go"}); len(got) != 2 || got[0].Doc != "ops/running.md" || got[1].Doc != "ref/webhooks.md" || got[1].Changed {
		t.Errorf("Impact of an untouched document = %v", got)
	}
}
