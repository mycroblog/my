// SPDX-License-Identifier: AGPL-3.0-only

// Package uow defines the UnitOfWork port (ADR-0003): every write to the database runs in one
// short, retryable transaction obtained through UnitOfWork.Tx. It also holds the pieces every
// database adapter shares: the bounded retry loop and the registry of synchronous projectors
// that run inside the command's transaction (ADR-0020 rule 7).
//
// The production implementations live in internal/adapter/db/*; their shared contract test
// suite is uowtest.UnitOfWorkContract in pkg/testkit/uowtest.
package uow

import (
	"context"
	"errors"
)

// ErrReadOnly is what Tx.Emit returns in a read-only transaction (UnitOfWork.Read). A write
// through the transaction's statements fails with the database's own error, which says the
// transaction is read-only (PostgreSQL SQLSTATE 25006, SQLite SQLITE_READONLY).
var ErrReadOnly = errors.New("uow: the transaction is read-only")

// UnitOfWork runs functions in database transactions.
type UnitOfWork interface {
	// Tx runs fn in a new transaction. It commits when fn returns nil and rolls back when fn
	// returns an error or panics (the panic is re-raised after the rollback).
	//
	// When fn or the commit fails with an error for which errors.Is(err, ErrRetryable) holds
	// (a serialization failure, deadlock, or lock timeout), Tx rolls back and calls fn again
	// in a fresh transaction, up to the adapter's RetryPolicy. fn must therefore be safe to
	// run more than once:
	//   - Do no external I/O in fn (HTTP, queue, blob storage): it cannot be rolled back and
	//     would repeat on retry. Enqueue jobs through the outbox instead (ADR-0004).
	//   - Generate IDs, timestamps, and idempotency keys before calling Tx and pass them in,
	//     so that every attempt writes the same values.
	//   - Reset any variable outside fn that fn assigns, at the start of fn.
	//   - Keep the transaction short; split bulk work into jobs.
	//
	// Use tx only inside fn and only from the goroutine running fn.
	//
	// Commit boundaries:
	//   - Tx returns nil only after the commit succeeded; the commit hooks then run for the
	//     events of the attempt that committed.
	//   - When ctx ends before the commit, Tx returns an error and nothing commits.
	//   - Concurrent transactions are isolated at least as READ COMMITTED, not serializable:
	//     they commit in no order that Tx defines, and a read-modify-write is safe only when a
	//     unique constraint or a conditional write (an upsert, an UPDATE ... WHERE on the value
	//     read) guards it. Of conflicting writers of one key, the constraint lets exactly one
	//     commit and the others fail with a non-retryable error (ADR-0003).
	//   - An error of the commit itself that the database does not report as a rollback (a
	//     connection lost during COMMIT) leaves the outcome unknown: the writes may or may not
	//     have committed, Tx does not retry it, and no commit hook runs. Writes made safe to
	//     repeat (idempotency keys, upserts) let the caller repeat the command.
	Tx(ctx context.Context, fn func(tx Tx) error) error

	// Read runs fn in a new read-only transaction, for several reads that must see one state
	// of the database. It does not commit, and it ends when fn returns: any error of fn is
	// returned as it is.
	//
	//   - A write through tx fails with an error that says the transaction is read-only
	//     (ErrReadOnly, or the database's own error), and Tx.Emit returns ErrReadOnly: no
	//     projector or commit hook runs.
	//   - Read does not retry fn: a reader has no conflict to retry. fn still does no external
	//     I/O and keeps the transaction short, as in Tx.
	//   - On SQLite the transaction does not take the database's one write lock, so it neither
	//     waits for a writer nor holds one up (WAL journaling lets a reader see the state as of
	//     its first read); Tx takes the lock when it begins. On PostgreSQL it is a READ ONLY
	//     transaction at READ COMMITTED, as Tx is.
	//   - A single statement needs no transaction: read it through the repository outside of
	//     any Tx. Use Read only where fn must see one state across statements.
	//
	// Use tx only inside fn and only from the goroutine running fn. A Tx of Read is accepted
	// by the repositories as any other, for their reads.
	Read(ctx context.Context, fn func(tx Tx) error) error
}

// Tx is a transaction in progress, passed to the function given to UnitOfWork.Tx.
//
// Repositories bind to it: an adapter's repository implementation recovers the adapter's own
// transaction from the Tx it receives, so application code never sees driver types. A Tx belongs
// to the database that began it: a repository of another database, even one of the same
// adapter, rejects it rather than writing in the database the Tx runs in.
type Tx interface {
	// Emit runs the synchronous projectors registered for e.EventType() inside this transaction,
	// in registration order. It returns the first projector error; returning that error from fn
	// rolls back the command together with its projections. Once the transaction commits, the
	// adapter passes the events of that attempt to Projectors.Committed, which runs the commit
	// hooks registered for them.
	Emit(ctx context.Context, e Event) error
}

// Options configure a UnitOfWork adapter. The zero value is valid: the default RetryPolicy and
// no projectors.
type Options struct {
	Retry RetryPolicy
	// Projectors are the synchronous projectors that Tx.Emit runs and the commit hooks that run
	// after the commit. The registry is shared, not
	// copied, so the composition root may register projectors after creating the adapter, as
	// long as it does so before the first transaction.
	Projectors *Projectors
}
