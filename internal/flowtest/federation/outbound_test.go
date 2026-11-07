// SPDX-License-Identifier: AGPL-3.0-only

package federation_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestFederationFlows(t *testing.T) {
	t.Parallel()
	testkit.FederationFlows(t, func(t *testing.T, opts uow.Options) testkit.FederationHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.FederationHarness{UnitOfWork: db, Federation: db.FederationRepository()}
	})
}
