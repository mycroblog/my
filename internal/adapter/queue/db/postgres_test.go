// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_postgres

package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
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

func postgresHarness(t *testing.T, clock *testkit.Clock, dsn string, relay bool) testkit.QueueHarness {
	t.Helper()
	d, err := postgres.Open(t.Context(), dsn, uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(d.Close)
	opts := queuedb.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)}
	q := queuedb.NewPostgres(d, opts)
	h := testkit.QueueHarness{Queue: q, Source: q, DeadJobs: q, UnitOfWork: d, Outbox: q, Inspector: q, Attempts: q}
	if relay {
		o := queuedb.NewPostgresOutbox(d, opts)
		h.Outbox = o
		h.Relay = func(ctx context.Context) error {
			_, err := o.Relay(ctx, q, 0)
			return err
		}
		return h
	}
	h.Restart = func(t *testing.T) testkit.QueueHarness {
		d.Close()
		return postgresHarness(t, clock, dsn, false)
	}
	return h
}

func TestPostgresQueueContract(t *testing.T) {
	testkit.QueueContract(t, func(t *testing.T, clock *testkit.Clock) testkit.QueueHarness {
		return postgresHarness(t, clock, newDatabase(t), false)
	})
}

// TestPostgresOutboxContract runs the contract with jobs enqueued in transactions through
// outbox_events and relayed into the queue.
func TestPostgresOutboxContract(t *testing.T) {
	testkit.QueueContract(t, func(t *testing.T, clock *testkit.Clock) testkit.QueueHarness {
		return postgresHarness(t, clock, newDatabase(t), true)
	})
}

func TestPostgresOverlappingRelays(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	d, err := postgres.Open(t.Context(), newDatabase(t), uow.Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(d.Close)
	opts := queuedb.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)}
	testOverlappingRelays(t, d, queuedb.NewPostgresOutbox(d, opts), queuedb.NewPostgresOutbox(d, opts))
}

func TestPostgresWakeContract(t *testing.T) {
	testkit.QueueWakeContract(t, func(t *testing.T, clock *testkit.Clock, waker queue.Waker) testkit.QueueWakeHarness {
		projectors := &uow.Projectors{}
		d, err := postgres.Open(t.Context(), newDatabase(t), uow.Options{Projectors: projectors})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(d.Close)
		q := queuedb.NewPostgres(d, queuedb.Options{
			Clock: clock, IDs: testkit.NewIDGenerator(clock), Waker: waker, Projectors: projectors, Logger: discardLogger(),
		})
		return testkit.QueueWakeHarness{Queue: q, Source: q, DeadJobs: q, Finder: q, UnitOfWork: d, Outbox: q}
	})
}

func TestPostgresBacklogContract(t *testing.T) {
	testkit.QueueBacklogContract(t, func(t *testing.T, clock *testkit.Clock) testkit.QueueHarness {
		return postgresHarness(t, clock, newDatabase(t), false)
	})
}
