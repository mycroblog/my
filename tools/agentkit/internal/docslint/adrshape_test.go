// SPDX-License-Identifier: AGPL-3.0-only

package docslint

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// shapeRepo writes a repository with a Taskfile, Go packages, and the ADRs given, and returns its root and paths.
func shapeRepo(t *testing.T, adrs map[string]string) (string, []string) {
	t.Helper()
	files := map[string]string{
		"Taskfile.yml":              "version: \"3\"\nincludes:\n  docs: tasks/docs.yml\ntasks:\n  check:\n    desc: all\n",
		"tasks/docs.yml":            "version: \"3\"\ntasks:\n  rules:\n    desc: the rules\n",
		"internal/kernel/kernel.go": "package kernel\n",
		"api/spec/main.tsp":         "",
		"scripts/task.sh":           "",
	}
	for p, body := range adrs {
		files[p] = body
	}
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
	return root, paths
}

func TestArtifactKind(t *testing.T) {
	root, paths := shapeRepo(t, nil)
	a, err := newArtifacts(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	for span, want := range map[string]string{
		// The implementation.
		"internal/app/content/service.go": "a path",
		"internal/adapter/db/<dialect>":   "a path",
		"./internal/app/content":          "a path",
		"COPY internal/app/x /x":          "a path",
		"scripts/task.sh docs:rules":      "a file",
		"api/openapi.yaml":                "a file",
		"slow-suite.yml":                  "a file",
		"routes/admin.*.tsx":              "a file",
		"adapters_no_search_<name>.go":    "a file",
		"docs:rules":                      "a task",
		"task docs:rules":                 "a task",
		"kernel.HTTPClient":               "an identifier",
		"kernel.Store.Get(ctx, id)":       "an identifier",
		"ErrNotFound":                     "an identifier",
		// Layers and components, which a decision may be about.
		"internal/domain": "",
		"pkg/mykit/":      "",
		"cmd/":            "",
		// Terms of the product, the protocol, and the platform.
		"public":                       "",
		"local":                        "",
		"Retry-After":                  "",
		"Content-Type: text/css":       "",
		"Create":                       "",
		"Update(Actor)":                "",
		"EmojiReact":                   "",
		"contentMap":                   "",
		"created_at":                   "",
		"manage_settings":              "",
		"MY_DATABASE_URL":              "",
		"CGO_ENABLED=0":                "",
		"my serve":                     "",
		"check":                        "",
		"allow:db-destructive":         "",
		"type:bug":                     "",
		"GET /api/v1/statuses/{id}":    "",
		"/internal/app/x":              "",
		"https://example.com/a/b.js":   "",
		"example.com/static/beacon.js": "",
		"golang.org/x/net/html":        "",
		"@my/ui":                       "",
		"--config=a.yaml":              "",
		"robots.txt":                   "",
		"go.mod":                       "",
		"package.json":                 "",
		"http.DefaultClient":           "",
		"Intl.DateTimeFormat":          "",
		"my.job.attempt":               "",
		"v0.1.0":                       "",
	} {
		if got := a.kind(span); got != want {
			t.Errorf("kind(%q) = %q, want %q", span, got, want)
		}
	}
}

func TestADRShape(t *testing.T) {
	rows := func(n int) string { return "| a | b |\n| --- | --- |\n" + strings.Repeat("| 1 | 2 |\n", n) }
	decision := "# ADR-0001: x\n\n## Decision（決定）\n\n"
	root, paths := shapeRepo(t, map[string]string{
		// The shape of a decision: the limits themselves, a fenced block, and product terms.
		"docs/adr/0001-ok.md": decision + "`public` and `internal/domain`.\n\n" + rows(ADRMaxTableRows) +
			"\n```\ninternal/app/content/service.go\n| not | a | table |\n```\n\n## 採らなかった案\n\n" + strings.Repeat("あ", 5000),
		"docs/adr/0002-names.md": decision + "The port `kernel.HTTPClient` in `internal/kernel/http.go`,\nrun by `docs:rules`.\n",
		"docs/adr/0003-table.md": decision + "text\n\n" + rows(ADRMaxTableRows+1),
		"docs/adr/0004-long.md":  decision + strings.Repeat("あ", ADRMaxDecisionChars) + "\n\n## 経緯\n\n" + strings.Repeat("あ", 2000),
		"docs/adr/0005-old.md":   decision + "`scripts/task.sh`\n",
		"docs/adr/0006-done.md":  decision + "done\n",
		"docs/adr/template.md":   "`scripts/task.sh`\n",
	})
	c := &Config{}
	c.Pending.ADRShape = []string{"docs/adr/0005-old.md", "docs/adr/0006-done.md", "docs/adr/0007-gone.md"}
	fs, err := ADRs(root, paths, c)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range fs {
		got = append(got, f.String())
	}
	names := ": " + adrHolds + "; the names of files, functions, tasks, and keys belong to the code, so name the concept"
	want := []string{
		"docs/adr/0002-names.md:5: adr-shape: `kernel.HTTPClient` names an identifier of the implementation" + names,
		"docs/adr/0002-names.md:5: adr-shape: `internal/kernel/http.go` names a path of the implementation" + names,
		"docs/adr/0002-names.md:6: adr-shape: `docs:rules` names a task of the implementation" + names,
		"docs/adr/0003-table.md:7: adr-shape: a table of 9 rows, over 8: a list of jobs, keys, or tables transcribes the code; keep what was decided about them",
		"docs/adr/0004-long.md: adr-shape: 6041 characters, over the 6000 of an ADR: " + adrHolds + "; what the code says stays in the code",
		"docs/adr/0004-long.md: adr-shape: the Decision section has 4002 characters, over 4000: state what was decided and why; how it is built is in the code",
		"docs/lint.yaml: pending: pending.adr-shape lists docs/adr/0006-done.md, which has the shape of an ADR now; remove it",
		"docs/lint.yaml: pending: pending.adr-shape lists docs/adr/0007-gone.md, which does not exist; remove it",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	shapes, err := ADRShapes(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 6 || shapes[0].Breaks || shapes[0].TableRows != ADRMaxTableRows || !shapes[1].Breaks || shapes[1].Artifacts != 3 {
		t.Errorf("ADRShapes = %+v", shapes)
	}
}
