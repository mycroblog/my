// SPDX-License-Identifier: AGPL-3.0-only

// Package hook implements the ports of the decision hooks (hook) on PostgreSQL: its repositories run the
// queries of package hookgen in the database and the transactions of package postgres.
package hook

import (
	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/hookgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *postgres.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*hookgen.Queries, error) {
	pgTx, err := d.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return hookgen.New(pgTx), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *hookgen.Queries { return hookgen.New(d.Reader()) }
