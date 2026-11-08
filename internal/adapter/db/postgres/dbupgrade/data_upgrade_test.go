// SPDX-License-Identifier: AGPL-3.0-only

package dbupgrade

import (
	"net/url"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/dbupgradetest"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestDataUpgradeRepositoryContract(t *testing.T) {
	dbupgradetest.DataUpgradeRepositoryContract(t, func(t *testing.T) dbupgradetest.DataUpgradeHarness {
		dbURL := pgtest.NewDatabase(t)
		db, err := postgres.Open(t.Context(), dbURL, uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(db.Close)
		// The empty database is a schema without tables in the same database.
		if _, err := db.Pool().Exec(t.Context(), "CREATE SCHEMA empty_upgrade"); err != nil {
			t.Fatal(err)
		}
		u, err := url.Parse(dbURL)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", "empty_upgrade")
		u.RawQuery = q.Encode()
		empty, err := postgres.Open(t.Context(), u.String(), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(empty.Close)
		return dbupgradetest.DataUpgradeHarness{
			UnitOfWork: db, Repository: NewDataUpgradeRepository(db), Empty: NewDataUpgradeRepository(empty),
			DropRecord: func(t *testing.T) {
				if _, err := db.Pool().Exec(t.Context(), "DROP TABLE data_upgrade_runs"); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}
