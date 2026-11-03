// SPDX-License-Identifier: AGPL-3.0-only

package kernel

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ErrDatabaseBusy reports that no connection of the database's pool became free within
// database_acquire_timeout. Nothing is wrong with the work that asked: it can run again, so a
// request answers 503 with Retry-After, and a job waits without using an attempt.
var ErrDatabaseBusy = errors.New("database busy: no free connection in time")

// DatabaseBusyRetryAfter is how long a client is told to wait after ErrDatabaseBusy.
const DatabaseBusyRetryAfter = 5 * time.Second

type databaseBusyKey struct{}

// WithDatabaseBusyNote returns a context whose NoteDatabaseBusy calls busy reports, for the
// interfaces that answer a request with a status and no error at hand.
func WithDatabaseBusyNote(ctx context.Context) (noted context.Context, busy func() bool) {
	note := new(atomic.Bool)
	return context.WithValue(ctx, databaseBusyKey{}, note), note.Load
}

// NoteDatabaseBusy records ErrDatabaseBusy on the note of ctx, if it has one.
func NoteDatabaseBusy(ctx context.Context) {
	if note, ok := ctx.Value(databaseBusyKey{}).(*atomic.Bool); ok {
		note.Store(true)
	}
}
