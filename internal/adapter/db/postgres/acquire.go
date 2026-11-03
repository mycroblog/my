// SPDX-License-Identifier: AGPL-3.0-only

package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mycroblog/my/internal/app/kernel"
)

// connections runs statements and transactions on connections of the pool, bounding only the wait
// for a connection: pgx has no such bound, and the context it takes would also end the statement.
// The statement and the transaction run under the caller's context, however long they take.
type connections struct {
	pool *pgxpool.Pool
	// wait is the bound, set once before the pool is used; 0 means none.
	wait time.Duration
}

// acquire returns a connection of the pool. It fails with an error wrapping kernel.ErrDatabaseBusy
// when none became free within the bound, and notes that on ctx.
func (c *connections) acquire(ctx context.Context) (*pgxpool.Conn, error) {
	if c.wait <= 0 {
		return c.pool.Acquire(ctx)
	}
	waiting, cancel := context.WithTimeoutCause(ctx, c.wait, kernel.ErrDatabaseBusy)
	defer cancel()
	conn, err := c.pool.Acquire(waiting)
	if err == nil {
		return conn, nil
	}
	// The caller's own deadline or cancellation is not a busy database.
	if ctx.Err() == nil && errors.Is(context.Cause(waiting), kernel.ErrDatabaseBusy) {
		kernel.NoteDatabaseBusy(ctx)
		return nil, fmt.Errorf("postgres: waited %s: %w", c.wait, kernel.ErrDatabaseBusy)
	}
	return nil, err
}

// Exec runs a statement on a connection of the pool.
func (c *connections) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	conn, err := c.acquire(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer conn.Release()
	return conn.Exec(ctx, sql, args...)
}

// Query runs a query on a connection of the pool, which the rows give back once they end.
func (c *connections) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	conn, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		conn.Release()
		return nil, err
	}
	return &releasedRows{Rows: rows, conn: conn}, nil
}

// QueryRow runs a query for one row on a connection of the pool, given back by Scan.
func (c *connections) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	conn, err := c.acquire(ctx)
	if err != nil {
		return failedRow{err: err}
	}
	return releasedRow{row: conn.QueryRow(ctx, sql, args...), conn: conn}
}

// BeginTx starts a transaction on a connection of the pool, given back when it ends.
func (c *connections) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	conn, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := conn.BeginTx(ctx, opts)
	if err != nil {
		conn.Release()
		return nil, err
	}
	return &releasedTx{Tx: tx, conn: conn}, nil
}

// releasedRows gives its connection back once the rows end, as the rows of the pool do.
type releasedRows struct {
	pgx.Rows
	conn *pgxpool.Conn
	once sync.Once
}

func (r *releasedRows) release() { r.once.Do(r.conn.Release) }

// Close implements pgx.Rows.
func (r *releasedRows) Close() {
	r.Rows.Close()
	r.release()
}

// Next implements pgx.Rows.
func (r *releasedRows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	r.Close()
	return false
}

// releasedRow gives its connection back when it is scanned.
type releasedRow struct {
	row  pgx.Row
	conn *pgxpool.Conn
}

// Scan implements pgx.Row.
func (r releasedRow) Scan(dest ...any) error {
	defer r.conn.Release()
	return r.row.Scan(dest...)
}

// failedRow is the row of a query that got no connection.
type failedRow struct{ err error }

// Scan implements pgx.Row.
func (r failedRow) Scan(...any) error { return r.err }

// releasedTx gives its connection back once the transaction ends. A commit that failed may still
// be rolled back, which finds the transaction closed and releases nothing twice.
type releasedTx struct {
	pgx.Tx
	conn *pgxpool.Conn
	once sync.Once
}

func (t *releasedTx) release() { t.once.Do(t.conn.Release) }

// Commit implements pgx.Tx.
func (t *releasedTx) Commit(ctx context.Context) error {
	defer t.release()
	return t.Tx.Commit(ctx)
}

// Rollback implements pgx.Tx.
func (t *releasedTx) Rollback(ctx context.Context) error {
	defer t.release()
	return t.Tx.Rollback(ctx)
}
