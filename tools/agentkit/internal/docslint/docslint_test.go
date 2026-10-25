// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func lint(t *testing.T, c *Config, files map[string]string) []string {
	t.Helper()
	root := t.TempDir()
	var paths []string
	for p, body := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	fs, err := Lint(root, paths, c)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Path + ":" + strconv.Itoa(f.Line) + ":" + f.Rule
	}
	return out
}

func TestLinks(t *testing.T) {
	got := lint(t, &Config{}, map[string]string{
		"README.md": strings.Join([]string{
			"# Title",
			"[ok](docs/a.md) [ok](docs/a.md#4-リモートの-actor--取得) [ok](docs/a.md#usage-1) [ok](#title)",
			"[dir](docs) [root](/docs/a.md#usage) [ext](https://example.com/x.md) [mail](mailto:a@example.com)",
			"[missing](docs/b.md)",         // 4
			"[bad anchor](docs/a.md#nope)", // 5
			"[outside](../x.md)",           // 6
			"`[in code](nope.md)` <!-- [comment](nope.md) -->",
			"```",
			"[fenced](nope.md)",
			"```",
			"[ref]: docs/c.md", // 11
			"[encoded](docs/a.md#%E4%BD%BF%E3%81%84%E6%96%B9) [code anchor](docs/a.md#code-heading) [link anchor](docs/a.md#linked-heading)",
			"[go](main.go#L10) [self bad](#nope)", // 13
		}, "\n"),
		"docs/a.md": strings.Join([]string{
			"# 4. リモートの Actor / 取得",
			"## Usage",
			"## Usage",
			"## 使い方",
			"## `code` heading",
			"## [Linked](https://example.com) heading",
			"```md",
			"## not a heading",
			"```",
		}, "\n"),
		"main.go": "package main\n",
	})
	want := []string{"README.md:4:link", "README.md:5:link", "README.md:6:link", "README.md:11:link", "README.md:13:link"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

func TestJapanese(t *testing.T) {
	c := &Config{English: []string{"*.md", "deploy/**/*.yaml"}}
	got := lint(t, c, map[string]string{
		"README.md": strings.Join([]string{
			"# Guide",
			"English only, with `日本語` in code and [a link](docs/ja.md#使い方).",
			"これは日本語です。",                     // 3
			"Full-width punctuation（here）.", // 4
			"<!-- docs-lint: allow-japanese -->",
			"The button reads ログイン.",
			"Allowed inline ログイン <!-- docs-lint: allow-japanese -->",
			"```text",
			"メンテナンス中です",
			"```",
			"<!-- 日本語のコメント -->", // 11
		}, "\n"),
		"docs/ja.md":                 "# 使い方\n\n日本語の設計文書。\n",
		"deploy/docker/compose.yaml": "# 開発環境\nservices: {}\n", // 1
		"deploy/docker/other.txt":    "日本語\n",
		"docs/adr/0001-x.md":         "# ADR-0001: 日本語\n",
	})
	want := []string{"README.md:3:japanese", "README.md:4:japanese", "README.md:11:japanese", "deploy/docker/compose.yaml:1:japanese"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

func TestDesignLinks(t *testing.T) {
	c := &Config{Operator: []string{"docs/operations/**"}}
	got := lint(t, c, map[string]string{
		"docs/operations/run.md": strings.Join([]string{
			"# Run",
			"See [the guide](other.md) and `ADR-0001` in code.",
			"Decided in [ADR-0001](../adr/0001-x.md).",                    // 3
			"See [the model](../architecture/model.md).",                  // 4
			"Tracked in https://github.com/mycroblog/my/issues/12.",       // 5
			"Fixed by [the PR](https://github.com/mycroblog/my/pull/34).", // 6
			"Per FR-101.", // 7
			"(#123)",      // 8
			"See [main](https://github.com/mycroblog/my/blob/main/docs/adr/0001-x.md).", // 9
			"Upstream [issue](https://github.com/other/repo/issues/1) and colour #fff.",
		}, "\n"),
		"docs/operations/other.md":   "# Other\n",
		"docs/adr/0001-x.md":         "# ADR-0001\n",
		"docs/architecture/model.md": "# Model\n",
		"docs/agents/x.md":           "[ADR-0001](../adr/0001-x.md)\n",
	})
	want := []string{
		"docs/operations/run.md:3:design-link", "docs/operations/run.md:3:design-link",
		"docs/operations/run.md:4:design-link", "docs/operations/run.md:5:design-link",
		"docs/operations/run.md:6:design-link", "docs/operations/run.md:7:design-link",
		"docs/operations/run.md:8:design-link", "docs/operations/run.md:9:design-link",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

func TestExclude(t *testing.T) {
	c := &Config{Operator: []string{"docs/reference/**", "!docs/reference/commands.md"}}
	got := lint(t, c, map[string]string{
		"docs/reference/commands.md": "Tasks run with go-task (ADR-0080).\n",
		"docs/reference/config.md":   "Decided in ADR-0010.\n",
	})
	want := []string{"docs/reference/config.md:1:design-link"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

func TestIndex(t *testing.T) {
	c := &Config{}
	c.Index.File = "docs/README.md"
	c.Index.Files = []string{"docs/operations/*.md", "docs/reference/*.md"}
	got := lint(t, c, map[string]string{
		"docs/README.md":             "- [Running](operations/running.md)\n- [Email](./operations/email.md#smtp)\n",
		"docs/operations/running.md": "# Running\n",
		"docs/operations/email.md":   "# Email\n\n## SMTP\n",
		"docs/operations/alerts.md":  "# Alerts\n",
		"docs/agents/workflow.md":    "# Workflow\n",
	})
	want := []string{"docs/operations/alerts.md:0:index"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

func TestPending(t *testing.T) {
	c := &Config{English: []string{"docs/operations/**"}, Operator: []string{"docs/operations/**"}}
	c.Pending.Japanese = []string{"docs/operations/ja.md", "docs/operations/en.md", "docs/operations/gone.md", "README.md"}
	c.Pending.DesignLinks = []string{"docs/operations/ja.md", "docs/operations/en.md"}
	got := lint(t, c, map[string]string{
		"README.md":             "# Readme\n",
		"docs/operations/ja.md": "# 運用\n\n[ADR-0001](../adr/0001-x.md)\n",
		"docs/operations/en.md": "# Operation\n",
		"docs/adr/0001-x.md":    "# ADR-0001\n",
	})
	sort.Strings(got)
	// en.md is listed twice but clean; gone.md does not exist; README.md is outside the rule.
	want := []string{"docs/lint.yaml:0:pending", "docs/lint.yaml:0:pending", "docs/lint.yaml:0:pending", "docs/lint.yaml:0:pending"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}
}

func TestSkip(t *testing.T) {
	c := &Config{Skip: []string{"**/node_modules/**"}, English: []string{"**/*.md"}}
	got := lint(t, c, map[string]string{"app/node_modules/x/README.md": "日本語 [x](missing.md)\n"})
	if len(got) != 0 {
		t.Errorf("findings = %v, want none", got)
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	body := "english: [\"*.md\"]\npending:\n  japanese: [docs/operations/a.md]\n"
	if err := os.WriteFile(filepath.Join(root, File), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.English, []string{"*.md"}) || !reflect.DeepEqual(c.Pending.Japanese, []string{"docs/operations/a.md"}) {
		t.Errorf("Load = %+v", c)
	}
	if err := os.WriteFile(filepath.Join(root, File), []byte("unknown: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Error("Load accepted an unknown key")
	}
}

func TestStripCode(t *testing.T) {
	for in, want := range map[string]string{
		"a `b` c":         "a   c",
		"a ``b ` c`` d":   "a   d",
		"unclosed ` tick": "unclosed ` tick",
		"``` x":           "``` x",
		"`a` and `b`":     "  and  ",
	} {
		if got := stripCode(in); got != want {
			t.Errorf("stripCode(%q) = %q, want %q", in, got, want)
		}
	}
}
