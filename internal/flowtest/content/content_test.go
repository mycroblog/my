// SPDX-License-Identifier: AGPL-3.0-only

// Package content_test runs the content flows of pkg/testkit once, on SQLite (ADR-0153 §1).
package content_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestInteractionFlows(t *testing.T) {
	t.Parallel()
	testkit.InteractionFlows(t, func(t *testing.T, opts uow.Options) testkit.InteractionHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()}
	})
}

func TestAutoDeleteFlows(t *testing.T) {
	t.Parallel()
	testkit.AutoDeleteFlows(t, func(t *testing.T, opts uow.Options) testkit.AutoDeleteHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.AutoDeleteHarness{
			InteractionHarness: testkit.InteractionHarness{UnitOfWork: db, Statuses: db.StatusRepository(), Interactions: db.InteractionRepository()},
			AutoDeletes:        db.StatusRepository(),
		}
	})
}
