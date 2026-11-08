// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"testing"

	postgresaccount "github.com/mycroblog/my/internal/adapter/db/postgres/account"
	postgrescontent "github.com/mycroblog/my/internal/adapter/db/postgres/content"
	postgresgraph "github.com/mycroblog/my/internal/adapter/db/postgres/graph"
	postgresidentity "github.com/mycroblog/my/internal/adapter/db/postgres/identity"
	postgresnotification "github.com/mycroblog/my/internal/adapter/db/postgres/notification"
	postgrestimeline "github.com/mycroblog/my/internal/adapter/db/postgres/timeline"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestAccountErasureContract(t *testing.T) {
	federationtest.AccountErasureContract(t, func(t *testing.T, opts uow.Options) federationtest.AccountErasureHarness {
		db := newTestDB(t, opts)
		return federationtest.AccountErasureHarness{
			RetentionHarness: federationtest.RetentionHarness{
				RemoteStatusHarness: federationtest.RemoteStatusHarness{
					AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
					Graph:          postgresgraph.NewRepository(db),
					Statuses:       postgrescontent.NewStatusRepository(db),
					Media:          postgrescontent.NewMediaRepository(db),
					Timeline:       postgrestimeline.NewRepository(db),
					Notifications:  postgresnotification.NewRepository(db),
					Inbox:          NewRepository(db),
				},
				Interactions: postgrescontent.NewInteractionRepository(db),
				Federation:   NewRepository(db),
				Retention:    postgrescontent.NewStatusRepository(db),
				Purges:       NewRepository(db),
				Instances:    NewRepository(db),
				Profiles:     postgresaccount.NewRepository(db),
			},
			AccountErasure:      postgresaccount.NewRepository(db),
			IdentityErasure:     postgresidentity.NewRepository(db),
			ContentErasure:      postgrescontent.NewStatusRepository(db),
			GraphErasure:        postgresgraph.NewRepository(db),
			TimelineErasure:     postgrestimeline.NewRepository(db),
			NotificationErasure: postgresnotification.NewRepository(db),
			Rows: func(t *testing.T, table, column, value string) int {
				t.Helper()
				var n int
				// The table and column names come from the contract, never from input.
				q := "SELECT count(*) FROM " + table + " WHERE " + column + " = $1"
				if err := db.Pool().QueryRow(t.Context(), q, value).Scan(&n); err != nil {
					t.Fatalf("count %s: %v", table, err)
				}
				return n
			},
		}
	})
}
