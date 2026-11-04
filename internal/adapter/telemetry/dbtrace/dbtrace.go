// SPDX-License-Identifier: AGPL-3.0-only

// Package dbtrace holds the database spans of ADR-0055 §13 that the database adapters share:
// one span per attempt of UnitOfWork.Tx (TxSpan) and one per query (QuerySpan), named after the
// sqlc query. A span carries the database system and the query's name, never the SQL text or
// its parameters (§4).
package dbtrace

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Attribute keys of the database spans, from the OpenTelemetry semantic conventions.
const (
	AttrSystem    = "db.system.name"
	AttrOperation = "db.operation.name"
)

// The database systems, the values of db.system.name.
const (
	SystemSQLite     = "sqlite"
	SystemPostgreSQL = "postgresql"
)

var (
	// TxSpan is the span of one attempt of UnitOfWork.Tx: a retried transaction has a span per
	// attempt.
	TxSpan = kernel.SpanSpec{Name: "my.db.transaction", Kind: kernel.SpanInternal, AttrKeys: []string{AttrSystem}}
	// QuerySpan is the span of one query, named after its operation (Operation).
	QuerySpan = kernel.SpanSpec{Name: "my.db.query", Kind: kernel.SpanClient, AttrKeys: []string{AttrSystem, AttrOperation}}
)

// operationOther is the operation of a query whose name cannot be told.
const operationOther = "_OTHER"

// maxKeyword bounds the length of an SQL keyword taken as an operation.
const maxKeyword = 16

// Operation returns the db.operation.name of query: the name of an sqlc query (the "-- name:"
// comment that sqlc puts first), or else the first SQL keyword upper-cased (BEGIN, COMMIT,
// SELECT), so that the value comes from a finite set fixed in code.
func Operation(query string) string {
	q := strings.TrimSpace(query)
	if rest, ok := strings.CutPrefix(q, "-- name:"); ok {
		name, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
		if name != "" && identifier(name) {
			return name
		}
		return operationOther
	}
	end := strings.IndexFunc(q, func(r rune) bool { return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') })
	if end < 0 {
		end = len(q)
	}
	if end == 0 || end > maxKeyword {
		return operationOther
	}
	return strings.ToUpper(q[:end])
}

func identifier(s string) bool {
	for _, c := range []byte(s) {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// StartQuery starts the span of query on system.
func StartQuery(ctx context.Context, t kernel.Tracer, system, query string) (context.Context, kernel.Span) {
	op := Operation(query)
	return t.Start(ctx, QuerySpan.Named(op), kernel.NewAttrs(kernel.StringAttr(AttrSystem, system), kernel.StringAttr(AttrOperation, op)))
}

// StartTx starts the span of an attempt of a transaction on system.
func StartTx(ctx context.Context, t kernel.Tracer, system string) (context.Context, kernel.Span) {
	return t.Start(ctx, TxSpan, kernel.NewAttrs(kernel.StringAttr(AttrSystem, system)))
}

// End ends a database span with the error type of err; sql.ErrNoRows is an answer, not a
// failure.
func End(s kernel.Span, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	s.End(kernel.ErrorType(err))
}

// DBTX is what the sqlc code of database/sql runs its queries on.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Wrap returns db with a span of every query on t, or db itself when t records nothing. A
// span of QueryContext and QueryRowContext ends when the query has run, before its rows are
// read.
func Wrap(db DBTX, t kernel.Tracer, system string) DBTX {
	if kernel.IsNopTracer(t) {
		return db
	}
	return traced{db: db, t: t, system: system}
}

type traced struct {
	db     DBTX
	t      kernel.Tracer
	system string
}

func (d traced) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, s := StartQuery(ctx, d.t, d.system, query)
	r, err := d.db.ExecContext(ctx, query, args...)
	End(s, err)
	return r, err
}

func (d traced) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return d.db.PrepareContext(ctx, query)
}

func (d traced) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, s := StartQuery(ctx, d.t, d.system, query)
	r, err := d.db.QueryContext(ctx, query, args...)
	End(s, err)
	return r, err
}

func (d traced) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	ctx, s := StartQuery(ctx, d.t, d.system, query)
	r := d.db.QueryRowContext(ctx, query, args...)
	End(s, r.Err())
	return r
}
