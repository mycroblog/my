// SPDX-License-Identifier: AGPL-3.0-only

// Package graph implements the ports of the graph module on PostgreSQL: its repositories run the
// queries of package graphgen in the database and the transactions of package postgres.
package graph

import (
	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/graphgen"
	"github.com/mycroblog/my/internal/app/uow"

	// Links the module's tables, and those of the modules it depends on, into the test
	// databases (package ddl).
	_ "github.com/mycroblog/my/schema/05-graph"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *postgres.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*graphgen.Queries, error) {
	pgTx, err := d.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return graphgen.New(pgTx), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *graphgen.Queries { return graphgen.New(d.Reader()) }
