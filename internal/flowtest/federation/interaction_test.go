// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitefederation "github.com/mycroblog/my/internal/adapter/db/sqlite/federation"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqlitenotification "github.com/mycroblog/my/internal/adapter/db/sqlite/notification"
	sqlitetimeline "github.com/mycroblog/my/internal/adapter/db/sqlite/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestRemoteInteractionFlows(t *testing.T) {
	t.Parallel()
	federationtest.RemoteInteractionFlows(t, func(t *testing.T, opts uow.Options) federationtest.RemoteInteractionHarness {
		db := flowtest.SQLite(t, opts)
		feds := sqlitefederation.NewRepository(db)
		return federationtest.RemoteInteractionHarness{
			RemoteStatusHarness: federationtest.RemoteStatusHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
				Graph:          sqlitegraph.NewRepository(db),
				Statuses:       sqlitecontent.NewStatusRepository(db),
				Media:          sqlitecontent.NewMediaRepository(db),
				Timeline:       sqlitetimeline.NewRepository(db),
				Notifications:  sqlitenotification.NewRepository(db),
				Inbox:          feds,
				Quotes:         sqlitecontent.NewStatusRepository(db),
			},
			Interactions: sqlitecontent.NewInteractionRepository(db),
			Federation:   feds,
		}
	})
}
