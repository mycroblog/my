// SPDX-License-Identifier: AGPL-3.0-only

// Package sqlite implements the persistence ports on a local SQLite file with modernc.org/sqlite,
// which needs no cgo (ADR-0003).
//
// Every connection uses WAL journaling and a busy timeout, and every transaction starts with
// BEGIN IMMEDIATE, so that writers queue on the database lock instead of failing when a read
// transaction upgrades to a write.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/adapter/telemetry/dbtrace"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/schema"
)

// DefaultBusyTimeoutMillis is how long a connection waits for the database lock before a
// statement fails with SQLITE_BUSY, unless the DSN sets busy_timeout.
const DefaultBusyTimeoutMillis = 5000

// DB is a SQLite database. It implements uow.UnitOfWork.
type DB struct {
	db   *sql.DB
	opts uow.Options
	// tx is the transaction that the reads of a copy made by in run in, nil for the pool.
	tx *sql.Tx
	// tracer records the spans of the transactions and the queries (SetTracer).
	tracer kernel.Tracer
}

var _ uow.UnitOfWork = (*DB)(nil)

// Open opens the SQLite database named by dsn, which is a file path or a "file:" URI with
// query parameters (the database_url setting for db = "sqlite"). The file is created if it does
// not exist. Open sets the parameters the adapter relies on: _txlock=immediate, and the pragmas
// journal_mode(wal), synchronous(normal), foreign_keys(1), and busy_timeout unless dsn sets
// them. In-memory databases are rejected: each connection would see its own database.
//
// Open does not apply the schema; Atlas does (ADR-0023).
func Open(ctx context.Context, dsn string, opts uow.Options) (*DB, error) {
	name, err := normalizeDSN(dsn)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", name)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	return &DB{db: db, opts: opts, tracer: kernel.NopTracer{}}, nil
}

// SetTracer records a span of every transaction attempt and every query of the repositories
// and of the database job queue on t (ADR-0063 §1). Call it before the first transaction.
func (d *DB) SetTracer(t kernel.Tracer) { d.tracer = kernel.TracerOrNop(t) }

// normalizeDSN turns a path or file: URI into a file: URI with the required parameters.
func normalizeDSN(dsn string) (string, error) {
	path, rawQuery, _ := strings.Cut(strings.TrimPrefix(dsn, "file:"), "?")
	if path == "" {
		return "", errors.New("sqlite: database_url names no file")
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", fmt.Errorf("sqlite: database_url: %w", err)
	}
	if path == ":memory:" || q.Get("mode") == "memory" {
		return "", errors.New("sqlite: in-memory databases are not supported; use a file")
	}
	set := map[string]bool{}
	for _, p := range q["_pragma"] {
		name, _, _ := strings.Cut(p, "(")
		set[strings.ToLower(strings.TrimSpace(name))] = true
	}
	// busy_timeout first: the driver applies pragmas in order, and later ones may wait on the lock.
	for _, p := range []struct{ name, value string }{
		{"busy_timeout", fmt.Sprint(DefaultBusyTimeoutMillis)},
		{"journal_mode", "wal"},
		{"synchronous", "normal"},
		{"foreign_keys", "1"},
	} {
		if !set[p.name] {
			q.Add("_pragma", p.name+"("+p.value+")")
		}
	}
	q.Set("_txlock", "immediate")
	return "file:" + path + "?" + q.Encode(), nil
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

// Ready reports whether the database is reachable and its schema is applied, for the
// readiness check. The schema counts as applied when every table that schema/sqlite
// declares exists; Atlas applies the schema as a whole (ADR-0023), so a missing table means the
// schema was never applied or is out of date.
func (d *DB) Ready(ctx context.Context) error {
	rows, err := d.db.QueryContext(ctx, "SELECT name FROM sqlite_schema WHERE type = 'table'")
	if err != nil {
		return fmt.Errorf("sqlite: ready: %w", err)
	}
	defer func() { _ = rows.Close() }()
	present := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("sqlite: ready: %w", err)
		}
		present[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: ready: %w", err)
	}
	var missing []string
	for _, t := range schema.Tables(schema.SQLite) {
		if !present[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("sqlite: schema not applied: missing tables %s", strings.Join(missing, ", "))
	}
	return nil
}

// Tx implements uow.UnitOfWork.
func (d *DB) Tx(ctx context.Context, fn func(tx uow.Tx) error) error {
	return uow.Retry(ctx, d.opts.Retry, func() error {
		err := d.attempt(ctx, fn)
		if isRetryable(err) {
			return uow.MarkRetryable(err)
		}
		return err
	})
}

func (d *DB) attempt(ctx context.Context, fn func(tx uow.Tx) error) (err error) {
	sctx, span := dbtrace.StartTx(ctx, d.tracer, dbtrace.SystemSQLite)
	defer func() { dbtrace.End(span, err) }()
	sqlTx, err := d.db.BeginTx(sctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = sqlTx.Rollback()
		}
	}()
	tx := &Tx{tx: sqlTx, owner: d.db, db: dbtrace.Wrap(sqlTx, d.tracer, dbtrace.SystemSQLite), projectors: d.opts.Projectors}
	if err := fn(tx); err != nil {
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit: %w", err)
	}
	committed = true
	d.opts.Projectors.Committed(ctx, tx.events)
	return nil
}

