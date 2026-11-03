// SPDX-License-Identifier: AGPL-3.0-only

package sqlite

import (
	"bytes"
	"database/sql"
	"runtime"
	"sync"
	"sync/atomic"
)

// A read on the pool while the same goroutine holds a transaction takes a second connection.
// When concurrent callers hold every connection of a pool, each waits for one more and none moves
// until a deadline ends the wait. PostgreSQL's pool is small by default, and nothing in SQLite
// shows the mistake: readers do not block there. Tests turn this check on to find such reads on the
// paths that they run on SQLite, the dialect of most of them.

// secondConnectionHook is what the check reports to; nil, as outside tests, turns it off.
var secondConnectionHook atomic.Pointer[func(stack []byte)]

// heldTransactions is the database of each goroutine in a transaction, while the check is on.
var heldTransactions sync.Map

// ReportSecondConnections calls report with the stack of each read on the pool, or transaction
// begun, by a goroutine that holds a transaction of the same database. It is for tests, and
// applies to every database of the process; a read in a goroutine that the transaction's function
// starts is not seen.
func ReportSecondConnections(report func(stack []byte)) { secondConnectionHook.Store(&report) }

func goroutineID() string {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	return string(bytes.Fields(b)[1]) // "goroutine 123 [running]:..."
}

// holdingTransaction runs fn, the function of a transaction of db.
func holdingTransaction(db *sql.DB, fn func() error) error {
	if secondConnectionHook.Load() == nil {
		return fn()
	}
	id := goroutineID()
	heldTransactions.Store(id, db)
	defer heldTransactions.Delete(id)
	return fn()
}

// checkSecondConnection reports a use of db's pool by a goroutine that holds a transaction of it.
func checkSecondConnection(db *sql.DB) {
	report := secondConnectionHook.Load()
	if report == nil {
		return
	}
	if held, ok := heldTransactions.Load(goroutineID()); !ok || held != db {
		return
	}
	stack := make([]byte, 16<<10)
	(*report)(stack[:runtime.Stack(stack, false)])
}
