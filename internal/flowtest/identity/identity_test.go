// SPDX-License-Identifier: AGPL-3.0-only

// Package identity_test runs the identity and account flows once, on SQLite (ADR-0153 §1).
package identity_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit/identitytest"
)

// accountHarness is the harness of the flows that use the identity and account repositories.
func accountHarness(t *testing.T, opts uow.Options) identitytest.AccountHarness {
	db := flowtest.SQLite(t, opts)
	return identitytest.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
}

func TestAccountFlows(t *testing.T) {
	t.Parallel()
	identitytest.AccountFlows(t, accountHarness)
}

func TestRoleFlows(t *testing.T) {
	t.Parallel()
	identitytest.RoleFlows(t, accountHarness)
}

func TestRegistrationFlows(t *testing.T) {
	t.Parallel()
	identitytest.RegistrationFlows(t, accountHarness)
}

func TestAdminAccountFlows(t *testing.T) {
	t.Parallel()
	identitytest.AdminAccountFlows(t, func(t *testing.T, opts uow.Options) identitytest.AdminAccountHarness {
		db := flowtest.SQLite(t, opts)
		return identitytest.AdminAccountHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAccountSearchFlows(t *testing.T) {
	t.Parallel()
	identitytest.AccountSearchFlows(t, func(t *testing.T, opts uow.Options) identitytest.AccountSearchHarness {
		db := flowtest.SQLite(t, opts)
		return identitytest.AccountSearchHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}
