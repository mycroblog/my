// SPDX-License-Identifier: AGPL-3.0-only

package search

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

// newTestDB opens a new database file under t.TempDir() with the product schema applied, a copy of
// the template of package sqlitetest.
func newTestDB(t *testing.T, dsnSuffix string, opts uow.Options) (*sqlite.DB, string) {
	t.Helper()
	path := sqlitetest.NewFile(t)
	db, err := sqlite.Open(t.Context(), path+dsnSuffix, opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}
