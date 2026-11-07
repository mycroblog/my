// SPDX-License-Identifier: AGPL-3.0-only

// Package supporters_test runs the supporter flows (supporterstest.SupporterFlows) once, on SQLite with
// the fake payment provider (ADR-0153 §1).
package supporters_test

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	sqlitesupporters "github.com/mycroblog/my/internal/adapter/db/sqlite/supporters"
	"github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
	"github.com/mycroblog/my/pkg/testkit/supporterstest"
)

func TestSupporterFlows(t *testing.T) {
	t.Parallel()
	supporterstest.SupporterFlows(t, func(t *testing.T, opts uow.Options) supporterstest.SupporterHarness {
		db := flowtest.SQLite(t, opts)
		return supporterstest.SupporterHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Supporters:     sqlitesupporters.NewSupporterRepository(db),
			NewWorld: func(o supporterstest.PaymentWorldOptions) supporterstest.PaymentWorld {
				return fake.New(fake.Options{Name: o.Name, Mode: o.Mode, Now: o.Now, PageSize: o.PageSize})
			},
		}
	})
}
