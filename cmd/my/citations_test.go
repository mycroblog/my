// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestStringsCiteNoDecisionRecords keeps ADR and requirement numbers out of what an operator reads:
// the help texts and messages explain the behaviour themselves. It reads every string literal of the
// commands, which covers the usage text of every command.
func TestStringsCiteNoDecisionRecords(t *testing.T) {
	citation := regexp.MustCompile(`\b(ADR|N?FR)-\d+`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && citation.MatchString(lit.Value) {
				t.Errorf("%s: a string cites %s", fset.Position(lit.Pos()), citation.FindString(lit.Value))
			}
			return true
		})
	}
}
