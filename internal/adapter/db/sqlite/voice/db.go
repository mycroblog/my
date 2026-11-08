// SPDX-License-Identifier: AGPL-3.0-only

// Package voice implements the ports of the voice module on SQLite: its repositories run the
// queries of package voicegen in the database and the transactions of package sqlite.
package voice

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/voicegen"
	"github.com/mycroblog/my/internal/app/uow"

	// Links the module's tables, and those of the modules it depends on, into the test
	// databases (package ddl).
	_ "github.com/mycroblog/my/schema/14-voice"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *sqlite.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*voicegen.Queries, error) {
	db, err := d.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return voicegen.New(db), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *voicegen.Queries { return voicegen.New(d.Reader()) }

// execAll runs the queries in order inside tx, for a rebuild that empties a projection and
// derives it again in SQL.
func (d *moduleDB) execAll(ctx context.Context, tx uow.Tx, what string, steps ...func(*voicegen.Queries, context.Context) error) error {
	q, err := d.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.RunQueries(ctx, q, what, steps...)
}
