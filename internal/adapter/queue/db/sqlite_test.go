// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package db_test

import (
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/queuetest"
	"github.com/mycroblog/my/schema"
)

// openSQLite opens the database file at path, applying the product schema when apply is set
// (see package schema for why executing the file matches an Atlas apply).
func openSQLite(t *testing.T, path string, apply bool) *sqlite.DB {
	t.Helper()
	return openSQLiteWith(t, path, apply, uow.Options{})
}

func openSQLiteWith(t *testing.T, path string, apply bool, opts uow.Options) *sqlite.DB {
	t.Helper()
	d, err := sqlite.Open(t.Context(), path, opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if apply {
		err := d.Tx(t.Context(), func(tx uow.Tx) error {
			sqlTx, err := d.SQLTx(tx)
			if err != nil {
				return err
			}
			_, err = sqlTx.ExecContext(t.Context(), schema.SQLite)
			return err
		})
		if err != nil {
			t.Fatalf("apply schema: %v", err)
		}
	}
	return d
}

func sqliteHarness(t *testing.T, clock *testkit.Clock, path string, apply bool) queuetest.QueueHarness {
	t.Helper()
	d := openSQLite(t, path, apply)
	opts := queuedb.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)}
	q := queuedb.NewSQLite(d, opts)
	h := queuetest.QueueHarness{Queue: q, Source: q, DeadJobs: q, UnitOfWork: d, Outbox: q, Inspector: q, Attempts: q}
	h.Restart = func(t *testing.T) queuetest.QueueHarness {
		_ = d.Close()
		return sqliteHarness(t, clock, path, false)
	}
	return h
}

func TestSQLiteBacklogContract(t *testing.T) {
	queuetest.QueueBacklogContract(t, func(t *testing.T, clock *testkit.Clock) queuetest.QueueHarness {
		return sqliteHarness(t, clock, filepath.Join(t.TempDir(), "queue.db"), true)
	})
}

func TestSQLiteTypesContract(t *testing.T) {
	queuetest.QueueTypesContract(t, func(t *testing.T, clock *testkit.Clock) (queuetest.QueueHarness, queuetest.QueueTypesSource) {
		h := sqliteHarness(t, clock, filepath.Join(t.TempDir(), "queue.db"), true)
		return h, h.Queue.(*queuedb.Queue).OnlyTypes
	})
}

func TestSQLiteWakeContract(t *testing.T) {
	queuetest.QueueWakeContract(t, func(t *testing.T, clock *testkit.Clock, waker queue.Waker) queuetest.QueueWakeHarness {
		projectors := &uow.Projectors{}
		d := openSQLiteWith(t, filepath.Join(t.TempDir(), "queue.db"), true, uow.Options{Projectors: projectors})
		q := queuedb.NewSQLite(d, queuedb.Options{
			Clock: clock, IDs: testkit.NewIDGenerator(clock), Waker: waker, Projectors: projectors, Logger: discardLogger(),
		})
		return queuetest.QueueWakeHarness{Queue: q, Source: q, DeadJobs: q, Finder: q, UnitOfWork: d, Outbox: q}
	})
}

func TestSQLiteQueueContract(t *testing.T) {
	queuetest.QueueContract(t, func(t *testing.T, clock *testkit.Clock) queuetest.QueueHarness {
		return sqliteHarness(t, clock, filepath.Join(t.TempDir(), "queue.db"), true)
	})
}
