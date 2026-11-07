// SPDX-License-Identifier: AGPL-3.0-only

// Package supporters_test runs the supporter flows (testkit.SupporterFlows) once, on SQLite with
// the fake payment provider (ADR-0153 §1).
package supporters_test

import (
	"testing"

	"github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

func TestSupporterFlows(t *testing.T) {
	t.Parallel()
	testkit.SupporterFlows(t, func(t *testing.T, opts uow.Options) testkit.SupporterHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.SupporterHarness{
			AccountHarness: testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()},
			Supporters:     db.SupporterRepository(),
			NewWorld: func(o testkit.PaymentWorldOptions) testkit.PaymentWorld {
				return fake.New(fake.Options{Name: o.Name, Mode: o.Mode, Now: o.Now, PageSize: o.PageSize})
			},
		}
	})
}
