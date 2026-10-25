// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"net/url"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestDataUpgradeRepositoryContract(t *testing.T) {
	testkit.DataUpgradeRepositoryContract(t, func(t *testing.T) testkit.DataUpgradeHarness {
		dbURL := pgtest.NewDatabase(t)
		db, err := Open(t.Context(), dbURL, uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		// The empty database is a schema without tables in the same database.
		if _, err := db.pool.Exec(t.Context(), "CREATE SCHEMA empty_upgrade"); err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(dbURL)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", "empty_upgrade")
		u.RawQuery = q.Encode()
		empty, err := Open(t.Context(), u.String(), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(empty.Close)
		return testkit.DataUpgradeHarness{
			UnitOfWork: db, Repository: db.DataUpgradeRepository(), Empty: empty.DataUpgradeRepository(),
			DropRecord: func(t *testing.T) {
				if _, err := db.pool.Exec(t.Context(), "DROP TABLE data_upgrade_runs"); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}
