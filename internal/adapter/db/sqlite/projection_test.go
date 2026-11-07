// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestProjectionStoreContract(t *testing.T) {
	t.Parallel()
	testkit.ProjectionStoreContract(t, func(t *testing.T) testkit.ProjectionStoreHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return testkit.ProjectionStoreHarness{UnitOfWork: db, Store: db.ProjectionStore()}
	})
}
