// SPDX-License-Identifier: AGPL-3.0-only

// Package flowtest holds the tests of the flows across modules (ADR-0153 §1): the use cases of
// several modules driven together, once, on the SQLite adapter, instead of once per dialect in the
// contracts of every database adapter. Each module's flows are in internal/flowtest/<module>, the
// most dependent module the flow goes through. The port contracts of pkg/testkit check that the
// SQL means the same on PostgreSQL.
//
// The packages may import the application and domain layers, the SQLite adapter, and pkg/testkit,
// never internal/interface or internal/platform (depguard, .golangci.yml).
package flowtest

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/sqlitetest"
)

// SQLite opens a new SQLite database with the product schema applied, a copy of sqlitetest's
// template, with the unit of work options opts, and closes it when t ends.
func SQLite(t *testing.T, opts uow.Options) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(t.Context(), sqlitetest.NewFile(t), opts)
	if err != nil {
		t.Fatalf("flowtest: open SQLite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
