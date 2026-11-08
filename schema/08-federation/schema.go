// SPDX-License-Identifier: AGPL-3.0-only

// Package federation embeds the desired-state schema of the tables of the federation module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/federation imports (kernel, uow, and queue are
// 10-infrastructure).
package federation

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	content "github.com/mycroblog/my/schema/04-content"
	graph "github.com/mycroblog/my/schema/05-graph"
	timeline "github.com/mycroblog/my/schema/06-timeline"
	instance "github.com/mycroblog/my/schema/09-instance"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	hook "github.com/mycroblog/my/schema/16-hook"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 08-federation.
var Module = ddl.New("08-federation", sqlite, postgres, account.Module, moderation.Module, content.Module, graph.Module, timeline.Module, instance.Module, infrastructure.Module, hook.Module)
