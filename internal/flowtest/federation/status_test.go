// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestRemoteStatusFlows(t *testing.T) {
	t.Parallel()
	testkit.RemoteStatusFlows(t, func(t *testing.T, opts uow.Options) testkit.RemoteStatusHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.RemoteStatusHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Statuses:       db.StatusRepository(),
			Media:          db.MediaRepository(),
			Timeline:       db.TimelineRepository(),
			Notifications:  db.NotificationRepository(),
			Inbox:          db.FederationRepository(),
			Federation:     db.FederationRepository(),
			Instances:      db.FederationRepository(),
			Pins:           db.InteractionRepository(),
			Quotes:         db.StatusRepository(),
		}
	})
}
