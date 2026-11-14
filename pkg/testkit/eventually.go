// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"testing"
	"time"
)

// Eventually checks cond until it holds, and reports whether it did within the timeout. It checks at once, then at
// intervals that grow from a millisecond to 50 milliseconds, and once more at the timeout.
//
// It is for a condition that signals nothing to wait on: another process, a database, a server across the network.
// Wait on a channel the code under test signals, or run the test in a testing/synctest bubble, wherever one exists.
func Eventually(t testing.TB, within time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.NewTimer(within)
	defer deadline.Stop()
	interval := time.Millisecond
	for {
		if cond() {
			return true
		}
		next := time.NewTimer(interval)
		select {
		case <-next.C:
		case <-deadline.C:
			next.Stop()
			return cond()
		}
		interval = min(2*interval, 50*time.Millisecond)
	}
}
