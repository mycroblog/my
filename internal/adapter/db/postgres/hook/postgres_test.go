// SPDX-License-Identifier: AGPL-3.0-only

package hook

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/hooktest"
)

func TestHookRepositoryContract(t *testing.T) {
	hooktest.HookRepositoryContract(t, func(t *testing.T) hooktest.HookHarness {
		db := newTestDB(t, uow.Options{})
		return hooktest.HookHarness{UnitOfWork: db, Repository: NewRepository(db)}
	})
}
