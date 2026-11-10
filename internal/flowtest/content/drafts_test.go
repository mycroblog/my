// SPDX-License-Identifier: AGPL-3.0-only

package content_test

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqliteinstance "github.com/mycroblog/my/internal/adapter/db/sqlite/instance"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestDraftFlows(t *testing.T) {
	t.Parallel()
	contenttest.DraftFlows(t, func(t *testing.T, opts uow.Options) contenttest.DraftHarness {
		db := flowtest.SQLite(t, opts)
		return contenttest.DraftHarness{
			PostingHarness: contenttest.PostingHarness{
				AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
				Graph:          sqlitegraph.NewRepository(db),
				Instance:       sqliteinstance.NewRepository(db),
				Statuses:       sqlitecontent.NewStatusRepository(db),
				Media:          sqlitecontent.NewMediaRepository(db),
			},
			Drafts: sqlitecontent.NewStatusRepository(db),
		}
	})
}
