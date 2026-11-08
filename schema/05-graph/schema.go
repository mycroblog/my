// SPDX-License-Identifier: AGPL-3.0-only

// Package graph embeds the desired-state schema of the tables of the graph module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/graph imports (kernel, uow, and queue are
// 10-infrastructure).
package graph

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 05-graph.
var Module = ddl.New("05-graph", sqlite, postgres, account.Module, infrastructure.Module)
