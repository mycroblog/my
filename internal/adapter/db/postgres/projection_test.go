// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/projectiontest"
	"github.com/mycroblog/my/schema"
)

func TestProjectionStoreContract(t *testing.T) {
	projectiontest.ProjectionStoreContract(t, func(t *testing.T) projectiontest.ProjectionStoreHarness {
		db := newTestDB(t, uow.Options{})
		return projectiontest.ProjectionStoreHarness{UnitOfWork: db, Store: db.ProjectionStore(schema.Tables(schema.Postgres))}
	})
}
