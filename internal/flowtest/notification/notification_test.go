// SPDX-License-Identifier: AGPL-3.0-only

// Package notification_test runs the notification flows (notificationtest.NotificationFlows) and the flows
// of the follows that suspensions cut (notificationtest.SeveranceFlows) once, on SQLite (ADR-0153 §1).
package notification_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitefederation "github.com/mycroblog/my/internal/adapter/db/sqlite/federation"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	sqlitenotification "github.com/mycroblog/my/internal/adapter/db/sqlite/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
	"github.com/mycroblog/my/pkg/testkit/notificationtest"
)

func accountHarness(db *sqlite.DB) identitytest.AccountHarness {
	return identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)}
}

func TestNotificationFlows(t *testing.T) {
	t.Parallel()
	notificationtest.NotificationFlows(t, func(t *testing.T, opts uow.Options) notificationtest.NotificationHarness {
		db := flowtest.SQLite(t, opts)
		return notificationtest.NotificationHarness{
			AccountHarness: accountHarness(db),
			Graph:          sqlitegraph.NewRepository(db),
			Statuses:       sqlitecontent.NewStatusRepository(db),
			Interactions:   sqlitecontent.NewInteractionRepository(db),
			Notifications:  sqlitenotification.NewRepository(db),
		}
	})
}

func TestSeveranceFlows(t *testing.T) {
	t.Parallel()
	notificationtest.SeveranceFlows(t, func(t *testing.T, opts uow.Options) notificationtest.SeveranceHarness {
		db := flowtest.SQLite(t, opts)
		feds := sqlitefederation.NewRepository(db)
		return notificationtest.SeveranceHarness{
			RemoteFollowHarness: federationtest.RemoteFollowHarness{
				FollowHarness: graphtest.FollowHarness{AccountHarness: accountHarness(db), Graph: sqlitegraph.NewRepository(db)},
				Notifications: sqlitenotification.NewRepository(db),
				Federation:    feds,
				Inbox:         feds,
			},
			Moderation: sqlitemoderation.NewRepository(db),
		}
	})
}
