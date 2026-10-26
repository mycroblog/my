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

// unit returns wall in the units of an idFloor, 1/4096 millisecond.
func unit(wall time.Time) int64 {
	return wall.UnixMilli()<<12 | int64(wall.Nanosecond()%1_000_000)*4096/1_000_000
}

func TestIDFloorKeepsIDsIncreasingOverSmallStepsBack(t *testing.T) {
	var f idFloor
	base := time.Date(2026, 10, 25, 13, 4, 2, 999_900_000, time.UTC)
	steps := []struct {
		wall time.Time
		want int64
	}{
		{base, unit(base)},
		{base, unit(base) + 1}, // the same instant: one unit later
		{base.Add(-773 * time.Millisecond), unit(base) + 2},                  // a step back across a second: held
		{base.Add(time.Second), unit(base.Add(time.Second))},                 // past it again
		{base.Add(-time.Hour), unit(base.Add(-time.Hour))},                   // a correction: followed
		{base.Add(-time.Hour - time.Second), unit(base.Add(-time.Hour)) + 1}, // held from there
	}
	for i, s := range steps {
		if got := f.next(s.wall); got != s.want {
			t.Errorf("step %d: next(%v) = %d, want %d", i, s.wall, got, s.want)
		}
	}
}

func TestIDFloorNeverRepeatsAcrossGoroutines(t *testing.T) {
	var f idFloor
	base := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			prev := int64(0)
			for i := range 1000 {
				// Each goroutine's wall clock jitters back and forth by up to 5 ms.
				got := f.next(base.Add(time.Duration(i)*time.Millisecond - time.Duration((i+g)%5)*time.Millisecond))
				if got <= prev {
					t.Errorf("goroutine %d: %d after %d", g, got, prev)
					return
				}
				prev = got
			}
		})
	}
	wg.Wait()
}
