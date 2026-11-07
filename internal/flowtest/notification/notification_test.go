// SPDX-License-Identifier: AGPL-3.0-only

// Package notification_test runs the notification flows (testkit.NotificationFlows) and the flows
// of the follows that suspensions cut (testkit.SeveranceFlows) once, on SQLite (ADR-0153 §1).
package notification_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func accountHarness(db *sqlite.DB) testkit.AccountHarness {
	return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
}

func TestNotificationFlows(t *testing.T) {
	t.Parallel()
	testkit.NotificationFlows(t, func(t *testing.T, opts uow.Options) testkit.NotificationHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.NotificationHarness{
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
	testkit.SeveranceFlows(t, func(t *testing.T, opts uow.Options) testkit.SeveranceHarness {
		db := flowtest.SQLite(t, opts)
		feds := db.FederationRepository()
		return testkit.SeveranceHarness{
			RemoteFollowHarness: testkit.RemoteFollowHarness{
				FollowHarness: testkit.FollowHarness{AccountHarness: accountHarness(db), Graph: db.GraphRepository()},
				Notifications: db.NotificationRepository(),
				Federation:    feds,
				Inbox:         feds,
			},
			Moderation: db.ModerationRepository(),
		}
	})
}
