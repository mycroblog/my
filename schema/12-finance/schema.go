// SPDX-License-Identifier: AGPL-3.0-only

// Package finance embeds the desired-state schema of the tables of the finance module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/finance imports (kernel, uow, and queue are
// 10-infrastructure).
package finance

import (
	_ "embed"

	moderation "github.com/mycroblog/my/schema/03-moderation"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 12-finance.
var Module = ddl.New("12-finance", sqlite, postgres, moderation.Module, infrastructure.Module)
