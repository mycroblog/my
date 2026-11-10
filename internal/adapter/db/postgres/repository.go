// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// DBTX runs the statements of the generated queries (package <module>gen): the pool, or a
// transaction.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Reader returns what runs the reads outside transactions on the pool, or the reads in the
// transaction of a copy that In made. The sub-packages of the modules bind their generated
// queries (package <module>gen) to it.
func (d *DB) Reader() DBTX {
	if d.tx != nil {
		return d.tx
	}
	return d.conns
}

// In returns a copy of d whose reads run in tx, for the repositories' read methods that take a
// transaction (their ...Tx variants): they run the method that reads outside transactions on a
// repository over the copy.
func (d *DB) In(tx uow.Tx) (*DB, error) {
	pgTx, err := d.txOf(tx)
	if err != nil {
		return nil, err
	}
	return &DB{pool: d.pool, conns: d.conns, opts: d.opts, tx: pgTx, tracer: d.tracer}, nil
}

// QueryRow runs a statement that returns one row on the pool, for the statements that no
// generated query covers (such as a look at information_schema).
func (d *DB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return d.conns.QueryRow(ctx, sql, args...)
}

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

// UTC returns t in UTC: timestamps read back from timestamptz are in the session's time zone, and
// the domain uses UTC.
func UTC(t time.Time) time.Time { return t.UTC() }

// UTCPtr is UTC for an optional time.
func UTCPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// ToNullString stores an optional reference, "" in the domain, as NULL.
func ToNullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// FromNullString reads an optional reference back: "" for NULL.
func FromNullString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ToInt32 converts a value of the logical int type (data-model §1.2), which is integer in PostgreSQL.
func ToInt32(n int, what string) (int32, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("postgres: %s %d is out of the integer range", what, n)
	}
	return int32(n), nil
}

// Wrap prefixes err with the adapter and what failed, with the stack of the repository method that
// called it (kernel.WithStackSkip), so that a reported database error starts where it left the
// adapter rather than where it was reported; it returns nil for a nil err.
func Wrap(err error, what string) error {
	if err == nil {
		return nil
	}
	return kernel.WithStackSkip(fmt.Errorf("postgres: %s: %w", what, err), 1)
}

// NotFound maps pgx.ErrNoRows to notFoundErr and wraps every other error with what, as Wrap does.
func NotFound(err error, notFoundErr error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, notFoundErr)
	}
	return kernel.WithStackSkip(fmt.Errorf("postgres: %s: %w", what, err), 1)
}

// IsUniqueViolation reports whether err is a unique_violation (23505) of the named index or constraint.
func IsUniqueViolation(err error, constraint string) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && e.Code == "23505" && e.ConstraintName == constraint
}

// ToNullTime stores a time whose zero value means none (the open side of a window) as NULL.
func ToNullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// FromNullTime reads a column of ToNullTime back: the zero time for NULL.
func FromNullTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

// RunSteps runs the statements on the same argument in order and stops at the first error.
func RunSteps[T any](ctx context.Context, arg T, what string, steps ...func(context.Context, T) error) error {
	for _, step := range steps {
		if err := step(ctx, arg); err != nil {
			return Wrap(err, what)
		}
	}
	return nil
}

// BatchLimit bounds the size of an erasure batch to the integer range of PostgreSQL.
func BatchLimit(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < 0:
		return 0
	}
	return int32(n)
}

// TextArray returns s as a text[] argument, never NULL.
func TextArray(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
