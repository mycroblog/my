// SPDX-License-Identifier: AGPL-3.0-only

package supporters

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqliteidentity "github.com/mycroblog/my/internal/adapter/db/sqlite/identity"
	"github.com/mycroblog/my/internal/adapter/payment/fake"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
	"github.com/mycroblog/my/pkg/testkit/supporterstest"
)

// newPaymentWorld is the provider's side of PaymentRepositoryContract: the fake adapter.
func newPaymentWorld(o supporterstest.PaymentWorldOptions) supporterstest.PaymentWorld {
	return fake.New(fake.Options{Name: o.Name, Mode: o.Mode, Now: o.Now, PageSize: o.PageSize})
}

func TestSupporterRepositoryContract(t *testing.T) {
	t.Parallel()
	supporterstest.SupporterRepositoryContract(t, func(t *testing.T, opts uow.Options) supporterstest.SupporterHarness {
		db, _ := newTestDB(t, "", opts)
		return supporterstest.SupporterHarness{
			AccountHarness: identitytest.AccountHarness{UnitOfWork: db, Identity: sqliteidentity.NewRepository(db), Accounts: sqliteaccount.NewRepository(db)},
			Supporters:     NewSupporterRepository(db), NewWorld: newPaymentWorld,
		}
	})
}

func TestPaymentRepositoryContract(t *testing.T) {
	t.Parallel()
	supporterstest.PaymentRepositoryContract(t, func(t *testing.T, opts uow.Options) supporterstest.PaymentHarness {
		db, _ := newTestDB(t, "", opts)
		return supporterstest.PaymentHarness{UnitOfWork: db, Payments: NewPaymentRepository(db), NewWorld: newPaymentWorld}
	})
}
