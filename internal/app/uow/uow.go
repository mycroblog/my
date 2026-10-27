// SPDX-License-Identifier: AGPL-3.0-only

// Package uow defines the UnitOfWork port (ADR-0003): every write to the database runs in one
// short, retryable transaction obtained through UnitOfWork.Tx. It also holds the pieces every
// database adapter shares: the bounded retry loop and the registry of synchronous projectors
// that run inside the command's transaction (ADR-0020 rule 7).
//
// The production implementations live in internal/adapter/db/*; their shared contract test
// suite is testkit.UnitOfWorkContract in pkg/testkit.
package uow

import "context"

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
