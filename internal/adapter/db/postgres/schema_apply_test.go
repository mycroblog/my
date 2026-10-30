// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/mycroblog/my/internal/adapter/db/schemaapply"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
	"github.com/mycroblog/my/schema"
)

// query returns the first column of the rows of sql on the database at url.
func query(t *testing.T, url, sql string) []string {
	t.Helper()
	ctx := context.WithoutCancel(t.Context())
	c, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(ctx) }()
	rows, err := c.Query(ctx, sql)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// columns lists the tables and the columns of the public schema, "t" and "t.c".
func columns(t *testing.T, url string) []string {
	t.Helper()
	return query(t, url, `SELECT table_name::text FROM information_schema.tables WHERE table_schema = 'public'
		UNION ALL SELECT table_name || '.' || column_name FROM information_schema.columns WHERE table_schema = 'public'
		ORDER BY 1`)
}

// tempSchemas lists the temporary schemas of ApplySchema left in the database at url.
func tempSchemas(t *testing.T, url string) []string {
	t.Helper()
	return query(t, url, "SELECT nspname::text FROM pg_namespace WHERE nspname LIKE '"+devSchemaPrefix+"%'")
}

func mustApply(t *testing.T, url, ddl string, opts schemaapply.Options) schemaapply.Result {
	t.Helper()
	r, err := applySchema(t.Context(), url, "", ddl, opts)
	if err != nil {
		t.Fatalf("applySchema: %v", err)
	}
	return r
}

