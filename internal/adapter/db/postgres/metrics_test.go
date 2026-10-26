// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestDatabaseSizerContract(t *testing.T) {
	testkit.DatabaseSizerContract(t, func(t *testing.T) testkit.DatabaseSizerHarness {
		db := newTestDB(t, uow.Options{})
		return testkit.DatabaseSizerHarness{Sizer: db, Grow: func(t *testing.T) {
			// Rows of 1 KiB stay below the size that PostgreSQL compresses.
			_, err := db.pool.Exec(t.Context(), `CREATE TABLE size_probe AS
				SELECT repeat(md5(i::text), 32) AS v FROM generate_series(1, 2048) AS i`)
			if err != nil {
				t.Fatal(err)
			}
		}}
	})
}
