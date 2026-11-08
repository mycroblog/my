// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"testing"

	postgresaccount "github.com/mycroblog/my/internal/adapter/db/postgres/account"
	postgresidentity "github.com/mycroblog/my/internal/adapter/db/postgres/identity"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestFollowRepositoryContract(t *testing.T) {
	graphtest.FollowRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db := newTestDB(t, opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
			Graph:          NewRepository(db),
		}
	})
}

func TestBlockRepositoryContract(t *testing.T) {
	graphtest.BlockRepositoryContract(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db := newTestDB(t, opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
			Graph:          NewRepository(db),
		}
	})
}
