// SPDX-License-Identifier: AGPL-3.0-only

// Command txscan lists calls in a function with a uow.Tx parameter that pass a context but not the
// transaction. Each is a candidate for a read or write on a second connection. It sees direct
// calls only: a helper called without tx that itself queries is listed at the call, not followed.
// The same call appears twice when a closure with tx nests in a function with tx.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	fset := token.NewFileSet()
	for _, root := range os.Args[1:] {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.FuncLit:
					scan(fset, v.Type, v.Body)
				case *ast.FuncDecl:
					if v.Body != nil {
						scan(fset, v.Type, v.Body)
					}
				}
				return true
			})
			return nil
		})
	}
}

func uses(e ast.Expr, name string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return true
	})
	return found
}

func scan(fset *token.FileSet, t *ast.FuncType, body *ast.BlockStmt) {
	tx := ""
	for _, p := range t.Params.List {
		if sel, ok := p.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Tx" && len(p.Names) == 1 {
			tx = p.Names[0].Name
		}
	}
	if tx == "" {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if id, ok := call.Args[0].(*ast.Ident); !ok || id.Name != "ctx" {
			return true
		}
		for _, a := range call.Args {
			if uses(a, tx) {
				return true
			}
		}
		name := exprName(call.Fun)
		if name == tx+".Emit" || strings.Contains(name, "Logger") || strings.HasPrefix(name, "slog.") {
			return true
		}
		p := fset.Position(call.Pos())
		fmt.Printf("%s:%d %s\n", p.Filename, p.Line, name)
		return true
	})
}

func exprName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return exprName(v.X) + "." + v.Sel.Name
	case *ast.Ident:
		return v.Name
	case *ast.CallExpr:
		return exprName(v.Fun) + "()"
	}
	return "?"
}
