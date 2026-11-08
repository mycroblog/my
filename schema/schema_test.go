// SPDX-License-Identifier: AGPL-3.0-only

package schema_test

import (
	"database/sql"
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

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
	"github.com/mycroblog/my/schema"
	"github.com/mycroblog/my/schema/ddl"
)

func TestTables(t *testing.T) {
	got := schema.Tables("-- CREATE TABLE commented (\nCREATE TABLE users (\n  id TEXT\n) STRICT;\n" +
		"CREATE TABLE IF NOT EXISTS \"jobs\" (\n);\nCREATE INDEX users_id ON users (id);\n")
	if want := []string{"users", "jobs"}; !slices.Equal(got, want) {
		t.Errorf("Tables = %v, want %v", got, want)
	}
}

// Both dialects declare the same tables, so that readiness means the same on each.
func TestTablesMatchAcrossDialects(t *testing.T) {
	sqlite, postgres := schema.Tables(schema.SQLite), schema.Tables(schema.Postgres)
	if len(sqlite) == 0 {
		t.Fatal("the schema declares no tables")
	}
	if !slices.Equal(sqlite, postgres) {
		t.Errorf("tables differ:\n sqlite:   %v\n postgres: %v", sqlite, postgres)
	}
}

// moduleDirs returns the directories <NN>-<module> of the schema, in name order.
func moduleDirs(t *testing.T) []string {
	t.Helper()
	dirs, err := fs.Glob(os.DirFS("."), "[0-9][0-9]-*")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no module directories (%v)", err)
	}
	slices.Sort(dirs)
	return dirs
}

// Modules is every directory <NN>-<module>, in name order, each with the files of the directory;
// SQLite and Postgres are those files concatenated in name order, the order in which Atlas
// executes them; and the binary links no other module.
func TestModulesAreTheDirectoriesInNameOrder(t *testing.T) {
	dirs := moduleDirs(t)
	var names []string
	for _, m := range schema.Modules {
		names = append(names, m.Name)
	}
	if !slices.Equal(names, dirs) {
		t.Fatalf("Modules = %v, want the directories %v", names, dirs)
	}
	if linked := ddl.Linked(); !slices.Equal(linked, schema.Modules) {
		t.Errorf("Linked differs from Modules")
	}
	var sqlite, postgres strings.Builder
	for i, dir := range dirs {
		for name, got := range map[string]string{"sqlite.sql": schema.Modules[i].SQLite, "postgres.sql": schema.Modules[i].Postgres} {
			want, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // G304: a file of this directory
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("%s: the embedded %s is not the file", dir, name)
			}
		}
		sqlite.WriteString(schema.Modules[i].SQLite)
		postgres.WriteString(schema.Modules[i].Postgres)
	}
	if schema.SQLite != sqlite.String() || schema.Postgres != postgres.String() {
		t.Error("SQLite and Postgres are not the files of the modules concatenated in name order")
	}
}

// Each module has its file in every dialect, and both declare the same tables in the same order,
// so that a module's tables are in its own directory.
func TestFilesMatchAcrossDialects(t *testing.T) {
	for _, dir := range moduleDirs(t) {
		names, err := fs.Glob(os.DirFS(dir), "*.sql")
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"postgres.sql", "sqlite.sql"}; !slices.Equal(names, want) {
			t.Errorf("%s: files %v, want %v", dir, names, want)
		}
	}
	for _, m := range schema.Modules {
		if a, b := schema.Tables(m.SQLite), schema.Tables(m.Postgres); !slices.Equal(a, b) {
			t.Errorf("%s: tables differ:\n sqlite:   %v\n postgres: %v", m.Name, a, b)
		}
	}
}

