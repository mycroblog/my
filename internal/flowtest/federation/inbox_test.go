// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitefederation "github.com/mycroblog/my/internal/adapter/db/sqlite/federation"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
)

// inboxHarness is the inbox's harness on a new SQLite database with every repository that the
// inbox flows write through.
func inboxHarness(t *testing.T, opts uow.Options) federationtest.InboxHarness {
	db := flowtest.SQLite(t, opts)
	feds := sqlitefederation.NewRepository(db)
	return federationtest.InboxHarness{
		UnitOfWork: db, Inbox: feds, Federation: feds, Instances: feds, Accounts: sqliteaccount.NewRepository(db), Statuses: sqlitecontent.NewStatusRepository(db),
		Graph: sqlitegraph.NewRepository(db), Moderation: sqlitemoderation.NewRepository(db),
	}
}

func TestInboxFlows(t *testing.T) {
	t.Parallel()
	federationtest.InboxFlows(t, inboxHarness)
}

func TestInboxProcessingFlows(t *testing.T) {
	t.Parallel()
	federationtest.InboxProcessingFlows(t, inboxHarness)
}
