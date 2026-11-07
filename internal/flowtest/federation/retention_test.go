// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
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

// retentionHarness is the harness of the retention flows on db.
func retentionHarness(db *sqlite.DB) federationtest.RetentionHarness {
	return federationtest.RetentionHarness{
		RemoteStatusHarness: federationtest.RemoteStatusHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          sqlitegraph.NewRepository(db),
			Statuses:       sqlitecontent.NewStatusRepository(db),
			Media:          sqlitecontent.NewMediaRepository(db),
			Timeline:       sqlitetimeline.NewRepository(db),
			Notifications:  sqlitenotification.NewRepository(db),
			Inbox:          sqlitefederation.NewRepository(db),
		},
		Interactions: sqlitecontent.NewInteractionRepository(db),
		Federation:   sqlitefederation.NewRepository(db),
		Retention:    sqlitecontent.NewStatusRepository(db),
		Purges:       sqlitefederation.NewRepository(db),
		Instances:    sqlitefederation.NewRepository(db),
		Profiles:     sqliteaccount.NewRepository(db),
		Emojis:       sqlitecontent.NewCustomEmojiRepository(db),
	}
}

func TestRetentionFlows(t *testing.T) {
	t.Parallel()
	federationtest.RetentionFlows(t, func(t *testing.T, opts uow.Options) federationtest.RetentionHarness {
		return retentionHarness(flowtest.SQLite(t, opts))
	})
}
