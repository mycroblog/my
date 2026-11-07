// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"database/sql"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

func TestAccountErasureFlows(t *testing.T) {
	t.Parallel()
	testkit.AccountErasureFlows(t, func(t *testing.T, opts uow.Options) testkit.AccountErasureHarness {
		path := sqlitetest.NewFile(t)
		db, err := sqlite.Open(t.Context(), path, opts)
		if err != nil {
			t.Fatalf("open SQLite: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		// The rows are counted on a connection of their own to the same file.
		raw, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatalf("open SQLite: %v", err)
		}
		t.Cleanup(func() { _ = raw.Close() })
		return testkit.AccountErasureHarness{
			RetentionHarness:    retentionHarness(db),
			AccountErasure:      db.AccountRepository(),
			IdentityErasure:     db.IdentityRepository(),
			ContentErasure:      db.StatusRepository(),
			GraphErasure:        db.GraphRepository(),
			TimelineErasure:     db.TimelineRepository(),
			NotificationErasure: db.NotificationRepository(),
			Rows: func(t *testing.T, table, column, value string) int {
				t.Helper()
				var n int
				// The table and column names come from the flows, never from input.
				q := "SELECT count(*) FROM " + table + " WHERE " + column + " = ?"
				if err := raw.QueryRowContext(t.Context(), q, value).Scan(&n); err != nil {
					t.Fatalf("count %s: %v", table, err)
				}
				return n
			},
		}
	})
}
