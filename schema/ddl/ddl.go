// SPDX-License-Identifier: AGPL-3.0-only

// Package ddl holds the type of a module's desired-state schema and the set of modules a binary
// links (ADR-0153 §3).
//
// Each directory schema/<NN>-<module> is a package that embeds the module's sqlite.sql and
// postgres.sql and declares its Module with New, naming the modules it depends on (the imports of
// internal/app/<module>, with kernel, uow, and queue in 10-infrastructure). Importing a module's
// package therefore links it and every module it depends on, and Linked returns them: package
// schema imports every module, and the test helpers pkg/testkit/sqlitetest and pgtest apply only
// the modules their test binary links, so that a change to one module's files rebuilds only the
// test binaries that use the module.
package ddl

import (
	_ "embed"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// PostgresBootstrap is schema/ddl/postgres.bootstrap.sql, which runs before the schema on every
// PostgreSQL database: what Atlas Community Edition cannot manage, such as extensions.
//
//go:embed postgres.bootstrap.sql
var PostgresBootstrap string

// Module is the desired-state schema of one module.
type Module struct {
	// Name is the name of the module's directory, <NN>-<module>; the schema applies in the order
	// of the names.
	Name string
	// SQLite and Postgres are the module's files sqlite.sql and postgres.sql.
	SQLite, Postgres string
	// Requires lists the modules this module depends on.
	Requires []*Module
}

// linked is every Module that New has made, in the order of their names.
var linked []*Module

// New returns the Module name with the files sqlite and postgres that depends on requires, and
// adds it to Linked. A module's package calls it once, in a package-level variable.
func New(name, sqlite, postgres string, requires ...*Module) *Module {
	m := &Module{Name: name, SQLite: sqlite, Postgres: postgres, Requires: requires}
	i, found := slices.BinarySearchFunc(linked, name, func(m *Module, name string) int { return strings.Compare(m.Name, name) })
	if found {
		panic(fmt.Sprintf("ddl: the module %s is declared twice", name))
	}
	linked = slices.Insert(linked, i, m)
	return m
}

// Linked returns every module the binary links, in the order of their names.
func Linked() []*Module { return slices.Clone(linked) }

// Closure returns m and every module it depends on, directly or not, in the order of their names.
func (m *Module) Closure() []*Module {
	seen := map[*Module]bool{}
	var out []*Module
	var visit func(*Module)
	visit = func(m *Module) {
		if seen[m] {
			return
		}
		seen[m] = true
		out = append(out, m)
		for _, r := range m.Requires {
			visit(r)
		}
	}
	visit(m)
	slices.SortFunc(out, func(a, b *Module) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// SQLite returns the SQLite files of modules, concatenated in the order given.
func SQLite(modules []*Module) string {
	var b strings.Builder
	for _, m := range modules {
		b.WriteString(m.SQLite)
	}
	return b.String()
}

// Postgres returns the PostgreSQL files of modules, concatenated in the order given.
func Postgres(modules []*Module) string {
	var b strings.Builder
	for _, m := range modules {
		b.WriteString(m.Postgres)
	}
	return b.String()
}

// createTable matches a CREATE TABLE statement that starts a line and captures the table name.
var createTable = regexp.MustCompile(`(?m)^CREATE TABLE (?:IF NOT EXISTS )?"?([A-Za-z_][A-Za-z0-9_]*)"? *\(`)

// Tables returns the names of the tables that the schema ddl declares with CREATE TABLE, in
// order.
func Tables(ddl string) []string {
	var names []string
	for _, m := range createTable.FindAllStringSubmatch(ddl, -1) {
		names = append(names, m[1])
	}
	return names
}
