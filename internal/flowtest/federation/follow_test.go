// SPDX-License-Identifier: AGPL-3.0-only

// Package federation_test runs the flows across modules whose most dependent module is federation
// (ADR-0153 §1, §3) once, on SQLite.
package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitefederation "github.com/mycroblog/my/internal/adapter/db/sqlite/federation"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqlitenotification "github.com/mycroblog/my/internal/adapter/db/sqlite/notification"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// remoteFollowHarness is the harness of the follow and move flows on db.
func remoteFollowHarness(db *sqlite.DB) federationtest.RemoteFollowHarness {
	feds := sqlitefederation.NewRepository(db)
	return federationtest.RemoteFollowHarness{
		FollowHarness: graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          sqlitegraph.NewRepository(db),
		},
		Notifications: sqlitenotification.NewRepository(db),
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
