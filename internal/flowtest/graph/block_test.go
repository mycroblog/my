// SPDX-License-Identifier: AGPL-3.0-only

// Package graph_test runs the graph flows (graphtest.BlockFlows, graphtest.FollowFlows) once, on SQLite (ADR-0153 §1).
package graph_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/graphtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestBlockFlows(t *testing.T) {
	t.Parallel()
	graphtest.BlockFlows(t, func(t *testing.T, opts uow.Options) graphtest.FollowHarness {
		db := flowtest.SQLite(t, opts)
		return graphtest.FollowHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
		}
	})
}
