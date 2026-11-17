// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package db_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	kvdb "github.com/mycroblog/my/internal/adapter/kv/db"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
)

// TestSQLiteReadsDoNotWaitForTheWriteLock holds the write lock from another connection: Get and
// GetMany return the stored values with no wait, and a Set does not go through before its
// context ends.
func TestSQLiteReadsDoNotWaitForTheWriteLock(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "kv.db")
	d, err := sqlite.Open(t.Context(), path, uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	err = d.Tx(t.Context(), func(tx uow.Tx) error {
		sqlTx, err := d.SQLTx(tx)
		if err != nil {
			return err
		}
		_, err = sqlTx.ExecContext(t.Context(), infrastructure.Module.SQLite)
		return err
	})
	if err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	store := kvdb.NewSQLite(d, testkit.NewClock(time.Unix(1_700_000_000, 0)))
	if err := store.Set(t.Context(), "ns", "a", []byte("1"), 0); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(t.Context(), "ns", "b", []byte("2"), 0); err != nil {
		t.Fatal(err)
	}

	sqlitetest.HoldWriteLock(t, path)

	got, err := store.Get(t.Context(), "ns", "a")
	if err != nil || string(got) != "1" {
		t.Errorf("Get under the write lock = %q, %v, want 1", got, err)
	}
	many, err := store.GetMany(t.Context(), "ns", []string{"a", "b", "missing"})
	if err != nil || len(many) != 2 || string(many["a"]) != "1" || string(many["b"]) != "2" {
		t.Errorf("GetMany under the write lock = %q, %v, want a=1 and b=2", many, err)
	}
	if _, err := store.Get(t.Context(), "ns", "missing"); !errors.Is(err, kernel.ErrKeyNotFound) {
		t.Errorf("Get of a missing key under the write lock = %v, want ErrKeyNotFound", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := store.Set(ctx, "ns", "a", []byte("changed"), 0); err == nil {
		t.Error("Set under the write lock = nil, want an error when the context ends")
	}
}
