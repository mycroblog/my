// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"slices"
	"sync"

	"github.com/mycroblog/my/internal/app/queue"
)

// Waker is a queue.Waker that records the signals it is given, for the tests of the wake-up
// signals (ADR-0120). It is safe for concurrent use.
type Waker struct {
	mu    sync.Mutex
	wakes []queue.Wake
	err   error
}

var _ queue.Waker = (*Waker)(nil)

// Wake implements queue.Waker: it records wakes, or returns the error of Fail and records
// nothing.
func (w *Waker) Wake(_ context.Context, wakes []queue.Wake) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	w.wakes = append(w.wakes, wakes...)
	return nil
}

// Fail makes the later calls of Wake return err, a lost signal; nil sends them again.
func (w *Waker) Fail(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = err
}

// Take returns the signals recorded since the last Take, in order, and forgets them.
func (w *Waker) Take() []queue.Wake {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := slices.Clone(w.wakes)
	w.wakes = nil
	return out
}

// Len returns the number of signals recorded since the last Take.
func (w *Waker) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.wakes)
}
