// SPDX-License-Identifier: AGPL-3.0-only

// Package hook implements the ports of the decision hooks (hook) on SQLite: its repositories run the
// queries of package hookgen in the database and the transactions of package sqlite.
package hook

import (
	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/hookgen"
	"github.com/mycroblog/my/internal/app/uow"

	// Links the module's tables, and those of the modules it depends on, into the test
	// databases (package ddl).
	_ "github.com/mycroblog/my/schema/16-hook"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *sqlite.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*hookgen.Queries, error) {
	db, err := d.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return hookgen.New(db), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *hookgen.Queries { return hookgen.New(d.Reader()) }
