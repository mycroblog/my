// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestRemoteInteractionFlows(t *testing.T) {
	t.Parallel()
	testkit.RemoteInteractionFlows(t, func(t *testing.T, opts uow.Options) testkit.RemoteInteractionHarness {
		db := flowtest.SQLite(t, opts)
		feds := db.FederationRepository()
		return testkit.RemoteInteractionHarness{
			RemoteStatusHarness: testkit.RemoteStatusHarness{
				AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
				Graph:          db.GraphRepository(),
				Statuses:       db.StatusRepository(),
				Media:          db.MediaRepository(),
				Timeline:       db.TimelineRepository(),
				Notifications:  db.NotificationRepository(),
				Inbox:          feds,
				Quotes:         db.StatusRepository(),
			},
			Interactions: db.InteractionRepository(),
			Federation:   feds,
		}
	})
}
