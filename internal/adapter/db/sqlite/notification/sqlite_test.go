// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitefederation "github.com/mycroblog/my/internal/adapter/db/sqlite/federation"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
	"github.com/mycroblog/my/pkg/testkit/notificationtest"
)

func TestNotificationContract(t *testing.T) {
	t.Parallel()
	notificationtest.NotificationContract(t, func(t *testing.T, opts uow.Options) notificationtest.NotificationHarness {
		db, _ := newTestDB(t, "", opts)
		return notificationtest.NotificationHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          sqlitegraph.NewRepository(db),
			Statuses:       sqlitecontent.NewStatusRepository(db),
			Interactions:   sqlitecontent.NewInteractionRepository(db),
			Notifications:  NewRepository(db),
		}
	})
}

func TestPushSubscriptionContract(t *testing.T) {
	t.Parallel()
	notificationtest.PushSubscriptionContract(t, func(t *testing.T) notificationtest.PushHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return notificationtest.PushHarness{UnitOfWork: db, Push: NewRepository(db)}
	})
}

func TestSeveranceContract(t *testing.T) {
	t.Parallel()
	notificationtest.SeveranceContract(t, func(t *testing.T, opts uow.Options) notificationtest.SeveranceHarness {
		db, _ := newTestDB(t, "", opts)
		feds := sqlitefederation.NewRepository(db)
		return notificationtest.SeveranceHarness{
			RemoteFollowHarness: federationtest.RemoteFollowHarness{
				FollowHarness: graphtest.FollowHarness{
					AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
					Graph:          sqlitegraph.NewRepository(db),
				},
				Notifications: NewRepository(db),
				Federation:    feds,
				Inbox:         feds,
			},
			Moderation: sqlitemoderation.NewRepository(db),
		}
	})
}
