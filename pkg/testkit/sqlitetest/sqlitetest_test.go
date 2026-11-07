// SPDX-License-Identifier: AGPL-3.0-only

package sqlitetest_test

import (
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
	"github.com/mycroblog/my/schema"
)

func tables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	return names
}

func open(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNewFileHasEveryTableOfTheSchema(t *testing.T) {
	t.Parallel()
	want := schema.Tables(schema.SQLite)
	slices.Sort(want)
	a, b := sqlitetest.NewFile(t), sqlitetest.NewFile(t)
	if a == b {
		t.Fatalf("two calls returned the same path %s", a)
	}
	for _, path := range []string{a, b} {
		if got := tables(t, open(t, path)); !slices.Equal(got, want) {
			t.Errorf("tables of %s = %v, want %v", path, got, want)
		}
	}
}

func TestApplySchemaRollsBackOnAnError(t *testing.T) {
	t.Parallel()
	db := open(t, filepath.Join(t.TempDir(), "my.db"))
	err := sqlitetest.ApplySchema(t.Context(), db, "CREATE TABLE a (id INTEGER PRIMARY KEY);\nCREATE TABLE a (id INTEGER);")
	if err == nil {
		t.Fatal("ApplySchema with a duplicate table succeeded")
	}
	if got := tables(t, db); len(got) != 0 {
		t.Errorf("tables after a failed ApplySchema = %v, want none", got)
	}
}

// BenchmarkNewFile measures the copy of the template that most tests pay for; the first call also
// makes the template.
func BenchmarkNewFile(b *testing.B) {
	for b.Loop() {
		sqlitetest.NewFile(b)
	}
}

// BenchmarkApplySchema measures the schema applied in one transaction to a new file.
func BenchmarkApplySchema(b *testing.B) {
	for b.Loop() {
		db, err := sql.Open("sqlite", filepath.Join(b.TempDir(), "my.db"))
		if err != nil {
			b.Fatal(err)
		}
		if err := sqlitetest.ApplySchema(b.Context(), db, schema.SQLite); err != nil {
			b.Fatal(err)
		}
		_ = db.Close()
	}
}
