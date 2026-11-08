// SPDX-License-Identifier: AGPL-3.0-only

package finance

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/financetest"
)

func TestFinanceRepositoryContract(t *testing.T) {
	financetest.FinanceRepositoryContract(t, func(t *testing.T, opts uow.Options) financetest.FinanceHarness {
		db := newTestDB(t, opts)
		return financetest.FinanceHarness{UnitOfWork: db, Finance: NewRepository(db)}
	})
}
