// SPDX-License-Identifier: AGPL-3.0-only

// Package notification_test runs the notification flows (notificationtest.NotificationFlows) and the flows
// of the follows that suspensions cut (notificationtest.SeveranceFlows) once, on SQLite (ADR-0153 §1).
package notification_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
	"github.com/mycroblog/my/pkg/testkit/notificationtest"
)

func accountHarness(db *sqlite.DB) identitytest.AccountHarness {
	return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
}

func TestNotificationFlows(t *testing.T) {
	t.Parallel()
	notificationtest.NotificationFlows(t, func(t *testing.T, opts uow.Options) notificationtest.NotificationHarness {
		db := flowtest.SQLite(t, opts)
		return notificationtest.NotificationHarness{
			AccountHarness: accountHarness(db),
			Graph:          db.GraphRepository(),
			Statuses:       db.StatusRepository(),
			Interactions:   db.InteractionRepository(),
			Notifications:  db.NotificationRepository(),
		}
	})
}

func TestSeveranceFlows(t *testing.T) {
	t.Parallel()
	notificationtest.SeveranceFlows(t, func(t *testing.T, opts uow.Options) notificationtest.SeveranceHarness {
		db := flowtest.SQLite(t, opts)
		feds := db.FederationRepository()
		return notificationtest.SeveranceHarness{
			RemoteFollowHarness: federationtest.RemoteFollowHarness{
				FollowHarness: graphtest.FollowHarness{AccountHarness: accountHarness(db), Graph: db.GraphRepository()},
				Notifications: db.NotificationRepository(),
				Federation:    feds,
				Inbox:         feds,
			},
			Moderation: db.ModerationRepository(),
		}
	})
}
