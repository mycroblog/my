// SPDX-License-Identifier: AGPL-3.0-only

// Package identity_test runs the identity and account flows once, on SQLite (ADR-0153 §1).
package identity_test

import (
	"testing"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/flowtest"
	"github.com/mycroblog/my/pkg/testkit"
)

// accountHarness is the harness of the flows that use the identity and account repositories.
func accountHarness(t *testing.T, opts uow.Options) testkit.AccountHarness {
	db := flowtest.SQLite(t, opts)
	return testkit.AccountHarness{UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository()}
}

func TestAccountFlows(t *testing.T) {
	t.Parallel()
	testkit.AccountFlows(t, accountHarness)
}

func TestRoleFlows(t *testing.T) {
	t.Parallel()
	testkit.RoleFlows(t, accountHarness)
}

func TestRegistrationFlows(t *testing.T) {
	t.Parallel()
	testkit.RegistrationFlows(t, accountHarness)
}

func TestAdminAccountFlows(t *testing.T) {
	t.Parallel()
	testkit.AdminAccountFlows(t, func(t *testing.T, opts uow.Options) testkit.AdminAccountHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.AdminAccountHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(), Moderation: db.ModerationRepository(),
		}
	})
}

func TestAccountSearchFlows(t *testing.T) {
	t.Parallel()
	testkit.AccountSearchFlows(t, func(t *testing.T, opts uow.Options) testkit.AccountSearchHarness {
		db := flowtest.SQLite(t, opts)
		return testkit.AccountSearchHarness{
			UnitOfWork: db, Identity: db.IdentityRepository(), Accounts: db.AccountRepository(),
			Graph: db.GraphRepository(), Moderation: db.ModerationRepository(),
		}
	})
}
