// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/adapter/db/sqlite/sqlitegen"
	"github.com/mycroblog/my/internal/adapter/telemetry/dbtrace"
	"github.com/mycroblog/my/internal/app/uow"
)

// queries returns the generated queries bound to the SQLite transaction behind tx, which must
// be a transaction of d's database.
func (d *DB) queries(tx uow.Tx) (*sqlitegen.Queries, error) {
	db, err := d.QueryTx(tx)
	if err != nil {
		return nil, err
	}
	return sqlitegen.New(db), nil
}

// reader returns the generated queries on the connection pool, for reads outside transactions,
// or on the transaction of a copy that in made.
func (d *DB) reader() *sqlitegen.Queries {
	if d.tx != nil {
		return sqlitegen.New(dbtrace.Wrap(d.tx, d.tracer, dbtrace.SystemSQLite))
	}
	return sqlitegen.New(dbtrace.Wrap(d.db, d.tracer, dbtrace.SystemSQLite))
}

// in returns a copy of d whose reads run in tx, for the repositories' read methods that take a
// transaction (their ...Tx variants): they run the method that reads outside transactions on a
// repository over the copy.
func (d *DB) in(tx uow.Tx) (*DB, error) {
	sqlTx, err := d.txOf(tx)
	if err != nil {
		return nil, err
	}
	return &DB{db: d.db, opts: d.opts, tx: sqlTx, tracer: d.tracer}, nil
}

// Timestamps are stored as Unix milliseconds (data-model §1.2).
func toMillis(t time.Time) int64 { return t.UnixMilli() }

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

func toNullMillis(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

func fromNullMillis(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := fromMillis(n.Int64)
	return &t
}

// Optional references are NULL in the database and "" in the domain.
func toNullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("sqlite: %s: %w", what, err)
}

// notFound maps sql.ErrNoRows to notFoundErr and wraps every other error with what.
func notFound(err error, notFoundErr error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, notFoundErr)
	}
	return fmt.Errorf("sqlite: %s: %w", what, err)
}

// isUniqueViolation reports whether err is a UNIQUE or PRIMARY KEY violation on the given
// columns, which SQLite names as "table.column, table.column" in the message.
func isUniqueViolation(err error, columns string) bool {
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
