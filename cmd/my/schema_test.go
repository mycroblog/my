// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/platform/config"
)

func schemaCmd(args []string, out, errOut *bytes.Buffer) int { return runSchema(args, out, errOut) }

func TestSchemaApply(t *testing.T) {
	newAdminEnv(t)
	path := filepath.Join(t.TempDir(), "new.db")
	t.Setenv(config.EnvPrefix+"DATABASE_URL", path)
	// It reads only the database's settings.
	t.Setenv(config.EnvPrefix+"HOST", "")
	t.Setenv(config.EnvPrefix+"KEK", "")

	code, stdout, stderr := runCmd(t, schemaCmd, "apply", "-dry-run")
	if code != 0 || !strings.HasPrefix(stdout, "CREATE TABLE") || !strings.HasSuffix(stdout, "none applied (dry run)\n") {
		t.Fatalf("schema apply -dry-run = %d %q %q", code, stdout, stderr)
	}
	if n := countTables(t, path); n != 0 {
		t.Errorf("the dry run created %d table(s)", n)
	}

	if code, stdout, stderr := runCmd(t, schemaCmd, "apply"); code != 0 || !strings.HasSuffix(stdout, "statement(s) applied\n") {
		t.Fatalf("schema apply = %d %q %q", code, stdout, stderr)
	}
	if n := countTables(t, path); n == 0 {
		t.Error("schema apply created no table")
	}
	if code, stdout, stderr := runCmd(t, schemaCmd, "apply"); code != 0 || stdout != "ok: the database matches the schema; nothing to apply\n" {
		t.Errorf("schema apply again = %d %q %q", code, stdout, stderr)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE probe_extra (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCmd(t, schemaCmd, "apply"); code != 1 || !strings.Contains(stderr, "the plan drops table probe_extra") {
		t.Errorf("schema apply of a drop = %d %q", code, stderr)
	}
	if code, stdout, stderr := runCmd(t, schemaCmd, "apply", "-dry-run"); code != 0 || !strings.Contains(stdout, "needs -allow-destructive") {
		t.Errorf("schema apply -dry-run of a drop = %d %q %q", code, stdout, stderr)
	}
	if code, stdout, stderr := runCmd(t, schemaCmd, "apply", "-allow-destructive"); code != 0 || !strings.HasSuffix(stdout, "statement(s) applied\n") {
		t.Errorf("schema apply -allow-destructive = %d %q %q", code, stdout, stderr)
	}

	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{nil, 2, "Usage:"},
		{[]string{"diff"}, 2, `unknown subcommand "diff"`},
		{[]string{"apply", "now"}, 2, "want 0 argument(s)"},
	} {
		if code, _, stderr := runCmd(t, schemaCmd, tc.args...); code != tc.code || !strings.Contains(stderr, tc.want) {
			t.Errorf("schema %v = %d %q; want %d containing %q", tc.args, code, stderr, tc.code, tc.want)
		}
	}
}

// countTables returns the number of tables in the SQLite database at path.
func countTables(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sqlite_schema WHERE type = 'table'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
