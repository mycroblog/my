// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mycroblog/my/internal/adapter/db/postgres/postgresgen"
	"github.com/mycroblog/my/internal/app/uow"
)

// queries returns the generated queries bound to the PostgreSQL transaction behind tx, which
// must be a transaction of d's database.
func (d *DB) queries(tx uow.Tx) (*postgresgen.Queries, error) {
	pgTx, err := d.txOf(tx)
	if err != nil {
		return nil, err
	}
	return postgresgen.New(pgTx), nil
}

// reader returns the generated queries on the pool, for reads outside transactions, or on the
// transaction of a copy that in made.
func (d *DB) reader() *postgresgen.Queries {
	if d.tx != nil {
		return postgresgen.New(d.tx)
	}
	return postgresgen.New(d.pool)
}

// in returns a copy of d whose reads run in tx, for the repositories' read methods that take a
// transaction (their ...Tx variants): they run the method that reads outside transactions on a
// repository over the copy.
func (d *DB) in(tx uow.Tx) (*DB, error) {
	pgTx, err := d.txOf(tx)
	if err != nil {
		return nil, err
	}
	return &DB{pool: d.pool, opts: d.opts, tx: pgTx, tracer: d.tracer}, nil
}

// Timestamps read back from timestamptz are in the session's time zone; the domain uses UTC.
func utc(t time.Time) time.Time { return t.UTC() }

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// Optional references are NULL in the database and "" in the domain.
func toNullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fromNullString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// toInt32 converts a value of the logical int type (data-model §1.2), which is integer in PostgreSQL.
func toInt32(n int, what string) (int32, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("postgres: %s %d is out of the integer range", what, n)
	}
	return int32(n), nil
}

func wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("postgres: %s: %w", what, err)
}

// notFound maps pgx.ErrNoRows to notFoundErr and wraps every other error with what.
func notFound(err error, notFoundErr error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, notFoundErr)
	}
	return fmt.Errorf("postgres: %s: %w", what, err)
}

// isUniqueViolation reports whether err is a unique_violation (23505) of the named index or constraint.
func isUniqueViolation(err error, constraint string) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && e.Code == "23505" && e.ConstraintName == constraint
}
