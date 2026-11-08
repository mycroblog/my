// SPDX-License-Identifier: AGPL-3.0-only

// Package moderation embeds the desired-state schema of the tables of the moderation module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/moderation imports (kernel, uow, and queue are
// 10-infrastructure).
package moderation

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	content "github.com/mycroblog/my/schema/04-content"
	graph "github.com/mycroblog/my/schema/05-graph"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	hook "github.com/mycroblog/my/schema/16-hook"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 03-moderation.
var Module = ddl.New("03-moderation", sqlite, postgres, account.Module, content.Module, graph.Module, infrastructure.Module, hook.Module)
