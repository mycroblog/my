// SPDX-License-Identifier: AGPL-3.0-only

// Package diagnostics embeds the desired-state schema of the tables of the diagnostics module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/diagnostics imports (kernel, uow, and queue are
// 10-infrastructure).
package diagnostics

import (
	_ "embed"

	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 15-diagnostics.
var Module = ddl.New("15-diagnostics", sqlite, postgres, infrastructure.Module)
