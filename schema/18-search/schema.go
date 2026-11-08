// SPDX-License-Identifier: AGPL-3.0-only

// Package search embeds the desired-state schema of the tables of the search module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/search imports (kernel, uow, and queue are
// 10-infrastructure).
package search

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	content "github.com/mycroblog/my/schema/04-content"
	timeline "github.com/mycroblog/my/schema/06-timeline"
	instance "github.com/mycroblog/my/schema/09-instance"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 18-search.
var Module = ddl.New("18-search", sqlite, postgres, account.Module, moderation.Module, content.Module, timeline.Module, instance.Module, infrastructure.Module)
