// SPDX-License-Identifier: AGPL-3.0-only

//go:build !no_sqlite

package db_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

// TestSQLiteQueueReadsDoNotWaitForTheWriteLock holds the write lock from another connection: the
// queue's reads return what is stored with no wait.
func TestSQLiteQueueReadsDoNotWaitForTheWriteLock(t *testing.T) {
	t.Parallel()
	clock := testkit.NewClock(time.Unix(1_700_000_000, 0))
	path := filepath.Join(t.TempDir(), "queue.db")
	d := openSQLite(t, path, true)
	q := queuedb.NewSQLite(d, queuedb.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)})
	if err := q.Enqueue(t.Context(), queue.NewJob{Type: "test.job", Payload: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}

	sqlitetest.HoldWriteLock(t, path)

	backlog, err := q.Backlog(t.Context(), clock.Now())
	if err != nil || backlog.Ready["test.job"] != 1 {
		t.Errorf("Backlog under the write lock = %+v, %v, want one ready test.job", backlog, err)
	}
	due, err := q.DueJobs(t.Context(), clock.Now(), clock.Now(), 10)
	if err != nil || len(due) != 1 || due[0].Type != "test.job" {
		t.Errorf("DueJobs under the write lock = %+v, %v, want the test.job", due, err)
	}
}
