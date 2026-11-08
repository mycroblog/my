// SPDX-License-Identifier: AGPL-3.0-only

// Package notification embeds the desired-state schema of the tables of the notification module:
// sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/notification imports (kernel, uow, and queue are
// 10-infrastructure).
package notification

import (
	_ "embed"

	identity "github.com/mycroblog/my/schema/01-identity"
	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	content "github.com/mycroblog/my/schema/04-content"
	graph "github.com/mycroblog/my/schema/05-graph"
	instance "github.com/mycroblog/my/schema/09-instance"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	webhook "github.com/mycroblog/my/schema/11-webhook"
	supporters "github.com/mycroblog/my/schema/13-supporters"
	diagnostics "github.com/mycroblog/my/schema/15-diagnostics"
	hook "github.com/mycroblog/my/schema/16-hook"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 07-notification.
var Module = ddl.New("07-notification", sqlite, postgres, identity.Module, account.Module, moderation.Module, content.Module, graph.Module, instance.Module, infrastructure.Module, webhook.Module, supporters.Module, diagnostics.Module, hook.Module)
