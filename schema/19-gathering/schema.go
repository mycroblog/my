// SPDX-License-Identifier: AGPL-3.0-only

// Package gathering embeds the desired-state schema of the tables of the gathering module (the
// events of the local users): sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/gathering imports (kernel and uow are
// 10-infrastructure, the notifications of the events are 07-notification, and the reports and
// the audit log of their moderation are 03-moderation).
package gathering

import (
	_ "embed"

	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	notification "github.com/mycroblog/my/schema/07-notification"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 19-gathering.
var Module = ddl.New("19-gathering", sqlite, postgres, account.Module, moderation.Module, notification.Module, infrastructure.Module)
