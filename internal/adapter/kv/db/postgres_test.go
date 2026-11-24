// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package db_test

import (
	"os"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/kerneltest"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

// TestMain stops the PostgreSQL server of package pgtest after the tests.
func TestMain(m *testing.M) {
	os.Exit(pgtest.Main(m))
}

// newDatabase returns the URL of a new database with the product schema on the test server
// (package pgtest).
func newDatabase(t *testing.T) string {
	t.Helper()
	return pgtest.NewDatabase(t)
}

func TestPostgresKeyValueStoreContract(t *testing.T) {
	kerneltest.KeyValueStoreContract(t, func(t *testing.T, clock *testkit.Clock) kernel.KeyValueStore {
		d, err := postgres.Open(t.Context(), newDatabase(t), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(d.Close)
		return kvdb.NewPostgres(d, clock)
	})
}

// TestPostgresWritesOutsideTransactions checks that each write of the store on PostgreSQL is one
// statement on the pool, with no transaction around it: two round trips fewer than BEGIN and
// COMMIT would make, on every request that takes from a rate limit.
func TestPostgresWritesOutsideTransactions(t *testing.T) {
	d, err := postgres.Open(t.Context(), newDatabase(t), uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(d.Close)
	tr := testkit.NewTracer()
	d.SetTracer(tr)
	store := kvdb.NewPostgres(d, testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	ctx, parent := tr.Start(t.Context(), kernel.SpanSpec{Name: "GET /api/v1/preferences"}, kernel.Attrs{})
	if _, err := store.IncrBy(ctx, "rate", "k", 1, time.Minute); err != nil {
		t.Fatalf("IncrBy: %v", err)
	}
	if err := store.Set(ctx, "rate", "s", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if ok, err := store.CompareAndSwap(ctx, "rate", "s", []byte("v"), []byte("w"), time.Minute); err != nil || !ok {
		t.Fatalf("CompareAndSwap = %v, %v", ok, err)
	}
	if err := store.Delete(ctx, "rate", "s"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	parent.End("")
	if txs := tr.Named("my.db.transaction"); len(txs) != 0 {
		t.Errorf("the writes ran %d transactions, want none", len(txs))
	}
	for _, name := range []string{"IncrEntry", "SetEntry", "SwapEntry", "DeleteEntry"} {
		if qs := tr.Named(name); len(qs) != 1 {
			t.Errorf("%s ran %d times, want once", name, len(qs))
		}
	}
}
