// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/federationtest"
)

func TestRemoteActorFlows(t *testing.T) {
	t.Parallel()
	federationtest.RemoteActorFlows(t, func(t *testing.T, opts uow.Options) federationtest.RemoteActorHarness {
		db := flowtest.SQLite(t, opts)
		feds := db.FederationRepository()
		return federationtest.RemoteActorHarness{UnitOfWork: db, Accounts: db.AccountRepository(), Federation: feds, Instances: feds}
	})
}
