// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package db_test

import (
	"os"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
	"github.com/mycroblog/my/pkg/testkit/queuetest"
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

func postgresHarness(t *testing.T, clock *testkit.Clock, dsn string) queuetest.QueueHarness {
	t.Helper()
	d, err := postgres.Open(t.Context(), dsn, uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(d.Close)
	opts := queuedb.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)}
	q := queuedb.NewPostgres(d, opts)
	h := queuetest.QueueHarness{Queue: q, Source: q, DeadJobs: q, UnitOfWork: d, Outbox: q, Inspector: q, Attempts: q}
	h.Restart = func(t *testing.T) queuetest.QueueHarness {
		d.Close()
		return postgresHarness(t, clock, dsn)
	}
	return h
}

func TestPostgresQueueContract(t *testing.T) {
	queuetest.QueueContract(t, func(t *testing.T, clock *testkit.Clock) queuetest.QueueHarness {
		return postgresHarness(t, clock, newDatabase(t))
	})
}

func TestPostgresWakeContract(t *testing.T) {
	queuetest.QueueWakeContract(t, func(t *testing.T, clock *testkit.Clock, waker queue.Waker) queuetest.QueueWakeHarness {
		projectors := &uow.Projectors{}
		d, err := postgres.Open(t.Context(), newDatabase(t), uow.Options{Projectors: projectors})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(d.Close)
		q := queuedb.NewPostgres(d, queuedb.Options{
			Clock: clock, IDs: testkit.NewIDGenerator(clock), Waker: waker, Projectors: projectors, Logger: discardLogger(),
		})
		return queuetest.QueueWakeHarness{Queue: q, Source: q, DeadJobs: q, Finder: q, UnitOfWork: d, Outbox: q}
	})
}

func TestPostgresTypesContract(t *testing.T) {
	queuetest.QueueTypesContract(t, func(t *testing.T, clock *testkit.Clock) (queuetest.QueueHarness, queuetest.QueueTypesSource) {
		h := postgresHarness(t, clock, newDatabase(t))
		return h, h.Queue.(*queuedb.Queue).OnlyTypes
	})
}

func TestPostgresBacklogContract(t *testing.T) {
	queuetest.QueueBacklogContract(t, func(t *testing.T, clock *testkit.Clock) queuetest.QueueHarness {
		return postgresHarness(t, clock, newDatabase(t))
	})
}
