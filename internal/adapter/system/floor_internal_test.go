// SPDX-License-Identifier: AGPL-3.0-only

package system

import (
	"sync"
	"testing"
	"time"
)

func TestFloorHoldsOverSmallStepsBack(t *testing.T) {
	var f floor
	base := time.Date(2026, 10, 25, 13, 4, 2, 500_400_000, time.UTC)
	steps := []struct {
		wall time.Time
		want time.Time
	}{
		{base, base.Truncate(time.Millisecond)},
		{base.Add(-2 * time.Second), base.Truncate(time.Millisecond)},                         // a step back: held
		{base.Add(time.Second), base.Add(time.Second).Truncate(time.Millisecond)},             // past it again
		{base.Add(-time.Hour), base.Add(-time.Hour).Truncate(time.Millisecond)},               // a correction: followed
		{base.Add(-time.Hour - time.Minute), base.Add(-time.Hour).Truncate(time.Millisecond)}, // held from there
	}
	for i, s := range steps {
		if got := f.at(s.wall); !got.Equal(s.want) || got.Location() != time.UTC {
			t.Errorf("step %d: at(%v) = %v, want %v in UTC", i, s.wall, got, s.want)
		}
	}
}

func TestFloorNeverGoesBackAcrossGoroutines(t *testing.T) {
	var f floor
	base := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			prev := time.Time{}
			for i := range 1000 {
				// Each goroutine's wall clock jitters back and forth by up to 5 ms.
				got := f.at(base.Add(time.Duration(i)*time.Millisecond - time.Duration((i+g)%5)*time.Millisecond))
				if got.Before(prev) {
					t.Errorf("goroutine %d: %v after %v", g, got, prev)
					return
				}
				prev = got
			}
		})
	}
	wg.Wait()
}
