// SPDX-License-Identifier: AGPL-3.0-only

// Package postgres is the base of the persistence adapter on PostgreSQL and compatible services with
// pgx/v5 (ADR-0003): the pool, the transactions and their retries, and the helpers of the queries. The
// repositories of each module are in the sub-package of the module, internal/adapter/db/postgres/<module>,
// over the DB of this package and the queries that sqlc generates for the module (<module>gen); this
// package imports none of them (ADR-0153 §3).
//
// Transactions run at READ COMMITTED. Serialization failures (SQLSTATE 40001) and deadlocks
// (40P01) are retryable: UnitOfWork.Tx runs the transaction again.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mycroblog/my/internal/adapter/telemetry/dbtrace"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"

	// Links the tables of kernel, uow, and queue, which this package and every module use, into
	// the test databases of pkg/testkit/pgtest (package ddl).
	_ "github.com/mycroblog/my/schema/10-infrastructure"
)

// DB is a PostgreSQL connection pool. It implements uow.UnitOfWork.
type DB struct {
	pool *pgxpool.Pool
	// conns uses the pool with the bound on waiting for a connection; everything but the
	// statistics and the configuration goes through it.
	conns *connections
	opts  uow.Options
	// tx is the transaction that the reads of a copy made by in run in, nil for the pool.
	tx pgx.Tx
	// tracer records the spans of the transactions and, through the pool's QueryTracer, of
	// every query (SetTracer).
	tracer *tracerRef
}

var _ uow.UnitOfWork = (*DB)(nil)

// Open connects to the PostgreSQL database at url (the database_url setting for
// database = "postgres"), in any form pgx accepts: a postgres:// URL or a keyword/value string.
//
// Open does not apply the schema; Atlas does (ADR-0023).
func Open(ctx context.Context, url string, opts uow.Options) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// The error may quote the URL, which can hold a password; keep only its kind.
		return nil, errors.New("postgres: database_url is not a valid connection string")
	}
	ref := &tracerRef{}
	cfg.ConnConfig.Tracer = queryTracer{ref: ref}
	cfg.AfterConnect = scanTimestampsInUTC
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	return &DB{pool: pool, conns: &connections{pool: pool}, opts: opts, tracer: ref}, nil
}

// SetAcquireTimeout bounds how long a statement or a transaction waits for a connection of the
// pool (database_acquire_timeout); 0 means no bound. Call it before using the database. Once the
// bound passes, the wait fails with an error wrapping kernel.ErrDatabaseBusy. It does not bound
// what runs on the connection.
func (d *DB) SetAcquireTimeout(wait time.Duration) { d.conns.wait = wait }

// SetTracer records a span of every transaction attempt and every query on the pool, of the
// repositories and of the adapters that share it, on t (ADR-0055 §13).
func (d *DB) SetTracer(t kernel.Tracer) { d.tracer.set(t) }

// Close closes every connection of the pool.
func (d *DB) Close() { d.pool.Close() }

// Pool returns the connection pool, for what reads its configuration (stream/postgres connects its
// listener with the same settings).
func (d *DB) Pool() *pgxpool.Pool { return d.pool }

// Exec runs a statement outside a transaction, for the adapters that share the database's
// connections (stream/postgres sends its notifications with it).
func (d *DB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return d.conns.Exec(ctx, sql, args...)
}

// Ready reports whether the database is reachable and its schema is applied, for the
// readiness check. The schema counts as applied when every table of want exists in the
// connection's current schema (the caller passes the tables of the whole schema,
// schema.Tables(schema.Postgres)); Atlas applies the schema as a whole (ADR-0023), so a missing
// table means the schema was never applied or is out of date.
func (d *DB) Ready(ctx context.Context, want []string) error {
	rows, err := d.conns.Query(ctx,
		"SELECT table_name::text FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ANY($1)", want)
	if err != nil {
		return fmt.Errorf("postgres: ready: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("postgres: ready: %w", err)
	}
	present := map[string]bool{}
	for _, n := range names {
		present[n] = true
	}
	var missing []string
	for _, t := range want {
		if !present[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("postgres: schema not applied: missing tables %s", strings.Join(missing, ", "))
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
	sctx, span := dbtrace.StartTx(ctx, d.tracer.get(), dbtrace.SystemPostgreSQL)
	defer func() { dbtrace.End(span, err) }()
	pgTx, err := d.conns.BeginTx(sctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			// Roll back even when ctx has ended, so that the connection returns to the pool clean.
			_ = pgTx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	tx := &Tx{tx: pgTx, owner: d.pool, projectors: d.opts.Projectors}
	if err := fn(tx); err != nil {
		return err
	}
	if err := pgTx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	committed = true
	d.opts.Projectors.Committed(ctx, tx.events)
	return nil
}

// isRetryable reports serialization_failure (40001) and deadlock_detected (40P01).
func isRetryable(err error) bool {
	var e *pgconn.PgError
	return errors.As(err, &e) && (e.Code == "40001" || e.Code == "40P01")
}

// Tx is a PostgreSQL transaction. It implements uow.Tx.
type Tx struct {
	tx pgx.Tx
	// owner is the pool of the database the transaction runs in: a Tx works only with the
	// repositories of that database.
	owner      *pgxpool.Pool
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
var ErrForeignTx = errors.New("postgres: transaction does not belong to this database")

// txOf recovers the PostgreSQL transaction behind tx, for repositories of this adapter, when
// d's database began it.
func (d *DB) txOf(tx uow.Tx) (pgx.Tx, error) {
	t, ok := tx.(*Tx)
	if !ok {
		return nil, fmt.Errorf("%w: %T", ErrForeignTx, tx)
	}
	if t.owner != d.pool {
		return nil, fmt.Errorf("%w: it runs in another database", ErrForeignTx)
	}
	return t.tx, nil
}

// PgxTx returns the pgx transaction behind tx, for the adapters of other ports that keep
// their tables in this database (the database job queue, internal/adapter/queue/db).
// It fails with ErrForeignTx for a Tx that d's database did not begin.
func (d *DB) PgxTx(tx uow.Tx) (pgx.Tx, error) { return d.txOf(tx) }

// scanTimestampsInUTC makes a connection return timestamptz values in UTC, as the SQLite adapter
// returns them. pgx returns them in the process's local time zone otherwise, so a repository that
// did not convert a time itself returned it in another zone than one that did (#2508). The instant
// does not change.
func scanTimestampsInUTC(_ context.Context, conn *pgx.Conn) error {
	conn.TypeMap().RegisterType(&pgtype.Type{
		Name: "timestamptz", OID: pgtype.TimestamptzOID, Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
	})
	return nil
}
