// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// Spike #3535: does a temporary table of the same name shadow the main table for a statement
// that the connection's cache prepared before the temporary table existed?
func TestSpikeTempTableShadowsCachedStatement(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "spike.db") + "?_pragma=journal_mode(WAL)"
	conn, err := newCachingConnector(dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(conn)
	db.SetMaxOpenConns(1)
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE tallies (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	count := func(q interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	}) int {
		var n int
		if err := q.QueryRowContext(ctx, "SELECT count(*) FROM tallies").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	insert := func(q interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	}, id string) {
		if _, err := q.ExecContext(ctx, "INSERT INTO tallies (id) VALUES (?)", id); err != nil {
			t.Fatal(err)
		}
	}
	insert(db, "a")
	live := count(db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE tallies AS SELECT * FROM main.tallies WHERE 0"); err != nil {
		t.Fatal(err)
	}
	insert(tx, "b")
	insert(tx, "c")
	shadowed := count(tx)
	var mainN int
	_ = tx.QueryRowContext(ctx, "SELECT count(*) FROM main.tallies").Scan(&mainN)
	_ = tx.Rollback()
	after := count(db)
	t.Logf("live=%d shadowed=%d main-during=%d after=%d", live, shadowed, mainN, after)
	if shadowed != 2 || mainN != 1 || after != 1 {
		t.Errorf("the temporary table did not shadow the cached statements")
	}
}
