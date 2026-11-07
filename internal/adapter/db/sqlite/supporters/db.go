// SPDX-License-Identifier: AGPL-3.0-only

// Package supporters implements the ports of the supporters module on SQLite: its repositories run the
// queries of package supportersgen in the database and the transactions of package sqlite.
package supporters

import (
	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/supportersgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *sqlite.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*supportersgen.Queries, error) {
	db, err := d.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return supportersgen.New(db), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *supportersgen.Queries { return supportersgen.New(d.Reader()) }
