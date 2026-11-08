// SPDX-License-Identifier: AGPL-3.0-only

// Package supporters embeds the desired-state schema of the tables of the supporters module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/supporters imports (kernel, uow, and queue are
// 10-infrastructure).
package supporters

import (
	_ "embed"

	identity "github.com/mycroblog/my/schema/01-identity"
	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	instance "github.com/mycroblog/my/schema/09-instance"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	finance "github.com/mycroblog/my/schema/12-finance"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 13-supporters.
var Module = ddl.New("13-supporters", sqlite, postgres, identity.Module, account.Module, moderation.Module, instance.Module, infrastructure.Module, finance.Module)
