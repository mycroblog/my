// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqlitenotification "github.com/mycroblog/my/internal/adapter/db/sqlite/notification"
	sqlitetimeline "github.com/mycroblog/my/internal/adapter/db/sqlite/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestAccountErasureContract(t *testing.T) {
	t.Parallel()
	federationtest.AccountErasureContract(t, func(t *testing.T, opts uow.Options) federationtest.AccountErasureHarness {
		db, _ := newTestDB(t, "", opts)
		return federationtest.AccountErasureHarness{
			RetentionHarness: federationtest.RetentionHarness{
				RemoteStatusHarness: federationtest.RemoteStatusHarness{
					AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
					Graph:          sqlitegraph.NewRepository(db),
					Statuses:       sqlitecontent.NewStatusRepository(db),
					Media:          sqlitecontent.NewMediaRepository(db),
					Timeline:       sqlitetimeline.NewRepository(db),
					Notifications:  sqlitenotification.NewRepository(db),
					Inbox:          NewRepository(db),
				},
				Interactions: sqlitecontent.NewInteractionRepository(db),
				Federation:   NewRepository(db),
				Retention:    sqlitecontent.NewStatusRepository(db),
				Purges:       NewRepository(db),
				Instances:    NewRepository(db),
				Profiles:     sqliteaccount.NewRepository(db),
			},
			AccountErasure:      sqliteaccount.NewRepository(db),
			IdentityErasure:     sqliteidentity.NewRepository(db),
			ContentErasure:      sqlitecontent.NewStatusRepository(db),
			GraphErasure:        sqlitegraph.NewRepository(db),
			TimelineErasure:     sqlitetimeline.NewRepository(db),
			NotificationErasure: sqlitenotification.NewRepository(db),
			Rows: func(t *testing.T, table, column, value string) int {
				t.Helper()
				var n int
				// The table and column names come from the contract, never from input.
				q := "SELECT count(*) FROM " + table + " WHERE " + column + " = ?"
				if err := db.SQLDB().QueryRowContext(t.Context(), q, value).Scan(&n); err != nil {
					t.Fatalf("count %s: %v", table, err)
				}
				return n
			},
		}
	})
}
