// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/identitygen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
	"github.com/mycroblog/my/pkg/testkit/uowtest"
	"github.com/mycroblog/my/schema"
)

// newTestDB opens a new database file under t.TempDir() with the product schema applied, a copy of
// the template of package sqlitetest (see package schema for why executing the file matches an
// Atlas apply).
func newTestDB(t *testing.T, dsnSuffix string, opts uow.Options) (*DB, string) {
	t.Helper()
	path := sqlitetest.NewFile(t)
	db, err := Open(t.Context(), path+dsnSuffix, opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

// The UnitOfWork contract uses users as its probe table: a row per key, keyed by id.

func insertProbe(ctx context.Context, q *identitygen.Queries, key string) error {
	return q.InsertUser(ctx, identitygen.InsertUserParams{ID: key})
}

// probeQueries returns the queries of the probe table (users) bound to the transaction behind tx.
func probeQueries(db *DB, tx uow.Tx) (*identitygen.Queries, error) {
	q, err := db.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return identitygen.New(q), nil
}

func probeKeys(ctx context.Context, db *DB) ([]string, error) {
	rows, err := db.db.QueryContext(ctx, "SELECT id FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	keys := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func TestUnitOfWorkContract(t *testing.T) {
	t.Parallel()
	uowtest.UnitOfWorkContract(t, func(t *testing.T, opts uow.Options) uowtest.UnitOfWorkHarness {
		db, _ := newTestDB(t, "", opts)
		return uowtest.UnitOfWorkHarness{
			UnitOfWork: db,
			Insert: func(ctx context.Context, tx uow.Tx, name string) error {
				q, err := probeQueries(db, tx)
				if err != nil {
					return err
				}
				return insertProbe(ctx, q, name)
			},
			Keys: func(ctx context.Context) ([]string, error) { return probeKeys(ctx, db) },
		}
	})
}

func TestOpenSetsRequiredParameters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		dsn  string
		want []string
		not  []string
	}{
		{
			dsn:  "my.db",
			want: []string{"file:my.db?", "_txlock=immediate", "busy_timeout%285000%29", "journal_mode%28wal%29", "foreign_keys%281%29", "synchronous%28normal%29"},
		},
		{
			dsn:  "file:/var/lib/my/my.db?_txlock=deferred&_pragma=busy_timeout(100)",
			want: []string{"file:/var/lib/my/my.db?", "_txlock=immediate", "busy_timeout%28100%29"},
			not:  []string{"deferred", "busy_timeout%285000%29"},
		},
	} {
		got, err := normalizeDSN(tc.dsn)
		if err != nil {
			t.Fatalf("normalizeDSN(%q) = %v", tc.dsn, err)
		}
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("normalizeDSN(%q) = %q, missing %q", tc.dsn, got, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("normalizeDSN(%q) = %q, must not contain %q", tc.dsn, got, n)
			}
		}
	}
	for _, dsn := range []string{"", "file:", ":memory:", "file:x?mode=memory"} {
		if _, err := normalizeDSN(dsn); err == nil {
			t.Errorf("normalizeDSN(%q) succeeded, want an error", dsn)
		}
	}
}

func TestConnectionsUseWAL(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	var mode string
	if err := db.db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

// TestRetriesWhenTheDatabaseIsLocked holds the write lock from a second connection so that the
// UnitOfWork's BEGIN IMMEDIATE times out with SQLITE_BUSY, and releases it on the first retry.
func TestRetriesWhenTheDatabaseIsLocked(t *testing.T) {
	t.Parallel()
	var (
		retries []error
		blocker *sql.Tx
	)
	opts := uow.Options{Retry: uow.RetryPolicy{OnRetry: func(_ int, err error) {
		retries = append(retries, err)
		if blocker != nil {
			if err := blocker.Commit(); err != nil {
				t.Errorf("release the lock: %v", err)
			}
			blocker = nil
		}
	}}}
	db, path := newTestDB(t, "?_pragma=busy_timeout(10)", opts)

	other, err := Open(t.Context(), path, uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	if blocker, err = other.db.BeginTx(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err := insertProbe(t.Context(), identitygen.New(blocker), testkit.NamedID("blocker")); err != nil {
		t.Fatal(err)
	}

	err = db.Tx(t.Context(), func(tx uow.Tx) error {
		q, err := probeQueries(db, tx)
		if err != nil {
			return err
		}
		return insertProbe(t.Context(), q, testkit.NamedID("a"))
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
	if len(retries) != 1 {
		t.Fatalf("retried %d times, want 1", len(retries))
	}
	var se *msqlite.Error
	if !errors.Is(retries[0], uow.ErrRetryable) || !errors.As(retries[0], &se) || se.Code()&0xff != sqlite3.SQLITE_BUSY {
		t.Errorf("retried error = %v, want a retryable SQLITE_BUSY", retries[0])
	}
	got, err := probeKeys(t.Context(), db)
	want := []string{testkit.NamedID("a"), testkit.NamedID("blocker")}
	slices.Sort(want)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("committed keys = %q (%v), want %q", got, err, want)
	}
}

func TestRejectsForeignTransactions(t *testing.T) {
	t.Parallel()
	if _, err := (&DB{}).txOf(foreignTx{}); !errors.Is(err, ErrForeignTx) {
		t.Fatal("txOf accepted a transaction of another adapter")
	}
}

type foreignTx struct{}

func (foreignTx) Emit(context.Context, uow.Event) error { return nil }

func TestReady(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	if err := db.Ready(t.Context(), schema.Tables(schema.SQLite)); err != nil {
		t.Fatalf("Ready on an applied schema = %v", err)
	}
	if _, err := db.db.ExecContext(t.Context(), "DROP TABLE dead_jobs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Ready(t.Context(), schema.Tables(schema.SQLite)); err == nil || !strings.Contains(err.Error(), "dead_jobs") {
		t.Errorf("Ready with a missing table = %v, want an error naming dead_jobs", err)
	}

	empty, err := Open(t.Context(), filepath.Join(t.TempDir(), "empty.db"), uow.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := empty.Ready(t.Context(), schema.Tables(schema.SQLite)); err == nil {
		t.Error("Ready on an empty database = nil, want an error")
	}
	_ = empty.Close()
	if err := empty.Ready(t.Context(), schema.Tables(schema.SQLite)); err == nil {
		t.Error("Ready on a closed database = nil, want an error")
	}
}
