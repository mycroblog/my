// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestStringsCiteNoDecisionRecords keeps ADR and requirement numbers and design documents out of
// what an operator reads: the help texts, log messages, and metric descriptions explain the
// behaviour themselves. It reads every string literal of the commands, which covers the usage text
// of every command, and of the server code that logs and exports metrics.
func TestStringsCiteNoDecisionRecords(t *testing.T) {
	citation := regexp.MustCompile(`\b(ADR|N?FR)-\d+|docs/(adr|requirements|architecture)\b`)
	fset := token.NewFileSet()
	for _, root := range []string{".", "../../internal/app", "../../internal/platform", "../../internal/adapter"} {
		err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(fset, name, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && citation.MatchString(lit.Value) {
					t.Errorf("%s: a string cites %s", fset.Position(lit.Pos()), citation.FindString(lit.Value))
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
