// SPDX-License-Identifier: AGPL-3.0-only

// Package timeline embeds the desired-state schema of the tables of the timeline module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/timeline imports (kernel, uow, and queue are
// 10-infrastructure).
package timeline

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	content "github.com/mycroblog/my/schema/04-content"
	graph "github.com/mycroblog/my/schema/05-graph"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 06-timeline.
var Module = ddl.New("06-timeline", sqlite, postgres, account.Module, moderation.Module, content.Module, graph.Module, infrastructure.Module)
