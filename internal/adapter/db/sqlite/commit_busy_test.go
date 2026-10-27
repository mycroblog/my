// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

// TestRetriesACommitThatFailsWithBusy forces SQLITE_BUSY at COMMIT and checks that the failed
// transaction leaves nothing behind on its connection and that the retry runs the whole
// transaction function again, not just COMMIT (see attempt).
//
// WAL never makes COMMIT wait: BEGIN IMMEDIATE already holds the write lock. The test uses the
// rollback journal instead, where COMMIT needs the EXCLUSIVE lock and so waits for a reader that
// holds a SHARED lock, and the pool has one connection, so the retry runs on the connection the
// failed commit used.
func TestRetriesACommitThatFailsWithBusy(t *testing.T) {
	ctx := t.Context()
	var (
		retries []error
		reader  *sql.Tx
		db      *DB
	)
	opts := uow.Options{Retry: uow.RetryPolicy{OnRetry: func(_ int, err error) {
		retries = append(retries, err)
		// The reader still holds its lock, so nothing has committed; the connection must be
		// out of the failed transaction: its insert is gone and a new transaction can begin.
		if got, err := probeKeys(ctx, db); err != nil || len(got) != 0 {
			t.Errorf("keys after the failed commit = %q (%v), want none", got, err)
		}
		conn, err := db.db.Conn(ctx)
		if err != nil {
			t.Fatalf("take the connection: %v", err)
		}
		if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
			t.Errorf("BEGIN after the failed commit: %v", err)
		} else if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
			t.Errorf("ROLLBACK: %v", err)
		}
		_ = conn.Close()
		if reader != nil {
			if err := reader.Commit(); err != nil {
				t.Errorf("release the reader: %v", err)
			}
			reader = nil
		}
	}}}
	db, path := newTestDB(t, "?_pragma=busy_timeout(10)&_pragma=journal_mode(delete)&_pragma=synchronous(off)", opts)
	db.db.SetMaxOpenConns(1)

	// sql.Open, not this package's Open, which would switch the file to WAL.
	other, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(10)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	var mode string
	if err := other.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "delete" {
		t.Fatalf("journal_mode = %q (%v), want delete", mode, err)
	}
	if reader, err = other.BeginTx(ctx, nil); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := reader.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("take the SHARED lock: %v", err)
	}

	attempts := 0
	err = db.Tx(ctx, func(tx uow.Tx) error {
		attempts++
		return insertProbe(ctx, sqlitegen.New(tx.(*Tx).tx), testkit.NamedID("a"))
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
	if attempts != 2 {
		t.Errorf("ran the transaction function %d times, want 2", attempts)
	}
	if len(retries) != 1 {
		t.Fatalf("retried %d times, want 1", len(retries))
	}
	var se *msqlite.Error
	if !errors.Is(retries[0], uow.ErrRetryable) || !errors.As(retries[0], &se) || se.Code()&0xff != sqlite3.SQLITE_BUSY ||
		!strings.Contains(retries[0].Error(), "sqlite: commit") {
		t.Errorf("retried error = %v, want a retryable SQLITE_BUSY from the commit", retries[0])
	}
	got, err := probeKeys(ctx, db)
	if want := []string{testkit.NamedID("a")}; err != nil || !slices.Equal(got, want) {
		t.Errorf("committed keys = %q (%v), want %q", got, err, want)
	}
}
