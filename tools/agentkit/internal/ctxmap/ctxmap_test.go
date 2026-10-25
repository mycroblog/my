// SPDX-License-Identifier: AGPL-3.0-only

package ctxmap

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestCheck(t *testing.T) {
	root := t.TempDir()
	for p, c := range map[string]string{"doc.md": "# Title\n\n## 1. Rules\n", "ops/a.md": "# A\n\n## Setup\n", "ops/b.md": "# B\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := &Map{
		Ignore:   []string{"**/*.gen.go"},
		Strict:   []string{"internal/app/*"},
		Userdocs: Userdocs{Cover: []string{"ops/*.md"}},
		Entries: []Entry{
			{Name: "go", Paths: []string{"internal/**"}, Docs: []string{"doc.md#1-rules"}, Checks: []string{"go"}, Userdocs: []string{"ops/a.md#setup"}},
			{Name: "content", Paths: []string{"internal/*/content/**"}, Docs: []string{"doc.md#missing"}, Userdocs: []string{"ops/a.md#teardown", "ops/gone.md"}},
			{Name: "docs", Paths: []string{"*.md", "ops/*.md"}},
		},
	}
	files := []string{"doc.md", "internal/app/content/a.go", "internal/app/graph/b.go", "internal/app/graph/c.gen.go", "README.txt", "ops/a.md", "ops/b.md"}
	var got []string
	for _, p := range m.Check(root, files, []string{"go"}) {
		got = append(got, p.Path+" "+p.Msg)
	}
	want := []string{
		"./ no entry covers README.txt",
		"internal/app/graph/ matches strict pattern",
		"entry \"content\": doc.md has no heading #missing",
		"entry \"content\": userdocs: ops/a.md has no heading #teardown",
		"entry \"content\": userdocs: ops/gone.md does not exist",
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
