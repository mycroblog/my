// SPDX-License-Identifier: AGPL-3.0-only

package testkit_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/mycroblog/my"

// adaptersWithoutContract are the adapter packages, relative to the module root, whose own tests
// run no port contract, with the reason.
var adaptersWithoutContract = map[string]string{
	"internal/adapter/db/postgres/account":  "the contracts of identity, federation and the other modules run it from their postgres adapters' tests",
	"internal/adapter/db/sqlite/account":    "the contracts of identity, federation and the other modules run it from their sqlite adapters' tests",
	"internal/adapter/db/schemaapply":       "applies the declarative schema; no port",
	"internal/adapter/errorreport/otlp":     "an error report sink, checked against its wire format",
	"internal/adapter/errorreport/sentry":   "an error report sink, checked against its wire format",
	"internal/adapter/plugin/client":        "the plugin protocol client the plugin adapters share; no port",
	"internal/adapter/plugin/contract":      "the cases my plugin test runs against a plugin; no port",
	"internal/adapter/plugin/kind":          "the plugin kinds; no port",
	"internal/adapter/queue/cloudtasks":     "the Waker of the db queue, checked against a fake Cloud Tasks; the db queue runs the wake contract",
	"internal/adapter/telemetry/dbtrace":    "traces the database driver; no port",
	"internal/adapter/valkey":               "the client the valkey adapters share; no port",
	"internal/adapter/voice/turncloudflare": "a TURN credential issuer, checked against the Cloudflare API",
}

// TestEveryAdapterRunsAContract keeps the port contracts of pkg/testkit run (NFR-PORT-3): every
// exported ...Contract function is called by a test or by another contract, and the tests of every
// adapter package call one, unless adaptersWithoutContract names the package.
func TestEveryAdapterRunsAContract(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	contracts := map[string]bool{}     // import path + "." + name -> called
	contractsIn := map[string]string{} // the file declaring each contract
	type file struct {
		rel  string
		test bool
		ast  *ast.File
	}
	var files []file
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "testdata" || rel == "app" || rel == "tools") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return err
		}
		test := strings.HasSuffix(p, "_test.go")
		files = append(files, file{rel: rel, test: test, ast: f})
		if !test && strings.HasPrefix(rel, "pkg/testkit/") {
			for _, decl := range f.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.IsExported() && strings.HasSuffix(fn.Name.Name, "Contract") {
					key := modulePath + "/" + path.Dir(rel) + "." + fn.Name.Name
					contracts[key] = false
					contractsIn[key] = rel
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(contracts) == 0 {
		t.Fatal("found no contract in pkg/testkit")
	}

	runsContract := map[string]bool{} // adapter package directory -> its tests call a contract
	for _, f := range files {
		imports := map[string]string{}
		for _, spec := range f.ast.Imports {
			p, _ := strconv.Unquote(spec.Path.Value)
			name := path.Base(p)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = p
		}
		dir := path.Dir(f.rel)
		ast.Inspect(f.ast, func(n ast.Node) bool {
			var key string
			switch n := n.(type) {
			case *ast.SelectorExpr:
				x, ok := n.X.(*ast.Ident)
				if !ok || imports[x.Name] == "" {
					return true
				}
				key = imports[x.Name] + "." + n.Sel.Name
			case *ast.CallExpr:
				// A contract calling another contract of its own package.
				id, ok := n.Fun.(*ast.Ident)
				if !ok || f.test {
					return true
				}
				key = modulePath + "/" + dir + "." + id.Name
			default:
				return true
			}
			if _, ok := contracts[key]; !ok || contractsIn[key] == f.rel && !f.test {
				return true
			}
			if f.test || strings.HasPrefix(f.rel, "pkg/testkit/") {
				contracts[key] = true
			}
			if f.test {
				runsContract[dir] = true
			}
			return true
		})
	}

	var unused []string
	for key, called := range contracts {
		if !called {
			unused = append(unused, contractsIn[key]+": "+path.Base(key))
		}
	}
	slices.Sort(unused)
	for _, u := range unused {
		t.Errorf("%s is run by no test; run it from the tests of the adapters of its port, or delete it", u)
	}

	adapters := map[string]bool{}
	for _, f := range files {
		if f.test || !strings.HasPrefix(f.rel, "internal/adapter/") || ast.IsGenerated(f.ast) {
			continue
		}
		dir := path.Dir(f.rel)
		base := path.Base(dir)
		// The packages inside an adapter, the generated code, and the test helpers.
		inner := "/" + strings.TrimPrefix(dir, "internal/adapter/") + "/"
		if strings.Contains(inner, "/internal/") || strings.Contains(inner, "/gen/") ||
			strings.HasSuffix(base, "test") || strings.HasPrefix(base, "fake") {
			continue
		}
		adapters[dir] = true
	}
	for _, dir := range slices.Sorted(func(yield func(string) bool) {
		for dir := range adapters {
			if !yield(dir) {
				return
			}
		}
	}) {
		_, excused := adaptersWithoutContract[dir]
		switch {
		case runsContract[dir] && excused:
			t.Errorf("%s runs a port contract; remove it from adaptersWithoutContract", dir)
		case !runsContract[dir] && !excused:
			t.Errorf("%s: no test of the package runs a port contract of pkg/testkit; run the contract of the port it implements (NFR-PORT-3), or name the package and the reason in adaptersWithoutContract", dir)
		}
	}
	for dir := range adaptersWithoutContract {
		if !adapters[dir] {
			t.Errorf("adaptersWithoutContract names %s, which is no adapter package", dir)
		}
	}
}

// moduleRoot returns the directory of the module's go.mod, above the test's directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's directory")
		}
		dir = parent
	}
}
