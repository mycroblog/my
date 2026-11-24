// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
)

// fakeConn is a driver connection that counts the statements it prepares and closes.
type fakeConn struct {
	driverConn // nil: only the methods below run
	prepared   map[string]int
	open       int
}

func (c *fakeConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	if c.prepared == nil {
		c.prepared = map[string]int{}
	}
	c.prepared[query]++
	c.open++
	return &fakeStmt{c: c}, nil
}

func (c *fakeConn) Close() error { return nil }

type fakeStmt struct{ c *fakeConn }

func (s *fakeStmt) Close() error                               { s.c.open--; return nil }
func (s *fakeStmt) NumInput() int                              { return -1 }
func (s *fakeStmt) Exec([]driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (s *fakeStmt) Query([]driver.Value) (driver.Rows, error)  { return fakeRows{}, nil }
func (s *fakeStmt) ExecContext(context.Context, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (s *fakeStmt) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	return fakeRows{}, nil
}

type fakeRows struct{}

func (fakeRows) Columns() []string         { return nil }
func (fakeRows) Close() error              { return nil }
func (fakeRows) Next([]driver.Value) error { return io.EOF }

func newFakeCachingConn(t *testing.T) (*cachingConn, *fakeConn) {
	t.Helper()
	fc := &fakeConn{}
	c, err := newCachingConn(fc)
	if err != nil {
		t.Fatal(err)
	}
	return c.(*cachingConn), fc
}

// TestStmtCachePreparesOnce runs the same SQL on a connection's prepared statement.
func TestStmtCachePreparesOnce(t *testing.T) {
	c, fc := newFakeCachingConn(t)
	ctx := t.Context()
	for range 3 {
		if _, err := c.ExecContext(ctx, "UPDATE t SET a = ?", nil); err != nil {
			t.Fatal(err)
		}
		rows, err := c.QueryContext(ctx, "SELECT a FROM t", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if fc.prepared["UPDATE t SET a = ?"] != 1 || fc.prepared["SELECT a FROM t"] != 1 {
		t.Errorf("prepared %v, want each statement once", fc.prepared)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if fc.open != 0 {
		t.Errorf("%d statements open after the connection closed, want none", fc.open)
	}
}

// TestStmtCacheRunsABusyStatementApart runs SQL whose statement has open rows on a statement of
// its own, which closes with its rows.
func TestStmtCacheRunsABusyStatementApart(t *testing.T) {
	c, fc := newFakeCachingConn(t)
	ctx := t.Context()
	outer, err := c.QueryContext(ctx, "SELECT a FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := c.QueryContext(ctx, "SELECT a FROM t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if fc.prepared["SELECT a FROM t"] != 2 || fc.open != 2 {
		t.Errorf("prepared %v with %d open, want a second statement while the first is busy", fc.prepared, fc.open)
	}
	_ = inner.Close()
	if fc.open != 1 {
		t.Errorf("%d statements open after the inner rows closed, want the cached one", fc.open)
	}
	_ = outer.Close()
	if _, err := c.QueryContext(ctx, "SELECT a FROM t", nil); err != nil {
		t.Fatal(err)
	}
	if fc.prepared["SELECT a FROM t"] != 2 {
		t.Errorf("prepared %v, want the cached statement reused once free", fc.prepared)
	}
}

// TestStmtCacheEvictsTheLeastRecentlyUsed keeps stmtCacheSize statements and closes the others.
func TestStmtCacheEvictsTheLeastRecentlyUsed(t *testing.T) {
	c, fc := newFakeCachingConn(t)
	ctx := t.Context()
	for i := range stmtCacheSize + 10 {
		if _, err := c.ExecContext(ctx, fmt.Sprintf("SELECT %d", i), nil); err != nil {
			t.Fatal(err)
		}
	}
	if fc.open != stmtCacheSize || c.lru.Len() != stmtCacheSize {
		t.Errorf("%d statements open, %d cached, want %d", fc.open, c.lru.Len(), stmtCacheSize)
	}
	if _, err := c.ExecContext(ctx, "SELECT 0", nil); err != nil {
		t.Fatal(err)
	}
	if fc.prepared["SELECT 0"] != 2 {
		t.Errorf("SELECT 0 prepared %d times, want again after its eviction", fc.prepared["SELECT 0"])
	}
}

// TestStmtCacheOnSQLite reads the same SQL inside the iteration of its own rows, in a transaction
// and outside, on the driver itself.
func TestStmtCacheOnSQLite(t *testing.T) {
	d, err := Open(t.Context(), filepath.Join(t.TempDir(), "cache.db"), uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := t.Context()
	if _, err := d.db.ExecContext(ctx, "CREATE TABLE t (a INTEGER NOT NULL); INSERT INTO t VALUES (1), (2), (3)"); err != nil {
		t.Fatal(err)
	}
	const q = "SELECT a FROM t WHERE a >= ? ORDER BY a"
	sum := 0
	err = d.Tx(ctx, func(tx uow.Tx) error {
		db, err := d.QueryTx(tx)
		if err != nil {
			return err
		}
		rows, err := db.QueryContext(ctx, q, 1)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var a int
			if err := rows.Scan(&a); err != nil {
				return err
			}
			var n int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ("+q+")", a).Scan(&n); err != nil {
				return err
			}
			inner, err := db.QueryContext(ctx, q, a)
			if err != nil {
				return err
			}
			for inner.Next() {
				sum++
			}
			if err := inner.Close(); err != nil {
				return err
			}
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum != 3+2+1 {
		t.Errorf("the nested reads saw %d rows, want 6", sum)
	}
	for range 2 {
		var n int
		if err := d.Reader().QueryRowContext(ctx, q[:len("SELECT a FROM t")]+" LIMIT 1").Scan(&n); err != nil || n != 1 {
			t.Errorf("a read on the pool = %d, %v", n, err)
		}
	}
}
