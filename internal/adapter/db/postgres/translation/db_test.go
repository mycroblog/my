// SPDX-License-Identifier: AGPL-3.0-only

package translation

import (
	"os"
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/pgtest"
)

func TestMain(m *testing.M) {
	os.Exit(pgtest.Main(m))
}

// newTestDB opens a new database with the product schema on the test server (package pgtest).
func newTestDB(t *testing.T, opts uow.Options) *postgres.DB {
	t.Helper()
	db, err := postgres.Open(t.Context(), pgtest.NewDatabase(t), opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}
