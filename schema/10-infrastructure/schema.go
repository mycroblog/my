// SPDX-License-Identifier: AGPL-3.0-only

// Package infrastructure embeds the desired-state schema of the tables that adapters own (kernel, uow, and queue):
// sqlite.sql and postgres.sql (schema/README.md).
package infrastructure

import (
	_ "embed"

	"github.com/mycroblog/my/schema/ddl"
)

//go:embed sqlite.sql
var sqlite string

//go:embed postgres.sql
var postgres string

// Module is the schema of 10-infrastructure.
var Module = ddl.New("10-infrastructure", sqlite, postgres)
