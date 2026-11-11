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
strict: ["*", "internal/app/*"]
userdocs:
  cover: ["ops/*.md"]
entries:
  - paths: ["*.md"]
`

func TestCheck(t *testing.T) {
	root, files := write(t, map[string]string{
		"context-map.yaml": rootMap,
		"internal/context-map.yaml": `entries:
  - paths: ["**"]
    docs: ["doc.md#1-rules"]
    userdocs: ["ops/a.md#setup"]
    checks: [go, nope]
`,
		"internal/app/content/context-map.yaml": `entries:
  - paths: ["**"]
    docs: ["doc.md#missing"]
    userdocs: ["ops/a.md#teardown", "ops/gone.md"]
  - paths: ["a.go"]
    shared: true
`,
		"ops/context-map.yaml":        "entries:\n  - paths: [\"*.md\"]\n",
		"doc.md":                      "# Title\n\n## 1. Rules\n",
		"ops/a.md":                    "# A\n\n## Setup\n",
		"ops/b.md":                    "# B\n",
		"internal/app/content/a.go":   "",
		"internal/app/graph/b.go":     "",
		"internal/app/graph/c.gen.go": "",
		"README.txt":                  "",
		".hidden/x.txt":               "",
	})
	m, err := Load(root, files)
	if err != nil {
		t.Fatal(err)
	}
	es := m.For("internal/app/content/a.go")
	if len(es) != 3 || es[0].Name != "internal/app/content (**)" || es[1].Name != "internal/app/content (a.go)" || !es[1].Shared || es[2].Name != "internal" {
		t.Errorf("For = %+v", es)
	}
	var got []string
	for _, p := range m.Check(root, files, []string{"go"}) {
		got = append(got, p.Path+" "+p.Msg)
	}
	want := []string{
		"./ no entry covers README.txt",
		".hidden/ no entry covers x.txt",
		"internal/app/graph/ matches strict pattern \"internal/app/*\": add internal/app/graph/context-map.yaml",
		"internal/app/content/context-map.yaml entry \"internal/app/content (**)\": doc.md has no heading #missing",
		"internal/app/content/context-map.yaml entry \"internal/app/content (**)\": userdocs: ops/a.md has no heading #teardown",
		"internal/app/content/context-map.yaml entry \"internal/app/content (**)\": userdocs: ops/gone.md does not exist",
		"internal/context-map.yaml entry \"internal\": unknown check context \"nope\"",
		"ops/b.md is the userdocs of no entry",
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
	for name, c := range map[string]struct {
		files map[string]string
		want  string
	}{
		"notes":       {map[string]string{"context-map.yaml": "entries:\n  - paths: [\"*\"]\n    notes: [\"Never do it.\"]\n"}, "a rule lives in the check that enforces it"},
		"unknown":     {map[string]string{"context-map.yaml": "entries:\n  - paths: [\"*\"]\n    name: x\n"}, "field name not found"},
		"no root":     {map[string]string{"a/context-map.yaml": "entries:\n  - paths: [\"*\"]\n"}, "the repository root has no map file"},
		"settings":    {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "strict: [\"*\"]\nentries:\n  - paths: [\"*\"]\n"}, "settings of the root"},
		"outside":     {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "entries:\n  - paths: [\"../b/*\"]\n"}, "stay inside it"},
		"no entries":  {map[string]string{"context-map.yaml": rootMap, "a/context-map.yaml": "entries: []\n"}, "no entries"},
		"empty paths": {map[string]string{"context-map.yaml": "entries:\n  - docs: [\"a.md\"]\n"}, "no paths"},
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
		Entries: []Entry{
			{Name: "server", Paths: []string{"internal/**"}},
			{Name: "mail", Paths: []string{"internal/mail/**"}, Userdocs: []string{"ops/email.md#smtp", "ops/running.md"}},
			{Name: "mail-events", Paths: []string{"internal/mail/events.go"}, Userdocs: []string{"ops/email.md#bounces"}},
			{Name: "webhook", Paths: []string{"internal/webhook/**"}, Userdocs: []string{"ref/webhooks.md"}},
			{Name: "ops", Paths: []string{"ops/**"}, Userdocs: []string{"ops/email.md"}},
		},
	}
	got := m.Impact([]string{
		"internal/webhook/context-map.yaml",
		"internal/mail/smtp.go", "internal/mail/events.go", "internal/mail/smtp_test.go", "internal/webhook/x.gen.go",
		"internal/other.go", "ops/running.md", "ops/email.md",
	})
	want := []Impact{
		{Doc: "ops/email.md", Refs: []string{"ops/email.md", "ops/email.md#bounces", "ops/email.md#smtp"}, Paths: []string{"internal/mail/events.go", "internal/mail/smtp.go", "ops/running.md"}, Changed: true},
		{Doc: "ops/running.md", Refs: []string{"ops/running.md"}, Paths: []string{"internal/mail/events.go", "internal/mail/smtp.go"}, Changed: true},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("Impact =\n%v\nwant\n%v", got, want)
	}
	if got := m.Impact([]string{"internal/webhook/hook.go"}); len(got) != 1 || got[0].Doc != "ref/webhooks.md" || got[0].Changed {
		t.Errorf("Impact of an untouched document = %v", got)
	}
}
