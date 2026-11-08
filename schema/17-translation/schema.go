// SPDX-License-Identifier: AGPL-3.0-only

// Package translation embeds the desired-state schema of the tables of the translation module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/translation imports (kernel, uow, and queue are
// 10-infrastructure).
package translation

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	content "github.com/mycroblog/my/schema/04-content"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 17-translation.
var Module = ddl.New("17-translation", sqlite, postgres, account.Module, content.Module, infrastructure.Module)
