// SPDX-License-Identifier: AGPL-3.0-only

package grep

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mycroblog/my/tools/agentkit/internal/ctxmap"
	"github.com/mycroblog/my/tools/agentkit/internal/repo"
)

func fixture(t *testing.T) *repo.Repo {
	t.Helper()
	root := t.TempDir()
	long := strings.Repeat("x", 300) + " needle " + strings.Repeat("y", 300)
	files := map[string]string{
		ctxmap.File:                         "ignore:\n  - \"**/*.gen.go\"\n  - \"api/openapi.yaml\"\nentries: []\n",
		".gitignore":                        ".tmp/\nnode_modules/\n",
		"internal/app/note/service.go":      "package note\n\n// Service creates notes.\ntype Service struct{}\n\n// Create makes a note.\nfunc (s *Service) Create(text string) error {\n\tif text == \"\" {\n\t\treturn errEmpty\n\t}\n\treturn nil\n}\n\nfunc helper() { _ = \"Create\" }\n",
		"internal/app/note/service_test.go": "package note\n\nfunc TestCreate(t *testing.T) { Create() }\n",
		"internal/app/note/zz.gen.go":       "package note\n\nfunc CreateGenerated() {}\n",
		"api/openapi.yaml":                  "operationId: createNote\n",
		"app/web/src/note.ts":               "export function createNote(): void {\n  call(\"Create\");\n}\n",
		"app/web/src/note.test.ts":          "test(\"Create\", () => {});\n",
		"docs/notes.md":                     "# Notes\n\n## 連合\n\n投稿は連合先へ配送します。Create を送ります。\n\n## Other\n\n" + long + "\n",
		"go.sum":                            "example.com/Create v1.0.0 h1:abc=\n",
		"bin.dat":                           "Create\x00\x01\x02",
		".tmp/out.txt":                      "Create in scratch\n",
		"app/node_modules/x/index.js":       "Create\n",
		"many.txt":                          strings.Repeat("Create line\n", 100),
	}
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	r, err := repo.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func grep(t *testing.T, r *repo.Repo, dir string, args ...string) (string, bool) {
	t.Helper()
	o, err := ParseArgs(args)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	var out bytes.Buffer
	found, err := Run(r, filepath.Join(r.Root, dir), o, &out)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String(), found
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want Options
	}{
		{[]string{"foo"}, Options{Pattern: "foo", Limit: 40, Max: 6000}},
		{[]string{"-rn", "foo", "internal", "docs"}, Options{Pattern: "foo", Paths: []string{"internal", "docs"}, Limit: 40, Max: 6000}},
		{[]string{"-rniE", "a|b", "--include=*.go", "."}, Options{Pattern: "a|b", Paths: []string{"."}, IgnoreCase: true, Include: []string{"*.go"}, Limit: 40, Max: 6000}},
		{[]string{"foo", "-n", "5", "--kind", "go,ts", "-w"}, Options{Pattern: "foo", Word: true, Kinds: []string{"go", "ts"}, Limit: 5, Max: 6000}},
		{[]string{"-n", "5", "foo", "x"}, Options{Pattern: "foo", Paths: []string{"x"}, Limit: 5, Max: 6000}},
		{[]string{"-n", "404"}, Options{Pattern: "404", Limit: 40, Max: 6000}},
		{[]string{"-n", "func X", "-A12", "f.go"}, Options{Pattern: "func X", Paths: []string{"f.go"}, Context: 12, Limit: 40, Max: 6000}},
		{[]string{"-C", "2", "-e", "-x", "--max", "900", "--tests"}, Options{Pattern: "-x", Context: 2, Tests: true, Limit: 40, Max: 900}},
		{[]string{"-Fl", "a(b", "--only-tests", "--gen", "-t", "go"}, Options{Pattern: "a(b", Fixed: true, List: true, Tests: true, OnlyTests: true, Gen: true, Kinds: []string{"go"}, Limit: 40, Max: 6000}},
		{[]string{"-oq", "--", "-v"}, Options{Pattern: "-v", Only: true, Quiet: true, Limit: 40, Max: 6000}},
	}
	for _, c := range cases {
		got, err := ParseArgs(c.args)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v:\n got %+v (%v)\nwant %+v", c.args, got, err, c.want)
		}
	}
	for _, bad := range [][]string{{}, {"-v", "x"}, {"--nope", "x"}, {"x", "--kind", "rust"}, {"x", "-C"}, {""}} {
		if _, err := ParseArgs(bad); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
}

