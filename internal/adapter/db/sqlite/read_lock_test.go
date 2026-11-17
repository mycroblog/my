// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

func commitProbe(t *testing.T, db *DB, key string) {
	t.Helper()
	err := db.Tx(t.Context(), func(tx uow.Tx) error {
		q, err := probeQueries(db, tx)
		if err != nil {
			return err
		}
		return insertProbe(t.Context(), q, key)
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
}

// TestReadDoesNotWaitForTheWriteLock holds the write lock from another connection: a Read returns
// the committed row with no wait, and a Tx does not begin before its context ends.
func TestReadDoesNotWaitForTheWriteLock(t *testing.T) {
	t.Parallel()
	db, path := newTestDB(t, "", uow.Options{})
	a := testkit.NamedID("a")
	commitProbe(t, db, a)

	sqlitetest.HoldWriteLock(t, path)

	var seen []string
	err := db.Read(t.Context(), func(tx uow.Tx) error {
		sqlTx, err := db.SQLTx(tx)
		if err != nil {
			return err
		}
		seen, err = selectKeys(t.Context(), sqlTx)
		return err
	})
	if err != nil {
		t.Fatalf("Read under the write lock = %v", err)
	}
	if !slices.Equal(seen, []string{a}) {
		t.Errorf("Read saw %q, want %q", seen, []string{a})
	}

	// The Tx side: it has not begun when the context ends.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	began := false
	err = db.Tx(ctx, func(uow.Tx) error { began = true; return nil })
	if began {
		t.Error("a Tx began while another connection held the write lock")
	}
	if err == nil {
		t.Error("Tx under the write lock = nil, want an error")
	}
}

// TestReadRefusesWritesWithTheDatabasesError holds the contract of Read for SQLite: a write
// through the transaction fails with SQLITE_READONLY and is not kept.
func TestReadRefusesWritesWithTheDatabasesError(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	var wrote error
	err := db.Read(t.Context(), func(tx uow.Tx) error {
		q, err := probeQueries(db, tx)
		if err != nil {
			return err
		}
		wrote = insertProbe(t.Context(), q, testkit.NamedID("a"))
		return nil
	})
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	var se *msqlite.Error
	if !errors.As(wrote, &se) || se.Code()&0xff != sqlite3.SQLITE_READONLY {
		t.Errorf("write in Read = %v, want SQLITE_READONLY", wrote)
	}
	if got, err := probeKeys(t.Context(), db); err != nil || len(got) != 0 {
		t.Errorf("committed keys = %q (%v), want none", got, err)
	}
}

// TestReadLeavesTheConnectionAbleToWrite runs Reads of every ending on a pool of one connection,
// so that every Tx after them gets the connection a Read used.
func TestReadLeavesTheConnectionAbleToWrite(t *testing.T) {
	t.Parallel()
	db, _ := newTestDB(t, "", uow.Options{})
	db.SQLDB().SetMaxOpenConns(1)

	reads := []func(uow.Tx) error{
		func(uow.Tx) error { return nil },
		func(uow.Tx) error { return errors.New("boom") },
		func(tx uow.Tx) error {
			q, err := probeQueries(db, tx)
			if err != nil {
				return err
			}
			return insertProbe(t.Context(), q, testkit.NamedID("refused"))
		},
	}
	var want []string
	for i := range 20 {
		_ = db.Read(t.Context(), reads[i%len(reads)])
		key := testkit.NamedID(fmt.Sprintf("w%d", i))
		commitProbe(t, db, key)
		want = append(want, key)
	}
	slices.Sort(want)
	got, err := probeKeys(t.Context(), db)
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("committed keys = %q (%v), want %q", got, err, want)
	}
}
