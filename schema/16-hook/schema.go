// SPDX-License-Identifier: AGPL-3.0-only

// Package hook embeds the desired-state schema of the tables of the hook module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/hook imports (kernel, uow, and queue are
// 10-infrastructure).
package hook

import (
	_ "embed"

	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 16-hook.
var Module = ddl.New("16-hook", sqlite, postgres, infrastructure.Module)
