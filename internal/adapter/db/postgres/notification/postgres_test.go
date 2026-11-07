// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"testing"

	postgresaccount "github.com/mycroblog/my/internal/adapter/db/postgres/account"
	postgrescontent "github.com/mycroblog/my/internal/adapter/db/postgres/content"
	postgresfederation "github.com/mycroblog/my/internal/adapter/db/postgres/federation"
	postgresgraph "github.com/mycroblog/my/internal/adapter/db/postgres/graph"
	postgresidentity "github.com/mycroblog/my/internal/adapter/db/postgres/identity"
	postgresmoderation "github.com/mycroblog/my/internal/adapter/db/postgres/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
	"github.com/mycroblog/my/pkg/testkit/notificationtest"
)

func TestNotificationContract(t *testing.T) {
	notificationtest.NotificationContract(t, func(t *testing.T, opts uow.Options) notificationtest.NotificationHarness {
		db := newTestDB(t, opts)
		return notificationtest.NotificationHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
			Graph:          postgresgraph.NewRepository(db),
			Statuses:       postgrescontent.NewStatusRepository(db),
			Interactions:   postgrescontent.NewInteractionRepository(db),
			Notifications:  NewRepository(db),
		}
	})
}

func TestPushSubscriptionContract(t *testing.T) {
	notificationtest.PushSubscriptionContract(t, func(t *testing.T) notificationtest.PushHarness {
		db := newTestDB(t, uow.Options{})
		return notificationtest.PushHarness{UnitOfWork: db, Push: NewRepository(db)}
	})
}

func TestSeveranceContract(t *testing.T) {
	notificationtest.SeveranceContract(t, func(t *testing.T, opts uow.Options) notificationtest.SeveranceHarness {
		db := newTestDB(t, opts)
		feds := postgresfederation.NewRepository(db)
		return notificationtest.SeveranceHarness{
			RemoteFollowHarness: federationtest.RemoteFollowHarness{
				FollowHarness: graphtest.FollowHarness{
					AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: postgresidentity.NewRepository(db), Accounts: postgresaccount.NewRepository(db)},
					Graph:          postgresgraph.NewRepository(db),
				},
				Notifications: NewRepository(db),
				Federation:    feds,
				Inbox:         feds,
			},
			Moderation: postgresmoderation.NewRepository(db),
		}
	})
}
