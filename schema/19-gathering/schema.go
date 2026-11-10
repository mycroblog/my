// SPDX-License-Identifier: AGPL-3.0-only

// Package gathering embeds the desired-state schema of the tables of the gathering module (the
// events of the local users): sqlite.sql and postgres.sql (schema/README.md).
//
// It depends on the modules that internal/app/gathering imports (kernel and uow are
// 10-infrastructure).
package gathering

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

// Module is the schema of 19-gathering.
var Module = ddl.New("19-gathering", sqlite, postgres, account.Module, infrastructure.Module)
