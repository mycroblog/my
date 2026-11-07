// SPDX-License-Identifier: AGPL-3.0-only

// Package moderation implements the ports of the moderation module on PostgreSQL: its repositories run the
// queries of package moderationgen in the database and the transactions of package postgres.
package moderation

import (
	"context"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/moderationgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *postgres.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*moderationgen.Queries, error) {
	pgTx, err := d.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return moderationgen.New(pgTx), nil
}

// reader returns the module's queries for reads outside transactions, or in the transaction of a
// copy that in made.
func (d *moduleDB) reader() *moderationgen.Queries { return moderationgen.New(d.Reader()) }

// in returns a copy of d whose reads run in tx, for the repositories' read methods that take a
// transaction (their ...Tx variants).
func (d *moduleDB) in(tx uow.Tx) (*moduleDB, error) {
	db, err := d.In(tx)
	if err != nil {
		return nil, err
	}
	return &moduleDB{db}, nil
}

// execAll runs the queries in order inside tx, for a rebuild that empties a projection and
// derives it again in SQL.
func (d *moduleDB) execAll(ctx context.Context, tx uow.Tx, what string, steps ...func(*moderationgen.Queries, context.Context) error) error {
	q, err := d.queries(tx)
	if err != nil {
		return err
	}
	return postgres.RunQueries(ctx, q, what, steps...)
}
