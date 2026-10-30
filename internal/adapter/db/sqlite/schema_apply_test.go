// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/schema"
)

// newEmptyFile returns the path of a database file that does not exist yet.
func newEmptyFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "apply.db")
}

// mustApply runs applySchema and fails t on an error.
func mustApply(t *testing.T, path, ddl string, opts schemaapply.Options) schemaapply.Result {
	t.Helper()
	r, err := applySchema(t.Context(), path, ddl, opts)
	if err != nil {
		t.Fatalf("applySchema: %v", err)
	}
	return r
}

// objects lists the tables and the columns of the database at path, "t" and "t.c".
func objects(t *testing.T, path string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(t.Context(),
		"SELECT m.name, p.name FROM sqlite_schema m, pragma_table_info(m.name) p WHERE m.type = 'table' ORDER BY 1, 2")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var tbl, col string
		if err := rows.Scan(&tbl, &col); err != nil {
			t.Fatal(err)
		}
		if len(out) == 0 || !strings.HasPrefix(out[len(out)-1], tbl+".") {
			out = append(out, tbl)
		}
		out = append(out, tbl+"."+col)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// assertSyncedWithCLI checks with the Atlas CLI that the database at path matches
// schema/sqlite. It skips the check when the CLI is not installed (scripts/task.sh db:atlas, or
// the ATLAS variable).
func assertSyncedWithCLI(t *testing.T, path string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	atlas := os.Getenv("ATLAS")
	if atlas == "" {
		atlas = filepath.Join(root, "bin", "atlas")
	}
	if _, err := os.Stat(atlas); err != nil { //nolint:gosec // G703: the CLI the developer or CI installed
		t.Logf("the Atlas CLI is not installed (%v); the comparison with atlas schema diff is skipped", err)
		return
	}
	out, err := exec.CommandContext(t.Context(), atlas, "schema", "diff", //nolint:gosec // G204: the CLI the developer or CI installed
		"--from", "sqlite://"+path, "--to", "file://"+filepath.Join(root, "schema", "sqlite"),
		"--dev-url", "sqlite://dev?mode=memory").CombinedOutput()
	if err != nil {
		t.Fatalf("atlas schema diff: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Schemas are synced") {
		t.Errorf("atlas schema diff after the apply:\n%s", out)
	}
}

func TestApplySchemaToAnEmptyDatabase(t *testing.T) {
	path := newEmptyFile(t)
	r, err := ApplySchema(t.Context(), path, schemaapply.Options{})
	if err != nil {
		t.Fatalf("ApplySchema: %v", err)
	}
	if !r.Applied || len(r.Statements) == 0 || len(r.Drops) > 0 {
		t.Fatalf("ApplySchema on an empty database = %+v, want the statements applied with nothing dropped", r)
	}
	assertSyncedWithCLI(t, path)
	db, err := Open(t.Context(), path, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Ready(t.Context()); err != nil {
		t.Errorf("Ready after the apply: %v", err)
	}

	again, err := ApplySchema(t.Context(), path, schemaapply.Options{})
	if err != nil {
		t.Fatalf("ApplySchema again: %v", err)
	}
	if again.Applied || len(again.Statements) > 0 {
		t.Errorf("ApplySchema on an applied database = %+v, want nothing to do", again)
	}
}

func TestApplySchemaAddsAColumn(t *testing.T) {
	path := newEmptyFile(t)
	mustApply(t, path, schema.SQLite, schemaapply.Options{})
	table := schema.Tables(schema.SQLite)[0]
	r := mustApply(t, path, schema.SQLite+"\nALTER TABLE "+table+" ADD COLUMN probe_added TEXT;\n", schemaapply.Options{})
	if !r.Applied || len(r.Drops) > 0 {
		t.Fatalf("applying an added column = %+v, want it applied with nothing dropped", r)
	}
	if !slices.Contains(objects(t, path), table+".probe_added") {
		t.Errorf("the column %s.probe_added is missing after the apply", table)
	}
}

func TestApplySchemaRefusesDropsUnlessAllowed(t *testing.T) {
	path := newEmptyFile(t)
	table := schema.Tables(schema.SQLite)[0]
	mustApply(t, path, schema.SQLite+"\nCREATE TABLE probe_extra (id INTEGER PRIMARY KEY);\nALTER TABLE "+table+" ADD COLUMN probe_extra TEXT;\n", schemaapply.Options{})
	before := objects(t, path)

	r, err := ApplySchema(t.Context(), path, schemaapply.Options{})
	var de *schemaapply.DestructiveError
	if !errors.As(err, &de) {
		t.Fatalf("ApplySchema of drops without AllowDestructive: err = %v, want a DestructiveError", err)
	}
	if want := []string{"table probe_extra", "column " + table + ".probe_extra"}; !sameElements(de.Drops, want) {
		t.Errorf("Drops = %q, want %q", de.Drops, want)
	}
	if r.Applied {
		t.Error("Applied is true for a refused plan")
	}
	if after := objects(t, path); !slices.Equal(after, before) {
		t.Errorf("the refused apply changed the database:\nbefore %q\nafter  %q", before, after)
	}

	r, err = ApplySchema(t.Context(), path, schemaapply.Options{AllowDestructive: true})
	if err != nil || !r.Applied {
		t.Fatalf("ApplySchema with AllowDestructive = %+v, %v; want it applied", r, err)
	}
	after := objects(t, path)
	if slices.Contains(after, "probe_extra") || slices.Contains(after, table+".probe_extra") {
		t.Errorf("the dropped table and column remain: %q", after)
	}
	assertSyncedWithCLI(t, path)
}

func TestApplySchemaDryRunChangesNothing(t *testing.T) {
	path := newEmptyFile(t)
	r, err := ApplySchema(t.Context(), path, schemaapply.Options{DryRun: true})
	if err != nil {
		t.Fatalf("ApplySchema dry run: %v", err)
	}
	if r.Applied || len(r.Statements) == 0 {
		t.Fatalf("dry run = %+v, want the plan's statements and nothing applied", r)
	}
	if !strings.HasPrefix(r.Statements[0], "CREATE TABLE") {
		t.Errorf("the first statement of the plan is %q, want a CREATE TABLE", r.Statements[0])
	}
	if got := objects(t, path); len(got) > 0 {
		t.Errorf("the dry run created %q", got)
	}
}

func TestApplySchemaRollsBackAFailedPlan(t *testing.T) {
	path := newEmptyFile(t)
	mustApply(t, path, schema.SQLite, schemaapply.Options{})
	table := schema.Tables(schema.SQLite)[0]
	// A row that a new NOT NULL column without a default cannot take: the table's rebuild fails,
	// after the plan has added the other column.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.WithoutCancel(t.Context())
	if _, err := db.ExecContext(ctx, "CREATE TABLE zz_probe_rows (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO zz_probe_rows (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	_, err = applySchema(t.Context(), path, schema.SQLite+"\nCREATE TABLE zz_probe_rows (id INTEGER PRIMARY KEY, v TEXT NOT NULL);\nALTER TABLE "+table+" ADD COLUMN probe_added TEXT;\n", schemaapply.Options{})
	if err == nil {
		t.Fatal("applySchema of a column that the rows cannot take succeeded")
	}
	t.Logf("the failed apply: %v", err)
	if got := objects(t, path); slices.Contains(got, table+".probe_added") || slices.Contains(got, "zz_probe_rows.v") {
		t.Errorf("the failed apply left a part of the plan: %q", got)
	}
}

// sameElements reports whether a and b hold the same strings in any order.
func sameElements(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}
