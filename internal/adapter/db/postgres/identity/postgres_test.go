// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"testing"

	postgresaccount "github.com/mycroblog/my/internal/adapter/db/postgres/account"
	postgresgraph "github.com/mycroblog/my/internal/adapter/db/postgres/graph"
	postgresmoderation "github.com/mycroblog/my/internal/adapter/db/postgres/moderation"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

func TestAccountRepositoryContract(t *testing.T) {
	identitytest.AccountRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: postgresaccount.NewRepository(db)}
	})
}

func TestPreferencesRepositoryContract(t *testing.T) {
	identitytest.PreferencesRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: postgresaccount.NewRepository(db)}
	})
}

func TestRegistrationRepositoryContract(t *testing.T) {
	identitytest.RegistrationRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: postgresaccount.NewRepository(db)}
	})
}

func TestRoleRepositoryContract(t *testing.T) {
	identitytest.RoleRepositoryContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountHarness{UnitOfWork: db, Identity: NewRepository(db), Accounts: postgresaccount.NewRepository(db)}
	})
}

func TestAdminAccountContract(t *testing.T) {
	identitytest.AdminAccountContract(t, func(t *testing.T, opts uow.Options) identitytest.AdminAccountHarness {
		db := newTestDB(t, opts)
		return identitytest.AdminAccountHarness{
			UnitOfWork: db, Identity: NewRepository(db), Accounts: postgresaccount.NewRepository(db), Moderation: postgresmoderation.NewRepository(db),
		}
	})
}

func TestAccountSearchContract(t *testing.T) {
	identitytest.AccountSearchContract(t, func(t *testing.T, opts uow.Options) identitytest.AccountSearchHarness {
		db := newTestDB(t, opts)
		return identitytest.AccountSearchHarness{
			UnitOfWork: db, Identity: NewRepository(db), Accounts: postgresaccount.NewRepository(db),
			Graph: postgresgraph.NewRepository(db), Moderation: postgresmoderation.NewRepository(db),
		}
	})
}

func TestLoginProviderRepositoryContract(t *testing.T) {
	identitytest.LoginProviderRepositoryContract(t, func(t *testing.T) identitytest.LoginProviderHarness {
		db := newTestDB(t, uow.Options{})
		return identitytest.LoginProviderHarness{UnitOfWork: db, Identity: NewRepository(db)}
	})
}
