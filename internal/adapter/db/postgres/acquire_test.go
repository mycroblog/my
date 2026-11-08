// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
	"github.com/mycroblog/my/schema"
)

// newBoundedDB opens a database with a pool of one connection and the bound on waiting for it.
func newBoundedDB(t *testing.T, wait time.Duration) *DB {
	t.Helper()
	u, err := url.Parse(pgtest.NewDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("pool_max_conns", "1")
	u.RawQuery = q.Encode()
	db, err := Open(t.Context(), u.String(), uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(db.Close)
	db.SetAcquireTimeout(wait)
	return db
}

// holdConnection keeps the only connection in a transaction until the test's returned function ends it.
func holdConnection(t *testing.T, db *DB) (release func()) {
	t.Helper()
	held, done, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- db.Tx(context.Background(), func(uow.Tx) error {
			close(held)
			<-done
			return nil
		})
	}()
	<-held
	return func() {
		close(done)
		if err := <-finished; err != nil {
			t.Errorf("the holding transaction: %v", err)
		}
	}
}

func TestAcquireTimeoutEndsTheWaitForAConnection(t *testing.T) {
	const wait = 100 * time.Millisecond
	db := newBoundedDB(t, wait)
	release := holdConnection(t, db)
	defer release()

	for name, attempt := range map[string]func(context.Context) error{
		"Exec": func(ctx context.Context) error {
			_, err := db.Exec(ctx, "SELECT 1")
			return err
		},
		"Query": func(ctx context.Context) error {
			rows, err := db.conns.Query(ctx, "SELECT 1")
			if err == nil {
				rows.Close()
			}
			return err
		},
		"QueryRow": func(ctx context.Context) error {
			_, err := db.DatabaseSize(ctx)
			return err
		},
		"Tx": func(ctx context.Context) error {
			return db.Tx(ctx, func(uow.Tx) error { return nil })
		},
		"Ready": func(ctx context.Context) error { return db.Ready(ctx, schema.Tables(schema.Postgres)) },
	} {
		t.Run(name, func(t *testing.T) {
			ctx, busy := kernel.WithDatabaseBusyNote(t.Context())
			start := time.Now()
			err := attempt(ctx)
			if !errors.Is(err, kernel.ErrDatabaseBusy) {
				t.Fatalf("err = %v, want ErrDatabaseBusy", err)
			}
			if !busy() {
				t.Error("the context was not noted as busy")
			}
			if d := time.Since(start); d < wait || d > 10*time.Second {
				t.Errorf("waited %s, want about %s", d, wait)
			}
		})
	}
}

// The caller's own deadline is not a busy database, even when it ends during the wait.
func TestAcquireTimeoutTellsTheCallersDeadlineApart(t *testing.T) {
	db := newBoundedDB(t, time.Minute)
	release := holdConnection(t, db)
	defer release()
	ctx, busy := kernel.WithDatabaseBusyNote(t.Context())
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err := db.Exec(ctx, "SELECT 1")
	if err == nil || errors.Is(err, kernel.ErrDatabaseBusy) {
		t.Fatalf("err = %v, want the deadline's", err)
	}
	if busy() {
		t.Error("the context was noted as busy")
	}
}

// What runs on the connection outlives the bound, which covers only the wait for it.
func TestAcquireTimeoutDoesNotBoundWhatRunsOnTheConnection(t *testing.T) {
	const wait = 50 * time.Millisecond
	sleep := "SELECT pg_sleep(0.3)"
	db := newBoundedDB(t, wait)
	ctx := t.Context()

	if _, err := db.Exec(ctx, sleep); err != nil {
		t.Errorf("Exec: %v", err)
	}
	rows, err := db.conns.Query(ctx, sleep)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	time.Sleep(2 * wait)
	for rows.Next() {
		continue
	}
	if err := rows.Err(); err != nil {
		t.Errorf("rows: %v", err)
	}
	var out string
	if err := db.conns.QueryRow(ctx, "SELECT pg_sleep(0.3)::text").Scan(&out); err != nil {
		t.Errorf("QueryRow: %v", err)
	}
	err = db.Tx(ctx, func(tx uow.Tx) error {
		pgTx, err := db.txOf(tx)
		if err != nil {
			return err
		}
		_, err = pgTx.Exec(ctx, sleep)
		return err
	})
	if err != nil {
		t.Errorf("Tx: %v", err)
	}
}

// Every way of using a connection gives it back, or the pool of one would be stuck for the next.
func TestAcquireGivesTheConnectionBack(t *testing.T) {
	db := newBoundedDB(t, 5*time.Second)
	ctx := t.Context()
	boom := errors.New("boom")

	rows, err := db.conns.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
	rows, err = db.conns.Query(ctx, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() { // ending the rows gives it back without Close
		continue
	}
	// pgx reports some errors on the rows; a failed query gives the connection back either way.
	if rows, err := db.conns.Query(ctx, "SELECT FROM no_such_table"); err == nil {
		rows.Close()
	}
	var n int
	if err := db.conns.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if err := db.conns.QueryRow(ctx, "SELECT FROM no_such_table").Scan(&n); err == nil {
		t.Fatal("no error from the row of a failed query")
	}
	if err := db.Tx(ctx, func(uow.Tx) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Tx: %v", err)
	}
	if err := db.Tx(ctx, func(uow.Tx) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := db.pool.Stat().AcquiredConns(); got != 0 {
		t.Errorf("%d connections still acquired", got)
	}
}
