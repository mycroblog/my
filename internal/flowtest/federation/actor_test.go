// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitefederation "github.com/mycroblog/my/internal/adapter/db/sqlite/federation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
)

func TestRemoteActorFlows(t *testing.T) {
	t.Parallel()
	federationtest.RemoteActorFlows(t, func(t *testing.T, opts uow.Options) federationtest.RemoteActorHarness {
		db := flowtest.SQLite(t, opts)
		feds := sqlitefederation.NewRepository(db)
		return federationtest.RemoteActorHarness{UnitOfWork: db, Accounts: sqliteaccount.NewRepository(db), Federation: feds, Instances: feds}
	})
}
