// SPDX-License-Identifier: AGPL-3.0-only

// Package sqlitetest gives tests SQLite databases with the product schema applied (ADR-0015):
// the modules of the schema that the test binary links (package ddl, ADR-0153 §3). A test binary
// links a module by importing the database adapter of the module or the module's package of
// schema, so a change to one module's files reruns only the tests that use the module.
//
// Executing the schema outside a transaction commits each of its several hundred statements on its
// own, and SQLite syncs the file after every commit: seconds for one database, more on a loaded
// machine. ApplySchema executes it in one transaction instead, and NewFile and Template copy a
// database file that the test binary makes once.
package sqlitetest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/mycroblog/my/schema/ddl"
)

// ApplySchema executes ddl, the product schema (schema.SQLite), a part of it, or a variant of it,
// on db in one transaction.
func ApplySchema(ctx context.Context, db *sql.DB, ddl string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, ddl); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// template is the content of a database file with the linked modules' schema applied, made once
// per test binary.
var template struct {
	once sync.Once
	b    []byte
	err  error
}

func createTemplate() {
	dir, err := os.MkdirTemp("", "sqlitetest-")
	if err != nil {
		template.err = err
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "my.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		template.err = err
		return
	}
	if err := ApplySchema(context.Background(), db, ddl.SQLite(ddl.Linked())); err != nil {
		_ = db.Close()
		template.err = fmt.Errorf("apply schema: %w", err)
		return
	}
	if err := db.Close(); err != nil {
		template.err = err
		return
	}
	template.b, template.err = os.ReadFile(path) //nolint:gosec // G304: the file this function just created
}

// Template returns the content of a SQLite database file with the schema of every module the test
// binary links applied (ddl.Linked), in name order. Every call returns the same bytes, which the
// caller must not change.
func Template() ([]byte, error) {
	template.once.Do(createTemplate)
	return template.b, template.err
}

// NewFile writes a copy of Template to my.db in a new temporary directory of t and returns its
// path.
func NewFile(t testing.TB) string {
	t.Helper()
	b, err := Template()
	if err != nil {
		t.Fatalf("sqlitetest: %v", err)
	}
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.WriteFile("my.db", b, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "my.db")
}
