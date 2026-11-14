// SPDX-License-Identifier: AGPL-3.0-only

package instance

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/instancetest"
)

func TestInstanceRepositoryContract(t *testing.T) {
	t.Parallel()
	instancetest.InstanceRepositoryContract(t, func(t *testing.T, opts uow.Options) instancetest.InstanceHarness {
		db, _ := newTestDB(t, "", opts)
		return instancetest.InstanceHarness{UnitOfWork: db, Instance: NewRepository(db)}
	})
}
