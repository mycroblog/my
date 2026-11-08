// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/projectiontest"
	"github.com/mycroblog/my/schema"
)

func TestProjectionStoreContract(t *testing.T) {
	t.Parallel()
	projectiontest.ProjectionStoreContract(t, func(t *testing.T) projectiontest.ProjectionStoreHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return projectiontest.ProjectionStoreHarness{UnitOfWork: db, Store: db.ProjectionStore(schema.Tables(schema.SQLite))}
	})
}
