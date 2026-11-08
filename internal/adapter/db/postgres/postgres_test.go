// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mycroblog/my/internal/adapter/db/postgres/identitygen"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
	"github.com/mycroblog/my/pkg/testkit/uowtest"
	"github.com/mycroblog/my/schema"
)

func TestMain(m *testing.M) {
	os.Exit(pgtest.Main(m))
}

// newTestDB opens a new database with the product schema on the test server (package pgtest).
func newTestDB(t *testing.T, opts uow.Options) *DB {
	t.Helper()
	db, err := Open(t.Context(), pgtest.NewDatabase(t), opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// The UnitOfWork contract uses users as its probe table: a row per key, keyed by id.

func insertProbe(ctx context.Context, q *identitygen.Queries, key string) error {
	return q.InsertUser(ctx, identitygen.InsertUserParams{ID: key, CreatedAt: time.UnixMilli(0)})
}

// probeQueries returns the queries of the probe table (users) bound to the transaction behind tx.
func probeQueries(db *DB, tx uow.Tx) (*identitygen.Queries, error) {
	q, err := db.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return identitygen.New(q), nil
}

func probeKeys(ctx context.Context, db *DB) ([]string, error) {
	rows, err := db.pool.Query(ctx, "SELECT id FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if keys == nil {
		keys = []string{}
	}
	return keys, err
}

func TestUnitOfWorkContract(t *testing.T) {
	uowtest.UnitOfWorkContract(t, func(t *testing.T, opts uow.Options) uowtest.UnitOfWorkHarness {
		db := newTestDB(t, opts)
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

func TestTransactionsAreReadCommitted(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	err := db.Tx(t.Context(), func(tx uow.Tx) error {
		pgTx, err := db.txOf(tx)
		if err != nil {
			return err
		}
		var level string
		if err := pgTx.QueryRow(t.Context(), "SHOW transaction_isolation").Scan(&level); err != nil {
			return err
		}
		if level != "read committed" {
			t.Errorf("transaction_isolation = %q, want read committed", level)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
}

// TestRetriesDeadlocks makes two transactions lock two rows in opposite orders. PostgreSQL
// aborts one of them with deadlock_detected; the UnitOfWork must retry it to success.
func TestRetriesDeadlocks(t *testing.T) {
	var (
		mu      sync.Mutex
		retries []error
	)
	db := newTestDB(t, uow.Options{Retry: uow.RetryPolicy{OnRetry: func(_ int, err error) {
		mu.Lock()
		defer mu.Unlock()
		retries = append(retries, err)
	}}})
	setup := identitygen.New(db.pool)
	x, y := testkit.NamedID("x"), testkit.NamedID("y")
	for _, name := range []string{x, y} {
		if err := insertProbe(t.Context(), setup, name); err != nil {
			t.Fatal(err)
		}
	}

	var bothLocked sync.WaitGroup
	bothLocked.Add(2)
	lock := func(tx uow.Tx, key string) error {
		pgTx, err := db.txOf(tx)
		if err != nil {
			return err
		}
		_, err = pgTx.Exec(t.Context(), "SELECT 1 FROM users WHERE id = $1 FOR UPDATE", key)
		return err
	}
	lockBoth := func(first, second string) error {
		attempt := 0
		return db.Tx(t.Context(), func(tx uow.Tx) error {
			attempt++
			if err := lock(tx, first); err != nil {
				return err
			}
			if attempt == 1 {
				bothLocked.Done()
				bothLocked.Wait()
			}
			if err := lock(tx, second); err != nil {
				return err
			}
			q, err := probeQueries(db, tx)
			if err != nil {
				return err
			}
			return insertProbe(t.Context(), q, testkit.NamedID(first+"-done"))
		})
	}
	errs := make(chan error, 2)
	go func() { errs <- lockBoth(x, y) }()
	go func() { errs <- lockBoth(y, x) }()
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("Tx = %v", err)
		}
	}

	if len(retries) != 1 {
		t.Fatalf("retried %d times, want 1", len(retries))
	}
	var pgErr *pgconn.PgError
	if !errors.Is(retries[0], uow.ErrRetryable) || !errors.As(retries[0], &pgErr) || pgErr.Code != "40P01" {
		t.Errorf("retried error = %v, want a retryable deadlock_detected (40P01)", retries[0])
	}
	got, err := probeKeys(t.Context(), db)
	want := []string{x, y, testkit.NamedID(x + "-done"), testkit.NamedID(y + "-done")}
	slices.Sort(want)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("committed keys = %q (%v), want both transactions committed", got, err)
	}
}

func TestIsRetryable(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{&pgconn.PgError{Code: "40001"}, true},
		{&pgconn.PgError{Code: "40P01"}, true},
		{fmt.Errorf("postgres: commit: %w", &pgconn.PgError{Code: "40001"}), true},
		{&pgconn.PgError{Code: "23505"}, false},
		{errors.New("40001"), false},
		{nil, false},
	} {
		if got := isRetryable(tc.err); got != tc.want {
			t.Errorf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestOpenHidesTheURLInErrors(t *testing.T) {
	_, err := Open(t.Context(), "postgres://user:s3cret@host:notaport/db", uow.Options{})
	if err == nil {
		t.Fatal("Open succeeded with an invalid URL")
	}
	if got := err.Error(); strings.Contains(got, "s3cret") {
		t.Errorf("error %q leaks the password", got)
	}
}

func TestRejectsForeignTransactions(t *testing.T) {
	if _, err := (&DB{}).txOf(foreignTx{}); !errors.Is(err, ErrForeignTx) {
		t.Fatal("txOf accepted a transaction of another adapter")
	}
}

type foreignTx struct{}

func (foreignTx) Emit(context.Context, uow.Event) error { return nil }

func TestReady(t *testing.T) {
	db := newTestDB(t, uow.Options{})
	if err := db.Ready(t.Context(), schema.Tables(schema.Postgres)); err != nil {
		t.Fatalf("Ready on an applied schema = %v", err)
	}
	if _, err := db.pool.Exec(t.Context(), "DROP TABLE dead_jobs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Ready(t.Context(), schema.Tables(schema.Postgres)); err == nil || !strings.Contains(err.Error(), "dead_jobs") {
		t.Errorf("Ready with a missing table = %v, want an error naming dead_jobs", err)
	}
	db.Close()
	if err := db.Ready(t.Context(), schema.Tables(schema.Postgres)); err == nil {
		t.Error("Ready on a closed pool = nil, want an error")
	}
}