// isRetryable reports whether err is a lock timeout that a new transaction may get past.
func isRetryable(err error) bool {
	var e *msqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	switch e.Code() & 0xff { // primary result code
	case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED:
		return true
	}
	return false
}

// Tx is a SQLite transaction. It implements uow.Tx.
type Tx struct {
	tx *sql.Tx
	// owner is the database the transaction runs in: a Tx works only with the repositories of
	// that database.
	owner *sql.DB
	// db runs the queries of tx, with their spans.
	db         dbtrace.DBTX
	projectors *uow.Projectors
	// events are the events emitted so far, for the commit hooks.
	events []uow.Event
}

var _ uow.Tx = (*Tx)(nil)

// Emit implements uow.Tx.
func (t *Tx) Emit(ctx context.Context, e uow.Event) error {
	t.events = append(t.events, e)
	return t.projectors.Project(ctx, t, e)
}

// ErrForeignTx is returned when a repository is given a transaction that its database did not
// begin: one of another adapter, or of another database of this adapter.
var ErrForeignTx = errors.New("sqlite: transaction does not belong to this database")

// ownTx recovers the transaction behind tx when d's database began it.
func (d *DB) ownTx(tx uow.Tx) (*Tx, error) {
	t, ok := tx.(*Tx)
	if !ok {
		return nil, fmt.Errorf("%w: %T", ErrForeignTx, tx)
	}
	if t.owner != d.db {
		return nil, fmt.Errorf("%w: it runs in another database", ErrForeignTx)
	}
	return t, nil
}

// txOf recovers the SQLite transaction behind tx, for repositories of this adapter.
func (d *DB) txOf(tx uow.Tx) (*sql.Tx, error) {
	t, err := d.ownTx(tx)
	if err != nil {
		return nil, err
	}
	return t.tx, nil
}

// SQLTx returns the database/sql transaction behind tx, for the adapters of other ports that
// keep their tables in this database (the database job queue, internal/adapter/queue/db).
// It fails with ErrForeignTx for a Tx that d's database did not begin.
func (d *DB) SQLTx(tx uow.Tx) (*sql.Tx, error) { return d.txOf(tx) }

// QueryTx returns what runs the queries of tx with their spans (SetTracer), for the sqlc code
// of the adapters of other ports that keep their tables in this database. It fails with
// ErrForeignTx for a Tx that d's database did not begin.
func (d *DB) QueryTx(tx uow.Tx) (dbtrace.DBTX, error) {
	t, err := d.ownTx(tx)
	if err != nil {
		return nil, err
	}
	return t.db, nil
}
