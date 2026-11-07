// SPDX-License-Identifier: AGPL-3.0-only

package dbupgrade

import (
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/dbupgradetest"
)

func TestDataUpgradeRepositoryContract(t *testing.T) {
	t.Parallel()
	dbupgradetest.DataUpgradeRepositoryContract(t, func(t *testing.T) dbupgradetest.DataUpgradeHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		empty, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "empty.db"), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = empty.Close() })
		return dbupgradetest.DataUpgradeHarness{
			UnitOfWork: db, Repository: NewDataUpgradeRepository(db), Empty: NewDataUpgradeRepository(empty),
			DropRecord: func(t *testing.T) {
				if _, err := db.SQLDB().ExecContext(t.Context(), "DROP TABLE data_upgrade_runs"); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}
