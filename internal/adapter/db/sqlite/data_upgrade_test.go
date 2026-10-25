// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestDataUpgradeRepositoryContract(t *testing.T) {
	testkit.DataUpgradeRepositoryContract(t, func(t *testing.T) testkit.DataUpgradeHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		empty, err := Open(t.Context(), filepath.Join(t.TempDir(), "empty.db"), uow.Options{})
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = empty.Close() })
		return testkit.DataUpgradeHarness{
			UnitOfWork: db, Repository: db.DataUpgradeRepository(), Empty: empty.DataUpgradeRepository(),
			DropRecord: func(t *testing.T) {
				if _, err := db.db.ExecContext(t.Context(), "DROP TABLE data_upgrade_runs"); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}
