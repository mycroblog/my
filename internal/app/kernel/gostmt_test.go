// SPDX-License-Identifier: AGPL-3.0-only

package kernel_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// goStatementAllowed are the files and directories, relative to the module root, whose code may
// start goroutines with a bare go statement: kernel.Go itself and the test helpers.
var goStatementAllowed = []string{
	"internal/app/kernel/errorreport.go",
	"internal/adapter/keymanager/gcpkms/gcpkmstest/",
	"internal/platform/platformtest/",
}

// TestGoStatementsStartThroughGo keeps the goroutines of internal/ and cmd/ on kernel.Go, so
// that their panics are reported and stop the process (ADR-0103 §5). Tests may use go.
func TestGoStatementsStartThroughGo(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	var found []string
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for _, allowed := range goStatementAllowed {
				if rel == allowed || strings.HasSuffix(allowed, "/") && strings.HasPrefix(rel, allowed) {
					return nil
				}
			}
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if g, ok := n.(*ast.GoStmt); ok {
					found = append(found, rel+":"+strconv.Itoa(fset.Position(g.Pos()).Line))
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, at := range found {
		t.Errorf("%s: a bare go statement; start the goroutine with kernel.Go(ctx, name, reporter, fn)", at)
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
