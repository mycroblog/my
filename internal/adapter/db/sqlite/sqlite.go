// SPDX-License-Identifier: AGPL-3.0-only

// Package sqlite is the base of the persistence adapter on a local SQLite file with modernc.org/sqlite,
// which needs no cgo (ADR-0003): the connections, the transactions and their retries, and the helpers
// of the queries. The repositories of each module are in the sub-package of the module,
// internal/adapter/db/sqlite/<module>, over the DB of this package and the queries that sqlc generates
// for the module (<module>gen); this package imports none of them (ADR-0153 §3).
//
// Every connection uses WAL journaling and a busy timeout, and every transaction starts with
// BEGIN IMMEDIATE, so that writers queue on the database lock instead of failing when a read
// transaction upgrades to a write. UnitOfWork.Read is the exception: it begins deferred on a
// connection of a small pool of its own that is query-only from the start (ReadPoolMax), so it
// cannot write, takes no lock and, in WAL, waits for no writer. The pools are bounded and keep
// every connection they open (pool_max_conns).
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mycroblog/my/internal/adapter/telemetry/dbtrace"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"

	// Links the tables of kernel, uow, and queue, which this package and every module use, into
	// the test databases of pkg/testkit/sqlitetest (package ddl).
	_ "github.com/mycroblog/my/schema/10-infrastructure"
)

// DefaultBusyTimeoutMillis is how long a connection waits for the database lock before a
// statement fails with SQLITE_BUSY, unless the DSN sets busy_timeout.
const DefaultBusyTimeoutMillis = 5000

// DB is a SQLite database. It implements uow.UnitOfWork.
type DB struct {
	db *sql.DB
	// reads is the pool of the read transactions (Read), whose connections are query-only.
	reads *sql.DB
	opts  uow.Options
	// tx is the transaction that the reads of a copy made by in run in, nil for the pool.
	tx *sql.Tx
	// tracer records the spans of the transactions and the queries (SetTracer).
	tracer kernel.Tracer
}

var _ uow.UnitOfWork = (*DB)(nil)