// A test binary applies only the modules it links (pkg/testkit/sqlitetest): every module, with
// the modules it depends on, applies in name order on an empty database.
func TestEachModuleAppliesWithItsDependencies(t *testing.T) {
	for _, m := range schema.Modules {
		t.Run(m.Name, func(t *testing.T) {
			t.Parallel()
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "my.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			if err := sqlitetest.ApplySchema(t.Context(), db, ddl.SQLite(m.Closure())); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// infrastructure is the module of the tables that adapters own, which the shared packages
// kernel, uow, and queue of internal/app use.
const infrastructure = "10-infrastructure"

// A module requires exactly the modules that its package of internal/app imports, and
// 10-infrastructure when it imports kernel, uow, or queue, so that a test of the module links
// the tables its repositories may read.
func TestRequiresMatchTheImportsOfTheApplicationLayer(t *testing.T) {
	const prefix = "github.com/mycroblog/my/internal/app/"
	byModule := map[string]string{}
	for _, m := range schema.Modules {
		byModule[strings.SplitN(m.Name, "-", 2)[1]] = m.Name
	}
	for _, m := range schema.Modules {
		if m.Name == infrastructure {
			if len(m.Requires) != 0 {
				t.Errorf("%s requires %d modules, want none", m.Name, len(m.Requires))
			}
			continue
		}
		module := strings.SplitN(m.Name, "-", 2)[1]
		files, err := filepath.Glob(filepath.Join("..", "internal", "app", module, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("%s: no package internal/app/%s (%v)", m.Name, module, err)
		}
		want := map[string]bool{}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range file.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				dep, ok := strings.CutPrefix(p, prefix)
				if !ok || strings.Contains(dep, "/") {
					continue
				}
				switch dep {
				case "kernel", "uow", "queue":
					want[infrastructure] = true
				default:
					if name, ok := byModule[dep]; ok && name != m.Name {
						want[name] = true
					}
				}
			}
		}
		var got []string
		for _, r := range m.Requires {
			got = append(got, r.Name)
		}
		slices.Sort(got)
		wantNames := make([]string, 0, len(want))
		for name := range want {
			wantNames = append(wantNames, name)
		}
		slices.Sort(wantNames)
		if !slices.Equal(got, wantNames) {
			t.Errorf("%s requires %v, want the imports of internal/app/%s: %v", m.Name, got, module, wantNames)
		}
	}
}

// The CHECK constraint of instance_theme_tokens.token lists exactly the tokens a theme may override,
// which are generated from the web client's theme.css (ADR-0059).
func TestThemeTokenCheckMatchesTheTokenList(t *testing.T) {
	var want []string
	for _, tok := range instance.ThemeTokens() {
		want = append(want, tok.Name)
	}
	check := regexp.MustCompile(`(?s)CREATE TABLE instance_theme_tokens \(.*?token [^\n]*CHECK \(token IN \((.*?)\)\)`)
	for name, sql := range map[string]string{"sqlite": schema.SQLite, "postgres": schema.Postgres} {
		m := check.FindStringSubmatch(sql)
		if m == nil {
			t.Fatalf("%s: no CHECK on instance_theme_tokens.token", name)
		}
		var got []string
		for _, q := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1) {
			got = append(got, q[1])
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: CHECK lists %v, want %v", name, got, want)
		}
	}
}

// The CHECK constraint of cost_item_label_revisions.category lists exactly the categories of the
// finance domain (ADR-0067 §2).
func TestCostCategoryCheckMatchesTheDomain(t *testing.T) {
	var want []string
	for _, c := range finance.Categories() {
		want = append(want, string(c))
	}
	check := regexp.MustCompile(`(?s)CREATE TABLE cost_item_label_revisions \(.*?category [^\n]*CHECK \(category IN \((.*?)\)\)`)
	for name, sql := range map[string]string{"sqlite": schema.SQLite, "postgres": schema.Postgres} {
		m := check.FindStringSubmatch(sql)
		if m == nil {
			t.Fatalf("%s: no CHECK on cost_item_label_revisions.category", name)
		}
		var got []string
		for _, q := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1) {
			got = append(got, q[1])
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: CHECK lists %v, want %v", name, got, want)
		}
	}
}

// The CHECK constraint of payment_reconcile_runs.error_kind lists exactly the kinds of failure of
// the supporters module (ADR-0074 §4).
func TestPaymentErrorKindCheckMatchesTheModule(t *testing.T) {
	var want []string
	for _, k := range supporters.ErrorKinds() {
		want = append(want, string(k))
	}
	check := regexp.MustCompile(`(?s)CREATE TABLE payment_reconcile_runs \(.*?error_kind [^\n]*CHECK \(error_kind IN \((.*?)\)\)`)
	for name, sql := range map[string]string{"sqlite": schema.SQLite, "postgres": schema.Postgres} {
		m := check.FindStringSubmatch(sql)
		if m == nil {
			t.Fatalf("%s: no CHECK on payment_reconcile_runs.error_kind", name)
		}
		var got []string
		for _, q := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1) {
			got = append(got, q[1])
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s: CHECK lists %v, want %v", name, got, want)
		}
	}
}

// TestSQLiteTablesAreStrict checks that every SQLite table declares STRICT, which a table cannot
// gain later without being rebuilt.
func TestSQLiteTablesAreStrict(t *testing.T) {
	table := regexp.MustCompile(`(?ms)^CREATE TABLE (\w+) \(.*?^\)([^;]*);`)
	matches := table.FindAllStringSubmatch(schema.SQLite, -1)
	if len(matches) == 0 {
		t.Fatal("found no tables")
	}
	for _, m := range matches {
		if strings.TrimSpace(m[2]) != "STRICT" {
			t.Errorf("the table %s is not STRICT", m[1])
		}
	}
}
