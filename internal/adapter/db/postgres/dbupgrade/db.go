// SPDX-License-Identifier: AGPL-3.0-only

// Package dbupgrade implements the ports of the data upgrades (dbupgrade) on PostgreSQL: its repositories run the
// queries of package dbupgradegen in the database and the transactions of package postgres.
package dbupgrade

import (
	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/dbupgradegen"
	"github.com/mycroblog/my/internal/app/uow"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *postgres.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*dbupgradegen.Queries, error) {
	pgTx, err := d.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return dbupgradegen.New(pgTx), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *dbupgradegen.Queries { return dbupgradegen.New(d.Reader()) }
