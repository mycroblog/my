// SPDX-License-Identifier: AGPL-3.0-only

// Package account embeds the desired-state schema of the tables of the account module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/account imports (kernel, uow, and queue are
// 10-infrastructure).
package account

import (
	_ "embed"

	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	hook "github.com/mycroblog/my/schema/16-hook"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 02-account.
var Module = ddl.New("02-account", sqlite, postgres, infrastructure.Module, hook.Module)
