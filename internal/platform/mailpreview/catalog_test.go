// SPDX-License-Identifier: AGPL-3.0-only

package mailpreview

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	appnotification "github.com/mycroblog/my/internal/app/notification"
	"github.com/mycroblog/my/internal/domain/notification"
)

// module is the import path of the repository's Go module.
const module = "github.com/mycroblog/my"

// repoRoot is the repository's root from this package's directory.
var repoRoot = filepath.Join("..", "..", "..")

// mailJobName matches the name of the constant of a job that sends an email.
var mailJobName = regexp.MustCompile(`^Job\w*Email$`)

// mailSources are what the application layer declares about its emails: the job types (the
// string constants named Job...Email) and the draft functions (the exported functions that
// return a kernel.MailDraft), the latter as "<import path>.<name>".
func mailSources(t *testing.T) (jobTypes, drafts []string) {
	t.Helper()
	fset := token.NewFileSet()
	dir := filepath.Join(repoRoot, "internal", "app")
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := module + "/" + filepath.ToSlash(rel)
		for _, decl := range f.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				if decl.Tok != token.CONST {
					continue
				}
				for _, spec := range decl.Specs {
					v := spec.(*ast.ValueSpec)
					for i, name := range v.Names {
						if !mailJobName.MatchString(name.Name) || i >= len(v.Values) {
							continue
						}
						if lit, ok := v.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							s, err := strconv.Unquote(lit.Value)
							if err != nil {
								return err
							}
							jobTypes = append(jobTypes, s)
						}
					}
				}
			case *ast.FuncDecl:
				if decl.Recv != nil || !decl.Name.IsExported() || decl.Type.Results == nil {
					continue
				}
				for _, r := range decl.Type.Results.List {
					if sel, ok := r.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "MailDraft" {
						if x, ok := sel.X.(*ast.Ident); ok && x.Name == "kernel" {
							drafts = append(drafts, pkg+"."+decl.Name.Name)
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return jobTypes, drafts
}

// catalogCalls returns the functions that catalog.go calls in other packages, as
// "<import path>.<name>".
func catalogCalls(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "catalog.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[string]string{}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imports[name] = path
	}
	var calls []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if x, ok := sel.X.(*ast.Ident); ok && imports[x.Name] != "" {
				calls = append(calls, imports[x.Name]+"."+sel.Sel.Name)
			}
		}
		return true
	})
	return calls
}

// TestCatalogHasEveryEmail fails when an email of the server is not in the catalog: a mail job
// type that no entry has, a draft function that the catalog does not call, or a variant of the
// notification emails that no entry has. Add the email to Catalog (catalog.go), then run
// `scripts/task.sh mail:golden`.
func TestCatalogHasEveryEmail(t *testing.T) {
	jobTypes, drafts := mailSources(t)
	if len(jobTypes) == 0 || len(drafts) == 0 {
		t.Fatal("found no mail job type or draft function in internal/app")
	}
	catalog := Catalog()
	for _, j := range jobTypes {
		if !slices.ContainsFunc(catalog, func(e Entry) bool { return e.JobType == j }) {
			t.Errorf("the mail job %s has no entry in Catalog (catalog.go)", j)
		}
	}
	calls := catalogCalls(t)
	for _, d := range drafts {
		if !slices.Contains(calls, d) {
			t.Errorf("Catalog (catalog.go) does not call the draft function %s", d)
		}
	}

	bundle, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	l := bundle.Localizer("en")
	has := func(job string, t notification.Type) bool {
		return slices.ContainsFunc(catalog, func(e Entry) bool {
			return e.JobType == job && (e.Variant == string(t) || strings.HasPrefix(e.Variant, string(t)+"-"))
		})
	}
	for _, typ := range notification.Types {
		if _, ok := appnotification.ModeratorEmailDraft(l, sampleName, sampleTo, typ, sampleWebOrigin, ""); ok && !has(appnotification.JobModeratorEmail, typ) {
			t.Errorf("the moderator email of %s has no entry in Catalog (catalog.go)", typ)
		}
		if _, ok := appnotification.SupporterEmailDraft(l, sampleName, sampleTo, appnotification.SupporterMail{Type: typ}, sampleWebOrigin); ok && !has(appnotification.JobSupporterEmail, typ) {
			t.Errorf("the supporter email of %s has no entry in Catalog (catalog.go)", typ)
		}
	}
}

// TestCatalogNames keeps the entries' names, the stems of their files, unique and safe as file
// names.
func TestCatalogNames(t *testing.T) {
	safe := regexp.MustCompile(`^[a-z0-9_.-]+$`)
	seen := map[string]bool{}
	for _, e := range Catalog() {
		if !safe.MatchString(e.Name()) {
			t.Errorf("%q is not a safe file name", e.Name())
		}
		if seen[e.Name()] {
			t.Errorf("two entries are named %s", e.Name())
		}
		seen[e.Name()] = true
	}
}

// TestRenderEveryLocale composes every entry in every locale of the preview.
func TestRenderEveryLocale(t *testing.T) {
	bundle, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range Locales {
		rendered, err := Render(bundle, locale)
		if err != nil {
			t.Fatal(err)
		}
		if len(rendered) != len(Catalog()) {
			t.Errorf("%s: %d emails, want %d", locale, len(rendered), len(Catalog()))
		}
	}
}

// TestWrite writes the preview and its index.
func TestWrite(t *testing.T) {
	dir := t.TempDir()
	index, err := Write(filepath.Join(dir, "preview"), Locales)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(index) != "index.html" {
		t.Errorf("index = %s", index)
	}
	for _, locale := range Locales {
		for _, e := range Catalog() {
			for _, ext := range []string{".html", ".dark.html", ".txt"} {
				if _, err := fs.Stat(os.DirFS(filepath.Join(dir, "preview")), locale+"/"+e.Name()+ext); err != nil {
					t.Error(err)
				}
			}
		}
	}
}
