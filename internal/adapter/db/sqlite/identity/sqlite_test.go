// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"testing"

	sqliteaccount "github.com/mycroblog/my/internal/adapter/db/sqlite/account"
	sqlitegraph "github.com/mycroblog/my/internal/adapter/db/sqlite/graph"
	sqlitemoderation "github.com/mycroblog/my/internal/adapter/db/sqlite/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestAccountRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.AccountRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: sqliteaccount.NewRepository(db)}
	})
}

func TestPreferencesRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.PreferencesRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: sqliteaccount.NewRepository(db)}
	})
}

func TestRegistrationRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.RegistrationRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: sqliteaccount.NewRepository(db)}
	})
}

func TestRoleRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.RoleRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: sqliteaccount.NewRepository(db)}
	})
}

func TestAdminAccountContract(t *testing.T) {
	t.Parallel()
	identitytest.AdminAccountContract(t, func(t *testing.T, opts uow.Options) identitytest.AdminAccountHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AdminAccountHarness{
			UnitOfWork: db, Identity: NewRepository(db), Accounts: sqliteaccount.NewRepository(db), Moderation: sqlitemoderation.NewRepository(db),
		}
	})
}

func TestAccountSearchContract(t *testing.T) {
	t.Parallel()
	identitytest.AccountSearchContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountSearchHarness {
		db, _ := newTestDB(t, "", opts)
		return identitytest.AccountSearchHarness{
			UnitOfWork: db, Identity: NewRepository(db), Accounts: sqliteaccount.NewRepository(db),
			Graph: sqlitegraph.NewRepository(db), Moderation: sqlitemoderation.NewRepository(db),
		}
	})
}

func TestLoginProviderRepositoryContract(t *testing.T) {
	t.Parallel()
	identitytest.LoginProviderRepositoryContract(t, func(t *testing.T) identitytest.LoginProviderHarness {
		db, _ := newTestDB(t, "", uow.Options{})
		return identitytest.LoginProviderHarness{UnitOfWork: db, Identity: NewRepository(db)}
	})
}
