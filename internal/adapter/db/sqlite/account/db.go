// SPDX-License-Identifier: AGPL-3.0-only

// Package account implements the ports of the account module on SQLite: its repositories run the
// queries of package accountgen in the database and the transactions of package sqlite.
package account

import (
	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/accountgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *sqlite.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*accountgen.Queries, error) {
	db, err := d.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return accountgen.New(db), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *accountgen.Queries { return accountgen.New(d.Reader()) }

// in returns a copy of d whose reads run in tx, for the repositories' read methods that take a
// transaction (their ...Tx variants).
func (d *moduleDB) in(tx uow.Tx) (*moduleDB, error) {
	db, err := d.In(tx)
	if err != nil {
		return nil, err
	}
	return &moduleDB{db}, nil
}
