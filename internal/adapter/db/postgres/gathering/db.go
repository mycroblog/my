// SPDX-License-Identifier: AGPL-3.0-only

// Package gathering implements the ports of the gathering module (the events of the local users)
// on PostgreSQL: its repositories run the queries of package gatheringgen in the database and the
// transactions of package postgres.
package gathering

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/db/postgres"
	"github.com/mycroblog/my/internal/adapter/db/postgres/gatheringgen"
	appgathering "github.com/mycroblog/my/internal/app/gathering"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/gathering"

	// Links the module's tables, and those of the modules it depends on, into the test
	// databases (package ddl).
	_ "github.com/mycroblog/my/schema/19-gathering"
)

// moduleDB is the database with the queries of this module.
type moduleDB struct{ *postgres.DB }

// queries returns the module's queries bound to the transaction behind tx, which must be a
// transaction of d's database.
func (d *moduleDB) queries(tx uow.Tx) (*gatheringgen.Queries, error) {
	pgTx, err := d.PgxTx(tx)
	if err != nil {
		return nil, err
	}
	return gatheringgen.New(pgTx), nil
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
	return postgres.RunQueries(ctx, q, what, steps...)
}

// The conversions between the columns of this dialect and the domain: a timestamp is a
// timestamptz read back in UTC, an int is an integer, and an optional reference is NULL for "".
// The values the server writes fit an integer (the capacity, the positions, the page sizes).

func toTime(t time.Time) time.Time   { return t }
func fromTime(t time.Time) time.Time { return postgres.UTC(t) }
func toInt(n int) int32              { return int32(n) } //nolint:gosec // G115: bounded by the domain
func fromInt(n int32) int            { return int(n) }
func fromBool(b bool) bool           { return b }
func toRef(s string) *string         { return postgres.ToNullString(s) }
func fromRef(s *string) string       { return postgres.FromNullString(s) }

// listOf returns ids as the argument of a query that takes a list.
func listOf(ids []string) ([]string, error) { return ids, nil }

var (
	wrap     = postgres.Wrap
	notFound = postgres.NotFound
)

// cancellationOf returns the cancellation of a row of GetGathering, nil for none.
func cancellationOf(row gatheringgen.GetGatheringRow) *appgathering.Cancellation {
	if row.CancelReason == nil || row.CancelledAt == nil {
		return nil
	}
	return &appgathering.Cancellation{
		GatheringID: row.ID, Reason: gathering.CancelReason(*row.CancelReason), CancelledByAccountID: fromRef(row.CancelledByAccountID),
		Note: fromRef(row.CancelNote), CancelledAt: fromTime(*row.CancelledAt),
	}
}

// LockGathering implements appgathering.Repository with a transaction-scoped advisory lock on the
// event, so that its commands serialize under READ COMMITTED.
func (r *Repository) LockGathering(ctx context.Context, tx uow.Tx, id string) error {
	q, err := r.db.queries(tx)
	if err != nil {
		return err
	}
	return wrap(q.LockGathering(ctx, "gathering:"+id), "lock gathering")
}
