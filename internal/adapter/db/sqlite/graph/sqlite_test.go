// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestFollowRepositoryContract(t *testing.T) {
	t.Parallel()
	graphtest.FollowRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db, _ := newTestDB(t, "", opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          NewRepository(db),
		}
	})
}

func TestBlockRepositoryContract(t *testing.T) {
	t.Parallel()
	graphtest.BlockRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db, _ := newTestDB(t, "", opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          NewRepository(db),
		}
	})
}
