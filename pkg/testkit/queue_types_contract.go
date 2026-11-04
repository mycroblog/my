// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// QueueTypesSource returns the view of a queue whose Claim takes only jobs of types.
type QueueTypesSource func(types []string) queue.Source

// QueueTypesContract verifies a queue whose workers can run only some job types (ADR-0004 §16):
// such a worker takes the earliest due job of its types and never one of another type, even an
// older one; leases and due times work as in a plain claim; an earlier job of an ordering key
// holds a job back whatever its type; a plain claim still takes every type; and concurrent workers
// claim each job once.
func QueueTypesContract(t *testing.T, newHarness func(t *testing.T, clock *Clock) (QueueHarness, QueueTypesSource)) {
	t.Helper()

	setup := func(t *testing.T) (context.Context, *Clock, QueueHarness, QueueTypesSource) {
		t.Helper()
		clock := NewClock(contractStart)
		h, only := newHarness(t, clock)
		return t.Context(), clock, h, only
	}
	enqueue := func(t *testing.T, h QueueHarness, jobs ...queue.NewJob) {
		t.Helper()
		if err := h.Queue.Enqueue(t.Context(), jobs...); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	wantClaim := func(t *testing.T, src queue.Source, clock *Clock, wantType string) queue.Job {
		t.Helper()
		j, ok, err := src.Claim(t.Context(), "w", clock.Now(), contractLease)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if !ok {
			t.Fatalf("Claim found nothing, want a job of type %s", wantType)
		}
		if j.Type != wantType {
			t.Fatalf("Claim = type %s, want %s", j.Type, wantType)
		}
		return j
	}
	wantNone := func(t *testing.T, src queue.Source, clock *Clock) {
		t.Helper()
		j, ok, err := src.Claim(t.Context(), "w", clock.Now(), contractLease)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		if ok {
			t.Fatalf("Claim = %+v, want no job", j)
		}
	}

	t.Run("takes only its types, the earliest first", func(t *testing.T) {
		_, clock, h, only := setup(t)
		enqueue(t, h,
			queue.NewJob{Type: "test.skip", RunAt: contractStart.Add(-time.Hour)},
			queue.NewJob{Type: "test.b", RunAt: contractStart.Add(-time.Minute)},
			queue.NewJob{Type: "test.a", RunAt: contractStart.Add(-2 * time.Minute)},
		)
		src := only([]string{"test.a", "test.b", "test.none"})
		wantClaim(t, src, clock, "test.a")
		wantClaim(t, src, clock, "test.b")
		wantNone(t, src, clock)
		// A plain claim still takes every type.
		wantClaim(t, h.Source, clock, "test.skip")
	})

	t.Run("due times and leases hold", func(t *testing.T) {
		_, clock, h, only := setup(t)
		enqueue(t, h, queue.NewJob{Type: "test.a", RunAt: contractStart.Add(time.Minute)})
		src := only([]string{"test.a"})
		wantNone(t, src, clock)
		clock.Advance(time.Minute)
		j := wantClaim(t, src, clock, "test.a")
		if j.Attempt != 1 {
			t.Errorf("Attempt = %d, want 1", j.Attempt)
		}
		wantNone(t, src, clock)
		clock.Advance(contractLease + time.Second)
		j = wantClaim(t, src, clock, "test.a")
		if j.Attempt != 2 {
			t.Errorf("Attempt after the lease = %d, want 2", j.Attempt)
		}
		if err := src.Complete(t.Context(), "w", j); err != nil {
			t.Fatalf("Complete: %v", err)
		}
		wantNone(t, src, clock)
	})

	t.Run("an ordering key holds a job back across types", func(t *testing.T) {
		ctx, clock, h, only := setup(t)
		enqueue(t, h, queue.NewJob{Type: "test.first", OrderingKey: "k"})
		clock.Advance(time.Second)
		enqueue(t, h, queue.NewJob{Type: "test.second", OrderingKey: "k"})
		second := only([]string{"test.second"})
		// The earlier job is due and of a type this worker skips.
		wantNone(t, second, clock)
		// Another worker holds it.
		first := wantClaim(t, only([]string{"test.first"}), clock, "test.first")
		wantNone(t, second, clock)
		// It finishes.
		if err := h.Source.Complete(ctx, "w", first); err != nil {
			t.Fatalf("Complete: %v", err)
		}
		wantClaim(t, second, clock, "test.second")
	})

	t.Run("a held job does not block the other jobs of the type", func(t *testing.T) {
		_, clock, h, only := setup(t)
		enqueue(t, h, queue.NewJob{Type: "test.first", OrderingKey: "k"})
		clock.Advance(time.Second)
		enqueue(t, h,
			queue.NewJob{Type: "test.a", OrderingKey: "k"},
			queue.NewJob{Type: "test.a"},
		)
		j := wantClaim(t, only([]string{"test.a"}), clock, "test.a")
		if j.OrderingKey != "" {
			t.Errorf("Claim took the held job %+v", j)
		}
	})

	t.Run("concurrent workers claim each job once", func(t *testing.T) {
		ctx, clock, h, only := setup(t)
		const jobs = 40
		for i := range jobs {
			enqueue(t, h,
				queue.NewJob{Type: fmt.Sprintf("test.t%d", i%4), IdempotencyKey: fmt.Sprintf("run:%d", i)},
				queue.NewJob{Type: "test.skip", IdempotencyKey: fmt.Sprintf("skip:%d", i)},
			)
		}
		src := only([]string{"test.t0", "test.t1", "test.t2", "test.t3"})
		var (
			mu   sync.Mutex
			got  []string
			wg   sync.WaitGroup
			errs []error
		)
		for w := range 4 {
			wg.Go(func() {
				worker := fmt.Sprintf("w%d", w)
				for {
					j, ok, err := src.Claim(ctx, worker, clock.Now(), time.Hour)
					mu.Lock()
					if err != nil {
						errs = append(errs, err)
					}
					if ok {
						got = append(got, j.ID)
						if j.Type == "test.skip" {
							errs = append(errs, fmt.Errorf("claimed a skipped job %s", j.ID))
						}
					}
					mu.Unlock()
					if !ok || err != nil {
						return
					}
				}
			})
		}
		wg.Wait()
		for _, err := range errs {
			t.Error(err)
		}
		slices.Sort(got)
		if len(got) != jobs || len(slices.Compact(got)) != jobs {
			t.Errorf("claimed %d jobs, %d different, want %d", len(got), len(slices.Compact(got)), jobs)
		}
	})
}
