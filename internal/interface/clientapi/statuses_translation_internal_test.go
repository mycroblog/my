// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	"testing"
	"time"
)

func TestUntilNextMonth(t *testing.T) {
	for _, c := range []struct {
		now  time.Time
		want int
	}{
		{time.Date(2026, 11, 30, 23, 59, 0, 0, time.UTC), 60},
		{time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC), 3600},
		{time.Date(2026, 11, 30, 23, 59, 59, 500_000_000, time.UTC), 1},
		// The first minutes of December in Japan are still November in UTC.
		{time.Date(2026, 12, 1, 8, 59, 0, 0, time.FixedZone("JST", 9*3600)), 60},
	} {
		if got := untilNextMonth(c.now); got != c.want {
			t.Errorf("untilNextMonth(%v) = %d, want %d", c.now, got, c.want)
		}
	}
}
