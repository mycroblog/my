// SPDX-License-Identifier: AGPL-3.0-only

// Package voice embeds the desired-state schema of the tables of the voice module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/voice imports (kernel, uow, and queue are
// 10-infrastructure).
package voice

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	content "github.com/mycroblog/my/schema/04-content"
	graph "github.com/mycroblog/my/schema/05-graph"
	instance "github.com/mycroblog/my/schema/09-instance"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 14-voice.
var Module = ddl.New("14-voice", sqlite, postgres, account.Module, moderation.Module, content.Module, graph.Module, instance.Module, infrastructure.Module)
