// SPDX-License-Identifier: AGPL-3.0-only

// Package content_test runs the content flows (contenttest.PostingFlows) once, on SQLite (ADR-0153 §1).
package content_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/contenttest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestPostingFlows(t *testing.T) {
	t.Parallel()
	contenttest.PostingFlows(t, func(t *testing.T, opts uow.Options) contenttest.PostingHarness {
		db := flowtest.SQLite(t, opts)
		return contenttest.PostingHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Graph:          db.GraphRepository(),
			Instance:       db.InstanceRepository(),
			Statuses:       db.StatusRepository(),
			Media:          db.MediaRepository(),
		}
	})
}