// Open opens the SQLite database named by dsn, which is a file path or a "file:" URI with
// query parameters (the database_url setting for database = "sqlite"). The file is created if it does
// not exist. The pool is set by two parameters of dsn that the driver never sees: pool_max_conns,
// how many connections may be open (PoolMaxDefault by default), and pool_max_idle_conns, how many at most are kept when idle
// (the maximum by default). Open sets the parameters the adapter relies on: _txlock=immediate, and the pragmas
// journal_mode(wal), synchronous(normal), foreign_keys(1), and busy_timeout unless dsn sets
// them. In-memory databases are rejected: each connection would see its own database.
//
// Each connection keeps the statements it prepared (stmt_cache.go). Open does not apply the
// schema; Atlas does (ADR-0023).
func Open(ctx context.Context, dsn string, opts uow.Options) (*DB, error) {
	name, err := normalizeDSN(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := poolOf(dsn)
	if err != nil {
		return nil, err
	}
	db, err := openPool(ctx, name, false, pool.max, pool.idle)
	if err != nil {
		return nil, err
	}
	readMax := ReadPoolMax(pool.max)
	reads, err := openPool(ctx, name, true, readMax, min(readMax, pool.idle))
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{db: db, reads: reads, opts: opts, tracer: kernel.NopTracer{}}, nil
}

// ReadPoolMax is how many connections the read transactions may have open beside the pool of
// pool_max_conns: a quarter of it, and at least two. The read transactions are few and short
// (the job queue's polls, a few listings).
func ReadPoolMax(poolMax int) int { return max(2, poolMax/4) }

// openPool opens a pool of name of at most maxOpen connections, keeping maxIdle when idle.
func openPool(ctx context.Context, name string, readOnly bool, maxOpen, maxIdle int) (*sql.DB, error) {
	connector, err := newCachingConnector(name, readOnly)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	// Nothing closes a kept connection by time: the defaults of database/sql for both are "never", and
	// they are set so that no later change of a default can. A quiet instance answers its next request
	// on a connection it kept, which has parsed the schema, instead of opening one.
	db.SetConnMaxIdleTime(0)
	db.SetConnMaxLifetime(0)
	return db, nil
}

// SetTracer records a span of every transaction attempt and every query of the repositories
// and of the database job queue on t (ADR-0055 §13). Call it before the first transaction.
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
	if _, err := takePool(q); err != nil {
		return "", err
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

// Parameters of the SQLite database_url that set the pool, named after those of PostgreSQL's.
const (
	// ParamPoolMaxConns is how many connections may be open (PostgreSQL's pool_max_conns).
	ParamPoolMaxConns = "pool_max_conns"
	// ParamPoolMaxIdleConns is the most idle connections kept (database/sql's MaxIdleConns). It is a
	// ceiling, not the floor of pgxpool's pool_min_conns: connections are not opened ahead.
	ParamPoolMaxIdleConns = "pool_max_idle_conns"
)

// PoolMaxDefault is how many connections may be open unless database_url sets pool_max_conns: the
// greater of 4 and the number of processors, as the default of PostgreSQL's pool. A pool that keeps its
// connections gave 2 to 6 times the throughput of database/sql's default of two idle and any number
// open, whose extra connections are opened and closed per query and each parse the schema (#3181).
func PoolMaxDefault() int { return max(4, runtime.GOMAXPROCS(0)) }

type poolSize struct{ max, idle int }

// poolOf reads the pool of dsn: pool_max_conns, by default PoolMaxDefault, and as many idle.
func poolOf(dsn string) (poolSize, error) {
	_, rawQuery, _ := strings.Cut(strings.TrimPrefix(dsn, "file:"), "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return poolSize{}, fmt.Errorf("sqlite: database_url: %w", err)
	}
	return takePool(q)
}

// takePool removes the pool parameters from q and returns the pool they set.
func takePool(q url.Values) (poolSize, error) {
	p := poolSize{max: PoolMaxDefault()}
	p.idle = -1
	for _, name := range []string{ParamPoolMaxConns, ParamPoolMaxIdleConns} {
		v := q.Get(name)
		if !q.Has(name) {
			continue
		}
		q.Del(name)
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n == 0 && name == ParamPoolMaxConns {
			want := "a whole number of 1 or more"
			if name == ParamPoolMaxIdleConns {
				want = "a whole number of 0 or more"
			}
			return poolSize{}, fmt.Errorf("sqlite: database_url: %s must be %s, not %q", name, want, v)
		}
		if name == ParamPoolMaxConns {
			p.max = n
		} else {
			p.idle = n
		}
	}
	if p.idle < 0 {
		p.idle = p.max
	}
	if p.idle > p.max {
		return poolSize{}, fmt.Errorf("sqlite: database_url: %s (%d) must not be above %s (%d)", ParamPoolMaxIdleConns, p.idle, ParamPoolMaxConns, p.max)
	}
	return p, nil
}

// PoolMax is how many connections may be open.
func (d *DB) PoolMax() int { return d.db.Stats().MaxOpenConnections }

// Close closes the database.
func (d *DB) Close() error { return errors.Join(d.db.Close(), d.reads.Close()) }

// Ready reports whether the database is reachable and its schema is applied, for the
// readiness check. The schema counts as applied when every table of tables exists (the caller
// passes the tables of the whole schema, schema.Tables(schema.SQLite)); Atlas applies the schema
// as a whole (ADR-0023), so a missing table means the schema was never applied or is out of date.
func (d *DB) Ready(ctx context.Context, tables []string) error {
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
	for _, t := range tables {
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

// Read implements uow.UnitOfWork. The transaction begins deferred (the driver ignores _txlock for
// a read-only transaction) on a connection of the read pool, which PRAGMA query_only keeps from
// writing for its life, because the driver begins a read-only transaction but does not stop it
// from writing. The pragma is set when the connection opens and never again, so the connection
// keeps its prepared statements (#3487).
func (d *DB) Read(ctx context.Context, fn func(tx uow.Tx) error) (err error) {
	sctx, span := dbtrace.StartTx(ctx, d.tracer, dbtrace.SystemSQLite)
	defer func() { dbtrace.End(span, err) }()
	checkSecondConnection(d.db)
	sqlTx, err := d.reads.BeginTx(sctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("sqlite: begin: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	tx := &Tx{tx: sqlTx, owner: d.db, db: dbtrace.Wrap(sqlTx, d.tracer, dbtrace.SystemSQLite), projectors: d.opts.Projectors, readOnly: true}
	return holdingTransaction(d.db, func() error { return fn(tx) })
}

// trial runs fn in a transaction that it always rolls back, for a verification of a projection
// that writes only to temporary tables (ADR-0172 §3). It begins deferred on a connection of the
// write pool (the driver ignores _txlock for a read-only transaction and does not stop it from
// writing), so it takes no lock of the main database that a writer waits for, and in WAL it
// sees one state of the database from its first read.
func (d *DB) trial(ctx context.Context, fn func(tx uow.Tx) error) (err error) {
	sctx, span := dbtrace.StartTx(ctx, d.tracer, dbtrace.SystemSQLite)
	defer func() { dbtrace.End(span, err) }()
	checkSecondConnection(d.db)
	sqlTx, err := d.db.BeginTx(sctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("sqlite: begin: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	tx := &Tx{tx: sqlTx, owner: d.db, db: dbtrace.Wrap(sqlTx, d.tracer, dbtrace.SystemSQLite), projectors: d.opts.Projectors}
	return holdingTransaction(d.db, func() error { return fn(tx) })
}

func (d *DB) attempt(ctx context.Context, fn func(tx uow.Tx) error) (err error) {
	sctx, span := dbtrace.StartTx(ctx, d.tracer, dbtrace.SystemSQLite)
	defer func() { dbtrace.End(span, err) }()
	checkSecondConnection(d.db)
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
	if err := holdingTransaction(d.db, func() error { return fn(tx) }); err != nil {
		return err
	}
	// A failed COMMIT (SQLITE_BUSY under the rollback journal) leaves SQLite inside the
	// transaction, and database/sql marks sqlTx done, so the deferred Rollback is a no-op. The
	// driver rolls the connection back itself before returning the error (modernc.org/sqlite
	// tx.Commit), so the connection returns to the pool clean, and Tx retries the whole of fn
	// in a new transaction (TestRetriesACommitThatFailsWithBusy).
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
	// readOnly marks a transaction of UnitOfWork.Read.
	readOnly bool
}

var _ uow.Tx = (*Tx)(nil)

// Emit implements uow.Tx.
func (t *Tx) Emit(ctx context.Context, e uow.Event) error {
	if t.readOnly {
		return uow.ErrReadOnly
	}
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