// assertSyncedWithCLI checks with the Atlas CLI that the database at url matches
// schema/postgres, using devURL as its dev database. It skips the check when the CLI is not
// installed (scripts/task.sh db:atlas, or the ATLAS variable).
func assertSyncedWithCLI(t *testing.T, url, devURL string) {
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
	// The CLI's dev database needs the extensions of the bootstrap SQL.
	c, err := pgx.Connect(t.Context(), devURL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Exec(t.Context(), schema.PostgresBootstrap)
	_ = c.Close(context.WithoutCancel(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	scoped := func(u string) string { return u + "&search_path=public" }
	out, err := exec.CommandContext(t.Context(), atlas, "schema", "diff", //nolint:gosec // G204: the CLI the developer or CI installed
		"--from", scoped(url), "--to", "file://"+filepath.Join(root, "schema", "postgres"),
		"--dev-url", scoped(devURL)).CombinedOutput()
	if err != nil {
		t.Fatalf("atlas schema diff: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Schemas are synced") {
		t.Errorf("atlas schema diff after the apply:\n%s", out)
	}
}

func TestApplySchemaToAnEmptyDatabase(t *testing.T) {
	url := pgtest.NewEmptyDatabase(t)
	r, err := ApplySchema(t.Context(), url, "", schemaapply.Options{})
	if err != nil {
		t.Fatalf("ApplySchema: %v", err)
	}
	if !r.Applied || len(r.Statements) == 0 || len(r.Drops) > 0 {
		t.Fatalf("ApplySchema on an empty database = %+v, want the statements applied with nothing dropped", r)
	}
	if got := query(t, url, "SELECT extname::text FROM pg_extension WHERE extname = 'pg_trgm'"); len(got) != 1 {
		t.Error("the bootstrap SQL did not run: pg_trgm is missing")
	}
	if got := tempSchemas(t, url); len(got) > 0 {
		t.Errorf("temporary schemas remain: %q", got)
	}
	assertSyncedWithCLI(t, url, pgtest.NewEmptyDatabase(t))
	db, err := Open(t.Context(), url, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ready(t.Context()); err != nil {
		t.Errorf("Ready after the apply: %v", err)
	}

	again, err := ApplySchema(t.Context(), url, "", schemaapply.Options{})
	if err != nil {
		t.Fatalf("ApplySchema again: %v", err)
	}
	if again.Applied || len(again.Statements) > 0 {
		t.Errorf("ApplySchema on an applied database = %+v, want nothing to do", again)
	}
}

func TestApplySchemaWithADevDatabase(t *testing.T) {
	url, devURL := pgtest.NewEmptyDatabase(t), pgtest.NewEmptyDatabase(t)
	r, err := ApplySchema(t.Context(), url, devURL, schemaapply.Options{})
	if err != nil || !r.Applied {
		t.Fatalf("ApplySchema with a dev database = %+v, %v; want it applied", r, err)
	}
	if got := tempSchemas(t, devURL); len(got) > 0 {
		t.Errorf("temporary schemas remain in the dev database: %q", got)
	}
	if got := tempSchemas(t, url); len(got) > 0 {
		t.Errorf("the target holds a temporary schema: %q", got)
	}
	if got := columns(t, devURL); len(got) > 0 {
		t.Errorf("the dev database's public schema holds %q", got)
	}
	again, err := ApplySchema(t.Context(), url, devURL, schemaapply.Options{})
	if err != nil || len(again.Statements) > 0 {
		t.Errorf("ApplySchema again = %+v, %v; want nothing to do", again, err)
	}
}

func TestApplySchemaAddsAColumn(t *testing.T) {
	url := pgtest.NewEmptyDatabase(t)
	mustApply(t, url, schema.Postgres, schemaapply.Options{})
	table := schema.Tables(schema.Postgres)[0]
	r := mustApply(t, url, schema.Postgres+"\nALTER TABLE "+table+" ADD COLUMN probe_added text;\n", schemaapply.Options{})
	if !r.Applied || len(r.Drops) > 0 {
		t.Fatalf("applying an added column = %+v, want it applied with nothing dropped", r)
	}
	if !slices.Contains(columns(t, url), table+".probe_added") {
		t.Errorf("the column %s.probe_added is missing after the apply", table)
	}
}

func TestApplySchemaRefusesDropsUnlessAllowed(t *testing.T) {
	url := pgtest.NewEmptyDatabase(t)
	table := schema.Tables(schema.Postgres)[0]
	mustApply(t, url, schema.Postgres+"\nCREATE TABLE probe_extra (id bigint PRIMARY KEY);\nALTER TABLE "+table+" ADD COLUMN probe_extra text;\n", schemaapply.Options{})
	before := columns(t, url)

	r, err := ApplySchema(t.Context(), url, "", schemaapply.Options{})
	var de *schemaapply.DestructiveError
	if !errors.As(err, &de) {
		t.Fatalf("ApplySchema of drops without AllowDestructive: err = %v, want a DestructiveError", err)
	}
	if want := []string{"table probe_extra", "column " + table + ".probe_extra"}; !slices.Equal(slices.Sorted(slices.Values(de.Drops)), slices.Sorted(slices.Values(want))) {
		t.Errorf("Drops = %q, want %q", de.Drops, want)
	}
	if r.Applied {
		t.Error("Applied is true for a refused plan")
	}
	if after := columns(t, url); !slices.Equal(after, before) {
		t.Errorf("the refused apply changed the database:\nbefore %q\nafter  %q", before, after)
	}

	r, err = ApplySchema(t.Context(), url, "", schemaapply.Options{AllowDestructive: true})
	if err != nil || !r.Applied {
		t.Fatalf("ApplySchema with AllowDestructive = %+v, %v; want it applied", r, err)
	}
	after := columns(t, url)
	if slices.Contains(after, "probe_extra") || slices.Contains(after, table+".probe_extra") {
		t.Errorf("the dropped table and column remain: %q", after)
	}
}

func TestApplySchemaDryRunChangesNothing(t *testing.T) {
	url := pgtest.NewEmptyDatabase(t)
	r, err := ApplySchema(t.Context(), url, "", schemaapply.Options{DryRun: true})
	if err != nil {
		t.Fatalf("ApplySchema dry run: %v", err)
	}
	if r.Applied || len(r.Statements) == 0 {
		t.Fatalf("dry run = %+v, want the plan's statements and nothing applied", r)
	}
	if got := columns(t, url); len(got) > 0 {
		t.Errorf("the dry run created %q", got)
	}
	if got := tempSchemas(t, url); len(got) > 0 {
		t.Errorf("temporary schemas remain: %q", got)
	}
}

func TestApplySchemaDropsTheTemporarySchemaOnFailure(t *testing.T) {
	url := pgtest.NewEmptyDatabase(t)
	if _, err := applySchema(t.Context(), url, "", schema.Postgres+"\nCREATE TABLE probe_broken (id no_such_type);\n", schemaapply.Options{}); err == nil {
		t.Fatal("applySchema of a broken schema succeeded")
	}
	if got := tempSchemas(t, url); len(got) > 0 {
		t.Errorf("temporary schemas remain after the failure: %q", got)
	}
	if got := columns(t, url); len(got) > 0 {
		t.Errorf("the failed run created %q", got)
	}
}

func TestApplySchemaRollsBackAFailedPlan(t *testing.T) {
	url := pgtest.NewEmptyDatabase(t)
	mustApply(t, url, schema.Postgres, schemaapply.Options{})
	table := schema.Tables(schema.Postgres)[0]
	ctx := context.WithoutCancel(t.Context())
	c, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(ctx) }()
	// A row that a new NOT NULL column without a default cannot take, after the plan has added
	// the other column.
	if _, err := c.Exec(ctx, "CREATE TABLE zz_probe_rows (id bigint PRIMARY KEY); INSERT INTO zz_probe_rows VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	_, err = applySchema(t.Context(), url, "", schema.Postgres+
		"\nCREATE TABLE zz_probe_rows (id bigint PRIMARY KEY, v text NOT NULL);\nALTER TABLE "+table+" ADD COLUMN probe_added text;\n",
		schemaapply.Options{})
	if err == nil {
		t.Fatal("applySchema of a column that the rows cannot take succeeded")
	}
	if got := columns(t, url); slices.Contains(got, table+".probe_added") || slices.Contains(got, "zz_probe_rows.v") {
		t.Errorf("the failed apply left a part of the plan: %q", got)
	}
}

func TestApplySchemaConcurrently(t *testing.T) {
	url := pgtest.NewEmptyDatabase(t)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = ApplySchema(t.Context(), url, "", schemaapply.Options{})
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
	r, err := ApplySchema(t.Context(), url, "", schemaapply.Options{DryRun: true})
	if err != nil || len(r.Statements) > 0 {
		t.Errorf("the plan after both runs = %+v, %v; want none", r, err)
	}
	if got := tempSchemas(t, url); len(got) > 0 {
		t.Errorf("temporary schemas remain: %q", got)
	}
}
