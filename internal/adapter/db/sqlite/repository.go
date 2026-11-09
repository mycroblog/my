// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/adapter/telemetry/dbtrace"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// Reader returns what runs the reads outside transactions on the connection pool, with their
// spans, or the reads in the transaction of a copy that In made. The sub-packages of the modules
// bind their generated queries (package <module>gen) to it.
func (d *DB) Reader() dbtrace.DBTX {
	if d.tx != nil {
		return dbtrace.Wrap(d.tx, d.tracer, dbtrace.SystemSQLite)
	}
	checkSecondConnection(d.db)
	return dbtrace.Wrap(d.db, d.tracer, dbtrace.SystemSQLite)
}

// In returns a copy of d whose reads run in tx, for the repositories' read methods that take a
// transaction (their ...Tx variants): they run the method that reads outside transactions on a
// repository over the copy.
func (d *DB) In(tx uow.Tx) (*DB, error) {
	sqlTx, err := d.txOf(tx)
	if err != nil {
		return nil, err
	}
	return &DB{db: d.db, opts: d.opts, tx: sqlTx, tracer: d.tracer}, nil
}

// SQLDB returns the connection pool, for the statements that no generated query covers (such as
// a look at sqlite_schema).
func (d *DB) SQLDB() *sql.DB { return d.db }

// RunQueries runs the queries in order on q, for a rebuild that empties a projection and derives
// it again in SQL, and stops at the first error.
func RunQueries[Q any](ctx context.Context, q Q, what string, steps ...func(Q, context.Context) error) error {
	for _, step := range steps {
		if err := step(q, ctx); err != nil {
			return Wrap(err, what)
		}
	}
	return nil
}

// ToMillis returns t as Unix milliseconds, the form of every timestamp column (data-model §1.2).
func ToMillis(t time.Time) int64 { return t.UnixMilli() }

// FromMillis reads a timestamp column back as a time in UTC.
func FromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// ToNullMillis stores an optional time: NULL for nil.
func ToNullMillis(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

// FromNullMillis reads an optional timestamp column: nil for NULL.
func FromNullMillis(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := FromMillis(n.Int64)
	return &t
}

// ToNullString stores an optional reference, "" in the domain, as NULL.
func ToNullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// BoolToInt stores a boolean as the 0 or 1 of an INTEGER column.
func BoolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// Wrap prefixes err with the adapter and what failed, with the stack of the repository method that
// called it (kernel.WithStackSkip), so that a reported database error starts where it left the
// adapter rather than where it was reported; it returns nil for a nil err.
func Wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	return kernel.WithStackSkip(fmt.Errorf("sqlite: %s: %w", what, err), 1)
}

// NotFound maps sql.ErrNoRows to notFoundErr and wraps every other error with what, as Wrap does.
func NotFound(err error, notFoundErr error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, notFoundErr)
	}
	return kernel.WithStackSkip(fmt.Errorf("sqlite: %s: %w", what, err), 1)
}

// IsUniqueViolation reports whether err is a UNIQUE or PRIMARY KEY violation on the given
// columns, which SQLite names as "table.column, table.column" in the message.
func IsUniqueViolation(err error, columns string) bool {
	var e *msqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	switch e.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return strings.Contains(e.Error(), "constraint failed: "+columns)
	}
	return false
}

// ToNullTime stores a time whose zero value means none (the open side of a window) as NULL.
func ToNullTime(t time.Time) sql.NullInt64 {
	if t.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: ToMillis(t), Valid: true}
}

// FromNullTime reads a column of ToNullTime back: the zero time for NULL.
func FromNullTime(n sql.NullInt64) time.Time {
	if !n.Valid {
		return time.Time{}
	}
	return FromMillis(n.Int64)
}

// RunSteps runs the statements on the same argument in order and stops at the first error.
func RunSteps(ctx context.Context, arg string, what string, steps ...func(context.Context, string) error) error {
	for _, step := range steps {
		if err := step(ctx, arg); err != nil {
			return Wrap(err, what)
		}
	}
	return nil
}

// ToNullInt64 stores an optional integer: NULL for nil.
func ToNullInt64(n *int64) sql.NullInt64 {
	if n == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *n, Valid: true}
}

// FromNullInt64 reads an optional integer column: nil for NULL.
func FromNullInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}

// JSONList encodes a list argument as the JSON array that the queries read with json_each.
func JSONList(s []string) (string, error) {
	if s == nil {
		s = []string{}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", Wrap(err, "encode a list argument")
	}
	return string(b), nil
}

// JSONPairs encodes the pairs (a[i], b[i]) as the JSON array of {"a": ..., "b": ...} that the
// queries of many pairs read with json_each.
func JSONPairs(a, b []string) (string, error) {
	type pair struct {
		A string `json:"a"`
		B string `json:"b"`
	}
	pairs := make([]pair, len(a))
	for i := range a {
		pairs[i] = pair{A: a[i], B: b[i]}
	}
	out, err := json.Marshal(pairs)
	if err != nil {
		return "", Wrap(err, "encode a list of pairs")
	}
	return string(out), nil
}
