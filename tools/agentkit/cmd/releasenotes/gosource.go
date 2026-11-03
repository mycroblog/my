// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// upgradeStep is a dbupgrade.Step registered in dataUpgrades.
type upgradeStep struct {
	key, description string
	runs             bool
	projections      []string
}

// prerequisite is a dbupgrade.Prerequisite registered in dataUpgrades.
type prerequisite struct{ key, via string }

// retiredSetting is an entry of the config package's retiredKeys.
type retiredSetting struct{ key, renamedTo, removed string }

// parseUpgrades reads the steps and the prerequisites from the DataUpgrades literal that dataUpgrades returns. They
// are read as source because the binary of a previous release cannot be asked; anything but literals is an error,
// since a step the parser cannot see would otherwise be reported as "None".
func parseUpgrades(path string) ([]upgradeStep, []prerequisite, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	lit, err := dataUpgradesLiteral(file)
	if err != nil {
		return nil, nil, err
	}
	var (
		steps   []upgradeStep
		prereqs []prerequisite
	)
	err = eachField(fset, lit, func(name string, value ast.Expr) error {
		items, err := elements(fset, value)
		if err != nil {
			return err
		}
		for _, item := range items {
			switch name {
			case "Steps":
				s, err := parseStep(fset, item)
				if err != nil {
					return err
				}
				steps = append(steps, s)
			case "Prerequisites":
				var p prerequisite
				if err := stringFields(fset, item, map[string]*string{"Key": &p.key, "Via": &p.via}); err != nil {
					return err
				}
				prereqs = append(prereqs, p)
			}
		}
		return nil
	})
	return steps, prereqs, err
}

// dataUpgradesLiteral finds the DataUpgrades literal that the function dataUpgrades returns.
func dataUpgradesLiteral(file *ast.File) (*ast.CompositeLit, error) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "dataUpgrades" {
			continue
		}
		for _, stmt := range fn.Body.List {
			ret, ok := stmt.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			if lit, ok := ret.Results[0].(*ast.CompositeLit); ok && isIdent(lit.Type, "DataUpgrades") {
				return lit, nil
			}
		}
		return nil, errors.New("dataUpgrades does not return a DataUpgrades literal")
	}
	return nil, errors.New("no function dataUpgrades")
}

func parseStep(fset *token.FileSet, lit *ast.CompositeLit) (upgradeStep, error) {
	var s upgradeStep
	err := eachField(fset, lit, func(field string, v ast.Expr) error {
		var err error
		switch field {
		case "Key":
			s.key, err = stringValue(fset, v)
		case "Description":
			s.description, err = stringValue(fset, v)
		case "Run":
			s.runs = !isIdent(v, "nil")
		case "Projections":
			list, ok := v.(*ast.CompositeLit)
			if !ok {
				return notLiteral(fset, v)
			}
			for _, elt := range list.Elts {
				name, err := stringValue(fset, elt)
				if err != nil {
					return err
				}
				s.projections = append(s.projections, name)
			}
		}
		return err
	})
	return s, err
}

// parseRetired reads the entries of the package variable retiredKeys.
func parseRetired(path string) ([]retiredSetting, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	spec := retiredKeysSpec(file)
	if spec == nil {
		return nil, errors.New("no variable retiredKeys")
	}
	if len(spec.Values) == 0 {
		return nil, nil
	}
	items, err := elements(fset, spec.Values[0])
	if err != nil {
		return nil, err
	}
	retired := make([]retiredSetting, 0, len(items))
	for _, item := range items {
		var r retiredSetting
		err := stringFields(fset, item, map[string]*string{"key": &r.key, "renamedTo": &r.renamedTo, "removed": &r.removed})
		if err != nil {
			return nil, err
		}
		retired = append(retired, r)
	}
	return retired, nil
}

func retiredKeysSpec(file *ast.File) *ast.ValueSpec {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) == 1 && vs.Names[0].Name == "retiredKeys" {
				return vs
			}
		}
	}
	return nil
}

// elements are the composite literals inside the literal of a list.
func elements(fset *token.FileSet, e ast.Expr) ([]*ast.CompositeLit, error) {
	list, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil, notLiteral(fset, e)
	}
	items := make([]*ast.CompositeLit, 0, len(list.Elts))
	for _, elt := range list.Elts {
		item, ok := elt.(*ast.CompositeLit)
		if !ok {
			return nil, notLiteral(fset, elt)
		}
		items = append(items, item)
	}
	return items, nil
}

// stringFields reads the string fields named in fields from a keyed composite literal.
func stringFields(fset *token.FileSet, lit *ast.CompositeLit, fields map[string]*string) error {
	return eachField(fset, lit, func(name string, v ast.Expr) error {
		dst, ok := fields[name]
		if !ok {
			return nil
		}
		var err error
		*dst, err = stringValue(fset, v)
		return err
	})
}

// eachField calls fn with the field name and the value of each element of a keyed composite literal.
func eachField(fset *token.FileSet, lit *ast.CompositeLit, fn func(name string, value ast.Expr) error) error {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			return notLiteral(fset, elt)
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return notLiteral(fset, kv.Key)
		}
		if err := fn(key.Name, kv.Value); err != nil {
			return err
		}
	}
	return nil
}

// stringValue is the value of a string literal, or of literals joined with +.
func stringValue(fset *token.FileSet, e ast.Expr) (string, error) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strconv.Unquote(v.Value)
		}
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			x, err := stringValue(fset, v.X)
			if err != nil {
				return "", err
			}
			y, err := stringValue(fset, v.Y)
			return x + y, err
		}
	}
	return "", notLiteral(fset, e)
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func notLiteral(fset *token.FileSet, e ast.Node) error {
	return fmt.Errorf("line %d: not a literal, which scripts/release-notes.sh cannot read", fset.Position(e.Pos()).Line)
}
