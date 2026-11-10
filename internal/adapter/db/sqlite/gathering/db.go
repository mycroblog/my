// SPDX-License-Identifier: AGPL-3.0-only

// Package gathering implements the ports of the gathering module (the events of the local users)
// on SQLite: its repositories run the queries of package gatheringgen in the database and the
// transactions of package sqlite.
package gathering

import (
	"context"
	"database/sql"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/sqlite"
	"github.com/mycroblog/my/internal/adapter/db/sqlite/gatheringgen"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"

	// Links the module's tables, and those of the modules it depends on, into the test
	// databases (package ddl).
	_ "github.com/mycroblog/my/schema/19-gathering"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *sqlite.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*gatheringgen.Queries, error) {
	db, err := d.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return gatheringgen.New(db), nil
}

// reader returns the module's queries for reads outside transactions.
func (d *moduleDB) reader() *gatheringgen.Queries { return gatheringgen.New(d.Reader()) }

// execAll runs the queries in order inside tx, for a rebuild that empties a projection and
// derives it again in SQL.
func (d *moduleDB) execAll(ctx context.Context, tx uow.Tx, what string, steps ...func(*gatheringgen.Queries, context.Context) error) error {
	q, err := d.queries(tx)
	if err != nil {
		return err
	}
	return sqlite.RunQueries(ctx, q, what, steps...)
}

// The conversions between the columns of this dialect and the domain: a timestamp is Unix
// milliseconds, an int is an INTEGER, a bool is 0 or 1, and an optional reference is NULL for "".

func toTime(t time.Time) int64      { return sqlite.ToMillis(t) }
func fromTime(ms int64) time.Time   { return sqlite.FromMillis(ms) }
func toInt(n int) int64             { return int64(n) }
func fromInt(n int64) int           { return int(n) }
func fromBool(n int64) bool         { return n != 0 }
func toRef(s string) sql.NullString { return sqlite.ToNullString(s) }
func fromRef(s sql.NullString) string {
	if !s.Valid {
		return ""
	}
	return s.String
}

// listOf returns ids as the argument of a query that takes a list.
func listOf(ids []string) (string, error) { return sqlite.JSONList(ids) }

var (
	wrap     = sqlite.Wrap
	notFound = sqlite.NotFound
)

// cancellationOf returns the cancellation of a row of GetGathering, nil for none.
func cancellationOf(row gatheringgen.GetGatheringRow) *appgathering.Cancellation {
	if !row.CancelReason.Valid {
		return nil
	}
	return &appgathering.Cancellation{
		GatheringID: row.ID, Reason: gathering.CancelReason(row.CancelReason.String), CancelledByAccountID: fromRef(row.CancelledByAccountID),
		Note: fromRef(row.CancelNote), CancelledAt: fromTime(row.CancelledAt.Int64),
	}
}

// LockGathering implements appgathering.Repository. SQLite runs one write transaction at a time,
// so there is nothing to lock.
func (r *Repository) LockGathering(context.Context, uow.Tx, string) error { return nil }
