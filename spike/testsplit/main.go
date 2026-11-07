// SPDX-License-Identifier: AGPL-3.0-only

// Command testsplit writes the file-level dependency graph of the server module as JSON: for every Go file of a
// package of the module (tests included), the files of the module whose package-level declarations it uses, and the
// files a //go:embed of its package embeds. Spike code of #2454; never merged.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type file struct {
	Path    string   `json:"path"`
	Pkg     string   `json:"pkg"`
	Test    bool     `json:"test"`
	Deps    []string `json:"deps"`
	TestFns []string `json:"testFns,omitempty"`
	// FnDeps holds, for each Test function of a test file, the files its body uses.
	FnDeps map[string][]string `json:"fnDeps,omitempty"`
}

func main() {
	root, _ := filepath.Abs(os.Args[1])
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedEmbedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir:   root,
		Tests: true,
		Env:   append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0"),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		panic(err)
	}
	const mod = "github.com/mycroblog/my"
	rel := func(p string) string {
		r, err := filepath.Rel(root, p)
		if err != nil || strings.HasPrefix(r, "..") {
			return ""
		}
		return r
	}
	files := map[string]*file{}
	deps := map[string]map[string]bool{}
	add := func(path, pkg string) *file {
		if f, ok := files[path]; ok {
			return f
		}
		f := &file{Path: path, Pkg: pkg, Test: strings.HasSuffix(path, "_test.go")}
		files[path] = f
		deps[path] = map[string]bool{}
		return f
	}
	seen := map[string]bool{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if !strings.HasPrefix(p.PkgPath, mod) || p.TypesInfo == nil || seen[p.ID] {
			return
		}
		seen[p.ID] = true
		pkg := strings.TrimSuffix(p.PkgPath, "_test")
		var embeds []string
		for _, e := range p.EmbedFiles {
			if r := rel(e); r != "" {
				embeds = append(embeds, r)
			}
		}
		for i, syn := range p.Syntax {
			path := rel(p.CompiledGoFiles[i])
			if path == "" {
				continue
			}
			f := add(path, pkg)
			fs := p.Fset
			for _, cg := range syn.Comments {
				for _, c := range cg.List {
					if strings.HasPrefix(c.Text, "//go:embed") {
						for _, e := range embeds {
							deps[path][e] = true
						}
					}
				}
			}
			if f.Test {
				for _, d := range syn.Decls {
					if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && strings.HasPrefix(fd.Name.Name, "Test") && fd.Name.Name != "TestMain" {
						f.TestFns = append(f.TestFns, fd.Name.Name)
						set := map[string]bool{}
						ast.Inspect(fd, func(n ast.Node) bool {
							id, ok := n.(*ast.Ident)
							if !ok {
								return true
							}
							obj := p.TypesInfo.Uses[id]
							if obj == nil || obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), mod) || !obj.Pos().IsValid() {
								return true
							}
							if obj.Parent() != nil && obj.Parent() != obj.Pkg().Scope() {
								return true
							}
							if r := rel(fs.Position(obj.Pos()).Filename); r != "" && r != path {
								set[r] = true
							}
							return true
						})
						if f.FnDeps == nil {
							f.FnDeps = map[string][]string{}
						}
						for r := range set {
							f.FnDeps[fd.Name.Name] = append(f.FnDeps[fd.Name.Name], r)
						}
						sort.Strings(f.FnDeps[fd.Name.Name])
					}
				}
			}
			ast.Inspect(syn, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj := p.TypesInfo.Uses[id]
				if obj == nil || obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), mod) || !obj.Pos().IsValid() {
					return true
				}
				// Package-level objects, and methods and fields (declared in the file of their type or method).
				if obj.Parent() != nil && obj.Parent() != obj.Pkg().Scope() {
					return true
				}
				if _, isPkg := obj.(*types.PkgName); isPkg {
					return true
				}
				pos := fileOf(fs, obj.Pos(), p)
				if r := rel(pos); r != "" && r != path {
					deps[path][r] = true
				}
				return true
			})
		}
	})
	var out []*file
	for path, f := range files {
		for d := range deps[path] {
			f.Deps = append(f.Deps, d)
		}
		sort.Strings(f.Deps)
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "%d files\n", len(out))
}

// fileOf is the file that declares the object at pos. Objects of other packages carry positions of their own file
// set entries, which packages.Load shares.
func fileOf(fs *token.FileSet, pos token.Pos, _ *packages.Package) string {
	return fs.Position(pos).Filename
}
