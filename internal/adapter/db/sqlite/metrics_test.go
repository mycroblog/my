// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestDatabaseSizerContract(t *testing.T) {
	testkit.DatabaseSizerContract(t, func(t *testing.T) testkit.DatabaseSizerHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return testkit.DatabaseSizerHarness{Sizer: db, Grow: func(t *testing.T) {
			_, err := db.db.ExecContext(t.Context(), `CREATE TABLE size_probe AS
				WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 2048)
				SELECT randomblob(1000) AS v FROM n`)
			if err != nil {
				t.Fatal(err)
			}
		}}
	})
}
