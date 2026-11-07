// SPDX-License-Identifier: AGPL-3.0-only

package graph_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestFollowFlows(t *testing.T) {
	t.Parallel()
	testkit.FollowFlows(t, func(t *testing.T, opts uow.Options) testkit.FollowHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.FollowHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}
