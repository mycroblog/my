// SPDX-License-Identifier: AGPL-3.0-only

package testkit_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/mycroblog/my/pkg/testkit"
)

func TestEventually(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if !testkit.Eventually(t, time.Second, func() bool { return time.Since(start) >= 100*time.Millisecond }) {
			t.Error("a condition that holds after 100ms did not hold within a second")
		}
		if waited := time.Since(start); waited > 150*time.Millisecond {
			t.Errorf("Eventually returned %s after the start, want soon after the 100ms", waited)
		}

		start = time.Now()
		if testkit.Eventually(t, time.Second, func() bool { return false }) {
			t.Error("a condition that never holds held")
		}
		if waited := time.Since(start); waited != time.Second {
			t.Errorf("Eventually gave up after %s, want the timeout of 1s", waited)
		}
	})
}
