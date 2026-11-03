// SPDX-License-Identifier: AGPL-3.0-only

package schema_test

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/app/supporters"
	"github.com/mycroblog/my/internal/domain/finance"
	"github.com/mycroblog/my/internal/domain/instance"
	"github.com/mycroblog/my/schema"
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
		t.Fatal("schema/sqlite declares no tables")
	}
	if !slices.Equal(sqlite, postgres) {
		t.Errorf("tables differ:\n sqlite:   %v\n postgres: %v", sqlite, postgres)
	}
}

// SQLite and Postgres are the files of their directories concatenated in name order, the order in
// which Atlas executes them.
func TestDialectsConcatenateTheirFilesInNameOrder(t *testing.T) {
	dirFS := os.DirFS(".")
	for dir, got := range map[string]string{"sqlite": schema.SQLite, "postgres": schema.Postgres} {
		names, err := fs.Glob(dirFS, dir+"/*.sql")
		if err != nil || len(names) == 0 {
			t.Fatalf("%s: no files (%v)", dir, err)
		}
		slices.Sort(names)
		var want strings.Builder
		for _, name := range names {
			data, err := fs.ReadFile(dirFS, name)
			if err != nil {
				t.Fatal(err)
			}
			want.Write(data)
		}
		if got != want.String() {
			t.Errorf("%s: the embedded schema is not the files concatenated in name order", dir)
		}
	}
}

// Every dialect has the same files, and each file declares the same tables in every dialect, so
// that a module's tables are in the file of the same name.
func TestFilesMatchAcrossDialects(t *testing.T) {
	dirFS := os.DirFS(".")
	sqlite, _ := fs.Glob(dirFS, "sqlite/*.sql")
	postgres, _ := fs.Glob(dirFS, "postgres/*.sql")
	for i := range sqlite {
		sqlite[i] = path.Base(sqlite[i])
	}
	for i := range postgres {
		postgres[i] = path.Base(postgres[i])
	}
	if !slices.Equal(sqlite, postgres) {
		t.Fatalf("files differ:\n sqlite:   %v\n postgres: %v", sqlite, postgres)
	}
	for _, name := range sqlite {
		s, err := fs.ReadFile(dirFS, "sqlite/"+name)
		if err != nil {
			t.Fatal(err)
		}
		p, err := fs.ReadFile(dirFS, "postgres/"+name)
		if err != nil {
			t.Fatal(err)
		}
		if a, b := schema.Tables(string(s)), schema.Tables(string(p)); !slices.Equal(a, b) {
			t.Errorf("%s: tables differ:\n sqlite:   %v\n postgres: %v", name, a, b)
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
	var sql strings.Builder
	files, err := fs.Glob(os.DirFS("sqlite"), "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		b, err := fs.ReadFile(os.DirFS("sqlite"), name)
		if err != nil {
			t.Fatal(err)
		}
		sql.Write(b)
	}
	table := regexp.MustCompile(`(?ms)^CREATE TABLE (\w+) \(.*?^\)([^;]*);`)
	matches := table.FindAllStringSubmatch(sql.String(), -1)
	if len(matches) == 0 {
		t.Fatal("found no tables")
	}
	for _, m := range matches {
		if strings.TrimSpace(m[2]) != "STRICT" {
			t.Errorf("the table %s is not STRICT", m[1])
		}
	}
}
