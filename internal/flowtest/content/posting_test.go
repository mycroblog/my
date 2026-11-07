// SPDX-License-Identifier: AGPL-3.0-only

// Package content_test runs the content flows (contenttest.PostingFlows) once, on SQLite (ADR-0153 §1).
package content_test

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitecontent "github.com/mycroblog/my/internal/adapter/db/sqlite/content"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqliteinstance "github.com/mycroblog/my/internal/adapter/db/sqlite/instance"
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
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Graph:          sqlitegraph.NewRepository(db),
			Instance:       sqliteinstance.NewRepository(db),
			Statuses:       sqlitecontent.NewStatusRepository(db),
			Media:          sqlitecontent.NewMediaRepository(db),
		}
	})
}
