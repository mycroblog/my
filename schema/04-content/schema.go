// SPDX-License-Identifier: AGPL-3.0-only

// Package content embeds the desired-state schema of the tables of the content module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/content imports (kernel, uow, and queue are
// 10-infrastructure).
package content

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	graph "github.com/mycroblog/my/schema/05-graph"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	hook "github.com/mycroblog/my/schema/16-hook"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 04-content.
var Module = ddl.New("04-content", sqlite, postgres, account.Module, graph.Module, infrastructure.Module, hook.Module)
