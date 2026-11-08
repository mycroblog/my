// SPDX-License-Identifier: AGPL-3.0-only

// Package webhook embeds the desired-state schema of the tables of the webhook module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/webhook imports (kernel, uow, and queue are
// 10-infrastructure).
package webhook

import (
	_ "embed"

	identity "github.com/mycroblog/my/schema/01-identity"
	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	content "github.com/mycroblog/my/schema/04-content"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	diagnostics "github.com/mycroblog/my/schema/15-diagnostics"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 11-webhook.
var Module = ddl.New("11-webhook", sqlite, postgres, identity.Module, account.Module, moderation.Module, content.Module, infrastructure.Module, diagnostics.Module)
