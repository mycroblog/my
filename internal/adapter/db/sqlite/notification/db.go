// SPDX-License-Identifier: AGPL-3.0-only

// Package notification implements the ports of the notification module on SQLite: its repositories run the
// queries of package notificationgen in the database and the transactions of package sqlite.
package notification

import (
	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/notificationgen"
	"github.com/mycroblog/my/internal/app/uow"

	// Links the module's tables, and those of the modules it depends on, into the test
	// databases (package ddl).
	_ "github.com/mycroblog/my/schema/07-notification"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *sqlite.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*notificationgen.Queries, error) {
	db, err := d.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return notificationgen.New(db), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *notificationgen.Queries { return notificationgen.New(d.Reader()) }
