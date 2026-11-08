// SPDX-License-Identifier: AGPL-3.0-only

// Package timeline implements the ports of the timeline module on PostgreSQL: its repositories run the
// queries of package timelinegen in the database and the transactions of package postgres.
package timeline

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/timelinegen"
	"github.com/mycroblog/my/internal/app/uow"

	// Links the module's tables, and those of the modules it depends on, into the test
	// databases (package ddl).
	_ "github.com/mycroblog/my/schema/06-timeline"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *postgres.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*timelinegen.Queries, error) {
	pgTx, err := d.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return timelinegen.New(pgTx), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *timelinegen.Queries { return timelinegen.New(d.Reader()) }

// execAll runs the queries in order inside tx, for a rebuild that empties a projection and
// derives it again in SQL.
func (d *moduleDB) execAll(ctx context.Context, tx uow.Tx, what string, steps ...func(*timelinegen.Queries, context.Context) error) error {
	q, err := d.queries(tx)
	if err != nil {
		return err
	}
	return postgres.RunQueries(ctx, q, what, steps...)
}
