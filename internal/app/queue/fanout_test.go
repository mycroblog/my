// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mycroblog/my/internal/app/queue"
)

// fanOutFixture is a walk over keys, which a test may change between steps.
type fanOutFixture struct {
	keys    []string
	handled []string
	steps   [][]queue.FanOutStep
	fail    error
}

func (x *fanOutFixture) fan(size, ranges int) queue.FanOut {
	return queue.FanOut{
		Size: size, Ranges: ranges,
		Page: queue.PageByLastKey(func(_ context.Context, after string, limit int) ([]string, error) {
			var page []string
			for _, k := range x.keys {
				if k > after && len(page) < limit {
					page = append(page, k)
				}
			}
			return page, nil
		}),
		Handle: func(_ context.Context, keys []string) error {
			if x.fail != nil {
				return x.fail
			}
			x.handled = append(x.handled, keys...)
			return nil
		},
		Enqueue: func(_ context.Context, steps []queue.FanOutStep) error {
			x.steps = append(x.steps, steps)
			return nil
		},
	}
}

// runAll runs the jobs a fan-out enqueues until none is left, as the queue would.
func (x *fanOutFixture) runAll(t *testing.T, f queue.FanOut) {
	t.Helper()
	if err := f.Run(t.Context(), queue.FanOutStep{}); err != nil {
		t.Fatal(err)
	}
	for len(x.steps) > 0 {
		steps := x.steps[0]
		x.steps = x.steps[1:]
		for _, step := range steps {
			if err := f.Run(t.Context(), step); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestFanOutHandlesASmallWalkItself checks that the first job enqueues nothing when the keys fit in
// one job and the page shows that they end, and handles them.
func TestFanOutHandlesASmallWalkItself(t *testing.T) {
	t.Parallel()
	for _, keys := range [][]string{nil, {"a"}, {"a", "b"}} {
		x := &fanOutFixture{keys: keys}
		if err := x.fan(3, 0).Run(t.Context(), queue.FanOutStep{}); err != nil {
			t.Fatal(err)
		}
		if len(x.steps) != 0 || !slices.Equal(x.handled, keys) {
			t.Errorf("keys %v: enqueued %v, handled %v", keys, x.steps, x.handled)
		}
	}
}

// TestFanOutSplitsALargeWalk checks that the first job of more keys than fit handles none itself and
// that its ranges and what goes on from them cover every key once, without gaps, in jobs of the size.
func TestFanOutSplitsALargeWalk(t *testing.T) {
	t.Parallel()
	x := &fanOutFixture{keys: []string{"a", "b", "c", "d", "e", "f", "g"}}
	f := x.fan(2, 2)
	if err := f.Run(t.Context(), queue.FanOutStep{}); err != nil {
		t.Fatal(err)
	}
	if len(x.handled) != 0 {
		t.Errorf("the first job handled %v, want none", x.handled)
	}
	want := []queue.FanOutStep{
		{Range: &queue.KeyRange{Until: "b"}},
		{Range: &queue.KeyRange{After: "b", Until: "d"}},
		{Cursor: "d"},
	}
	if len(x.steps) != 1 || len(x.steps[0]) != len(want) {
		t.Fatalf("steps = %+v", x.steps)
	}
	for i, s := range x.steps[0] {
		if s.Cursor != want[i].Cursor || (s.Range == nil) != (want[i].Range == nil) || (s.Range != nil && *s.Range != *want[i].Range) {
			t.Errorf("step %d = %+v, want %+v", i, s, want[i])
		}
	}
	x2 := &fanOutFixture{keys: x.keys}
	x2.runAll(t, x2.fan(2, 2))
	if !slices.Equal(x2.handled, x.keys) {
		t.Errorf("handled %v, want every key once", x2.handled)
	}
}

// TestFanOutRangesAndChanges checks that a key added in a range or at the end is handled, that a
// removed key leaves a range reaching into the next without losing any, and that a range run again
// handles only its own keys.
func TestFanOutRangesAndChanges(t *testing.T) {
	t.Parallel()
	x := &fanOutFixture{keys: []string{"b", "d", "f", "h", "j"}}
	f := x.fan(2, 0)
	if err := f.Run(t.Context(), queue.FanOutStep{}); err != nil {
		t.Fatal(err)
	}
	steps := x.steps[0]
	first := *steps[0].Range
	if first != (queue.KeyRange{Until: "d"}) {
		t.Fatalf("first range = %+v", first)
	}
	// A retry of the first range repeats it alone.
	for range 2 {
		x.handled = nil
		if err := f.Run(t.Context(), steps[0]); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(x.handled, []string{"b", "d"}) {
			t.Errorf("first range handled %v", x.handled)
		}
	}
	// One removed and two added: every key there is now is handled.
	x.keys = []string{"b", "c", "d", "h", "j", "k"}
	x.handled = nil
	for _, step := range steps {
		if err := f.Run(t.Context(), step); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range x.keys {
		if !slices.Contains(x.handled, k) {
			t.Errorf("handled %v, missing %q", x.handled, k)
		}
	}
	if slices.Contains(x.handled, "f") {
		t.Errorf("handled the removed key: %v", x.handled)
	}
}

// TestFanOutDescending checks ranges over cursors that go down, and pages that leave keys out but
// still advance.
func TestFanOutDescending(t *testing.T) {
	t.Parallel()
	all := []string{"9", "8", "7", "6", "5"}
	var handled []string
	var steps []queue.FanOutStep
	f := queue.FanOut{
		Size: 2, Descending: true,
		Page: func(_ context.Context, before string, limit int) ([]string, string, error) {
			var page []string
			next := ""
			for _, k := range all {
				if before == "" || k < before {
					page = append(page, k)
					if len(page) == limit {
						next = k
						break
					}
				}
			}
			kept := slices.DeleteFunc(slices.Clone(page), func(k string) bool { return k == "8" })
			return kept, next, nil
		},
		Handle: func(_ context.Context, keys []string) error {
			handled = append(handled, keys...)
			return nil
		},
		Enqueue: func(_ context.Context, s []queue.FanOutStep) error {
			steps = append(steps, s...)
			return nil
		},
	}
	if err := f.Run(t.Context(), queue.FanOutStep{}); err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if err := f.Run(t.Context(), s); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(handled, []string{"9", "7", "6", "5"}) {
		t.Errorf("handled %v", handled)
	}
}

// TestFanOutFailures checks that a failure of a batch fails the job, and that a page that does not
// advance is an error instead of a walk without an end.
func TestFanOutFailures(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	x := &fanOutFixture{keys: []string{"a"}, fail: boom}
	if err := x.fan(2, 0).Run(t.Context(), queue.FanOutStep{}); !errors.Is(err, boom) {
		t.Errorf("Run = %v, want the failure of the batch", err)
	}
	stuck := queue.FanOut{Page: func(context.Context, string, int) ([]string, string, error) { return nil, "x", nil }}
	if err := stuck.Walk(t.Context(), queue.KeyRange{After: "x"}); !errors.Is(err, queue.ErrFanOutPage) {
		t.Errorf("Walk = %v, want ErrFanOutPage", err)
	}
}

// TestStepKey checks that a job's idempotency key names its step, and that the first job keeps the
// key of what started it.
func TestStepKey(t *testing.T) {
	t.Parallel()
	for step, want := range map[*queue.FanOutStep]string{
		{}:                                   "k",
		{Cursor: "c"}:                        "k:cursor:c",
		{Range: &queue.KeyRange{Until: "d"}}: "k:range:",
		{Range: &queue.KeyRange{After: "d"}}: "k:range:d",
	} {
		if got := queue.StepKey("k", *step); got != want {
			t.Errorf("StepKey(%+v) = %q, want %q", *step, got, want)
		}
	}
}
