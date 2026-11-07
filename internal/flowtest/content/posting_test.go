// SPDX-License-Identifier: AGPL-3.0-only

// Package content_test runs the content flows (testkit.PostingFlows) once, on SQLite (ADR-0153 §1).
package content_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestPostingFlows(t *testing.T) {
	t.Parallel()
	testkit.PostingFlows(t, func(t *testing.T, opts uow.Options) testkit.PostingHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.PostingHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Instance:       db.InstanceRepository(),
			Statuses:       db.StatusRepository(),
			Media:          db.MediaRepository(),
		}
	})
}
