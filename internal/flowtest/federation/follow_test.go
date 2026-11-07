// SPDX-License-Identifier: AGPL-3.0-only

// Package federation_test runs the flows across modules whose most dependent module is federation
// (ADR-0153 §1, §3) once, on SQLite.
package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// remoteFollowHarness is the harness of the follow and move flows on db.
func remoteFollowHarness(db *sqlite.DB) federationtest.RemoteFollowHarness {
	feds := db.FederationRepository()
	return federationtest.RemoteFollowHarness{
		FollowHarness: graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		},
		Notifications: db.NotificationRepository(),
		Federation:    feds,
		Inbox:         feds,
	}
}

func TestRemoteFollowFlows(t *testing.T) {
	t.Parallel()
	federationtest.RemoteFollowFlows(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		return remoteFollowHarness(flowtest.SQLite(t, opts))
	})
}

func TestAccountMoveFlows(t *testing.T) {
	t.Parallel()
	federationtest.AccountMoveFlows(t, func(t *testing.T, opts uow.Options) federationtest.RemoteFollowHarness {
		return remoteFollowHarness(flowtest.SQLite(t, opts))
	})
}
