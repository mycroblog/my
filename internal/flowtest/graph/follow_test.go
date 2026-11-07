// SPDX-License-Identifier: AGPL-3.0-only

package graph_test

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestFollowFlows(t *testing.T) {
	t.Parallel()
	graphtest.FollowFlows(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db := flowtest.SQLite(t, opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          sqlitegraph.NewRepository(db),
		}
	})
}
