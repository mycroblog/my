// SPDX-License-Identifier: AGPL-3.0-only

// Package instance embeds the desired-state schema of the tables of the instance module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/instance imports (kernel, uow, and queue are
// 10-infrastructure).
package instance

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 09-instance.
var Module = ddl.New("09-instance", sqlite, postgres, account.Module, moderation.Module, infrastructure.Module)
