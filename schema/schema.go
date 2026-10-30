// SPDX-License-Identifier: AGPL-3.0-only

// Package schema embeds the desired-state schema (ADR-0023, ADR-0126) so that Go code can apply it
// without depending on the working directory. Each dialect's schema is the directory schema/<dialect>, one
// file per module (ADR-0082); SQLite and Postgres hold its files concatenated in name order, the
// order in which Atlas executes them.
//
// `my schema apply` applies these files with the Atlas Go library in every real deployment (and in
// `scripts/task.sh db:apply`); PostgresBootstrap runs first on PostgreSQL. Database adapter tests
// instead execute them directly on a new, empty database: on an empty database Atlas's plan
// consists of exactly the statements in the files, and CI's `scripts/task.sh db:check-all` verifies, for both
// dialects, that `my schema apply` applies the same files cleanly and that the Atlas CLI finds no diff
// left. The Atlas CLI also checks changes to them (`scripts/task.sh db:lint`).
//
// Tables lists the tables a schema declares. The readiness check of the database adapters uses
// it to decide whether the schema is applied: every declared table exists.
package schema

import (
	"embed"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed sqlite/*.sql postgres/*.sql
var files embed.FS

// SQLite is the files of schema/sqlite, concatenated in name order.
var SQLite = concat("sqlite")

// Postgres is the files of schema/postgres, concatenated in name order.
var Postgres = concat("postgres")

// PostgresBootstrap is schema/postgres.bootstrap.sql, which runs before Postgres on every
// PostgreSQL database.
//
//go:embed postgres.bootstrap.sql
var PostgresBootstrap string

// createTable matches a CREATE TABLE statement that starts a line and captures the table name.
var createTable = regexp.MustCompile(`(?m)^CREATE TABLE (?:IF NOT EXISTS )?"?([A-Za-z_][A-Za-z0-9_]*)"? *\(`)

// concat returns the .sql files of the directory dir, concatenated in name order (fs.ReadDir sorts
// by name).
func concat(dir string) string {
	entries, err := fs.ReadDir(files, dir)
	if err != nil {
		panic(err)
	}
	var b strings.Builder
	for _, e := range entries {
		data, err := fs.ReadFile(files, dir+"/"+e.Name())
		if err != nil {
			panic(err)
		}
		b.Write(data)
	}
	return b.String()
}

// Tables returns the names of the tables that the schema ddl declares with CREATE TABLE, in
// order.
func Tables(ddl string) []string {
	var names []string
	for _, m := range createTable.FindAllStringSubmatch(ddl, -1) {
		names = append(names, m[1])
	}
	return names
}