func TestRun(t *testing.T) {
	r := fixture(t)
	has := func(name, out string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: no %q in\n%s", name, w, out)
			}
		}
	}
	lacks := func(name, out string, not ...string) {
		t.Helper()
		for _, w := range not {
			if strings.Contains(out, w) {
				t.Errorf("%s: unexpected %q in\n%s", name, w, out)
			}
		}
	}

	out, found := grep(t, r, "", "Create", "--kind", "go,ts")
	if !found {
		t.Fatal("no match")
	}
	has("sources", out,
		"internal/app/note/service.go (3)\n", "  6: // Create makes a note.  [Service.Create 6-12]\n  7: func (s *Service) Create(text string) error {\n",
		"  14: func helper() { _ = \"Create\" }  [helper 14]\n"[:20], "app/web/src/note.ts\n  2: call(\"Create\");  [createNote 1-3]\n",
		"note: 2 more in 2 test files (--tests)", "note: 1 more in 1 generated files (--gen)")
	lacks("sources", out, "service_test.go", "zz.gen.go", "go.sum", "bin.dat", "node_modules", ".tmp/", "docs/")

	out, _ = grep(t, r, "", "Create", "--tests", "--gen", "-l")
	has("everything", out, "many.txt (100)\n", "internal/app/note/service_test.go (1)\n", "internal/app/note/zz.gen.go (1)\n", "go.sum (1)\n")
	lacks("everything", out, "bin.dat", "node_modules", ".tmp/", "note:")

	out, _ = grep(t, r, "", "TestCreate")
	has("only in tests", out, "internal/app/note/service_test.go\n  3: func TestCreate", "note: no match outside tests")
	out, _ = grep(t, r, "", "CreateGenerated")
	has("only generated", out, "zz.gen.go", "note: no match in sources")
	out, _ = grep(t, r, "", "Create", "--only-tests")
	has("only tests", out, "service_test.go", "note.test.ts")
	lacks("only tests", out, "service.go")

	// A file named on the command line is searched whatever it is; a directory is searched from where the command runs.
	out, _ = grep(t, r, "", "Create", "go.sum", ".tmp/out.txt")
	has("named files", out, "go.sum\n", ".tmp/out.txt\n  1: Create in scratch")
	out, _ = grep(t, r, "internal/app", "-rn", "errEmpty", "note")
	has("from a subdirectory", out, "internal/app/note/service.go\n  9: return errEmpty  [Service.Create 6-12]")
	out, _ = grep(t, r, "app/web", "createNote")
	has("the working directory", out, "app/web/src/note.ts")
	lacks("the working directory", out, "openapi")
	out, found = grep(t, r, "", "Create", "nowhere")
	if found {
		t.Error("a match in a path that does not exist")
	}
	has("missing path", out, "no match", "note: nowhere: no tracked file")

	// Patterns.
	out, _ = grep(t, r, "", `errEmpty\|helper()`, "internal")
	has("basic alternation", out, "  9: return errEmpty", "  14: func helper()", `note: read \| as alternation`)
	out, _ = grep(t, r, "", "helper(", "internal")
	has("not a regular expression", out, "  14: func helper()", "searched as a fixed string")
	out, _ = grep(t, r, "", "-i", "-w", "create", "app/web/src/note.ts")
	has("word", out, "  2: call(\"Create\");")
	lacks("word", out, "createNote()")
	out, _ = grep(t, r, "", "-F", "_ = \"Create\"")
	has("fixed", out, "  14: ")
	out, _ = grep(t, r, "", `^func \(s \*Service\) Create\(`)
	has("anchors", out, "  7: func (s *Service) Create")
	out, _ = grep(t, r, "", `\s+return nil`)
	has("whitespace over a line end", out, "  11: return nil")

	// Japanese text and long lines.
	out, _ = grep(t, r, "", "連合先へ配送", "docs")
	has("japanese", out, "docs/notes.md\n  5: 投稿は連合先へ配送します。Create を送ります。  [#連合 3-6]\n")
	out, _ = grep(t, r, "", "needle", "docs")
	has("long line", out, "…", " needle ")
	if len(out) > 400 {
		t.Errorf("a long line was not cut: %d characters", len(out))
	}

	// Limits.
	out, _ = grep(t, r, "", "Create", "many.txt", "internal", "-n", "8")
	has("limit", out, "many.txt (100)\n", "  … 97 more here\n", "of 103 matches in 2 files shown", "Narrow with a path")
	out, _ = grep(t, r, "", "Create", "many.txt", "--max", "200", "-n", "1000")
	if n := strings.Count(out, "Create line"); n < 5 || n > 20 {
		t.Errorf("--max did not bound the output: %d lines", n)
	}
	out, _ = grep(t, r, "", "-C", "1", "errEmpty", "internal")
	has("context", out, "  8-   if text == \"\" {\n  9:     return errEmpty  [Service.Create 6-12]\n  10-   }\n")
	out, _ = grep(t, r, "", "-o", `Create\w*`, "--gen", "--tests")
	has("only matching", out, "  CreateGenerated\n", "  Create\n", "distinct texts")
	if _, found = grep(t, r, "", "-q", "nothing-like-this"); found {
		t.Error("-q found something")
	}
	if out, found = grep(t, r, "", "-q", "errEmpty"); !found || out != "" {
		t.Errorf("-q: %v %q", found, out)
	}
}
