// SPDX-License-Identifier: AGPL-3.0-only

// Package schema embeds the whole desired-state schema (ADR-0023) so that Go code can apply it
// without depending on the working directory. The schema is one directory per module,
// schema/<NN>-<module>, each a package that embeds the module's sqlite.sql and postgres.sql
// (package ddl, ADR-0153 §3); Modules lists them all, and SQLite and Postgres hold their files
// concatenated in name order, the order in which Atlas executes them.
//
// `my schema apply` applies these files with the Atlas Go library in every real deployment (and in
// `scripts/task.sh db:apply`); PostgresBootstrap runs first on PostgreSQL. Database adapter tests
// instead execute the files of the modules they link directly on a new, empty database
// (pkg/testkit/sqlitetest, pgtest): on an empty database Atlas's plan consists of exactly the
// statements in the files, and CI's `scripts/task.sh db:check-all` verifies, for both dialects,
// that `my schema apply` applies the same files cleanly and that the Atlas CLI finds no diff left.
// The Atlas CLI also checks changes to them (`scripts/task.sh db:lint`).
//
// Tables lists the tables a schema declares. The readiness check of the database adapters uses
// it to decide whether the schema is applied: every declared table exists.
package schema

import (
	identity "github.com/mycroblog/my/schema/01-identity"
	account "github.com/mycroblog/my/schema/02-account"
	moderation "github.com/mycroblog/my/schema/03-moderation"
	content "github.com/mycroblog/my/schema/04-content"
	graph "github.com/mycroblog/my/schema/05-graph"
	timeline "github.com/mycroblog/my/schema/06-timeline"
	notification "github.com/mycroblog/my/schema/07-notification"
	federation "github.com/mycroblog/my/schema/08-federation"
	instance "github.com/mycroblog/my/schema/09-instance"
	infrastructure "github.com/mycroblog/my/schema/10-infrastructure"
	webhook "github.com/mycroblog/my/schema/11-webhook"
	finance "github.com/mycroblog/my/schema/12-finance"
	supporters "github.com/mycroblog/my/schema/13-supporters"
	voice "github.com/mycroblog/my/schema/14-voice"
	diagnostics "github.com/mycroblog/my/schema/15-diagnostics"
	hook "github.com/mycroblog/my/schema/16-hook"
	translation "github.com/mycroblog/my/schema/17-translation"
	search "github.com/mycroblog/my/schema/18-search"
	"github.com/mycroblog/my/schema/ddl"
)

// Modules is every module's schema, in name order.
var Modules = []*ddl.Module{
	identity.Module, account.Module, moderation.Module, content.Module, graph.Module, timeline.Module,
	notification.Module, federation.Module, instance.Module, infrastructure.Module, webhook.Module,
	finance.Module, supporters.Module, voice.Module, diagnostics.Module, hook.Module,
	translation.Module, search.Module,
}

// SQLite is the sqlite.sql files of Modules, concatenated in name order.
var SQLite = ddl.SQLite(Modules)

// Postgres is the postgres.sql files of Modules, concatenated in name order.
var Postgres = ddl.Postgres(Modules)

// PostgresBootstrap is schema/ddl/postgres.bootstrap.sql, which runs before Postgres on every
// PostgreSQL database.
var PostgresBootstrap = ddl.PostgresBootstrap

// Tables returns the names of the tables that the schema ddl declares with CREATE TABLE, in
// order.
func Tables(schema string) []string { return ddl.Tables(schema) }
