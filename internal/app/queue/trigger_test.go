// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// memoryKV is a kernel.KeyValueStore whose entries expire by a stepClock. (testkit.KeyValueStore
// does the same, but the application layer does not import pkg/testkit.)
type memoryKV struct {
	clock   *stepClock
	mu      sync.Mutex
	entries map[string]kvEntry
}

type kvEntry struct {
	value     []byte
	expiresAt time.Time
}

func newMemoryKV(clock *stepClock) *memoryKV {
	return &memoryKV{clock: clock, entries: map[string]kvEntry{}}
}

func (s *memoryKV) live(k string) ([]byte, bool) {
	e, ok := s.entries[k]
	if !ok || (!e.expiresAt.IsZero() && !e.expiresAt.After(s.clock.Now())) {
		return nil, false
	}
	return e.value, true
}

func (s *memoryKV) put(k string, v []byte, ttl time.Duration) {
	e := kvEntry{value: v}
	if ttl > 0 {
		e.expiresAt = s.clock.Now().Add(ttl)
	}
	s.entries[k] = e
}

func (s *memoryKV) Get(_ context.Context, ns, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.live(ns + "/" + key); ok {
		return v, nil
	}
	return nil, fmt.Errorf("get %s: %w", key, kernel.ErrKeyNotFound)
}

func (s *memoryKV) Set(_ context.Context, ns, key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.put(ns+"/"+key, value, ttl)
	return nil
}

func (s *memoryKV) Delete(_ context.Context, ns, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, ns+"/"+key)
	return nil
}

func (s *memoryKV) IncrBy(context.Context, string, string, int64, time.Duration) (int64, error) {
	return 0, errors.New("not used")
}

func (s *memoryKV) CompareAndSwap(_ context.Context, ns, key string, old, value []byte, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := ns + "/" + key
	cur, ok := s.live(k)
	if (old == nil && ok) || (old != nil && (!ok || !bytes.Equal(cur, old))) {
		return false, nil
	}
	if value == nil {
		delete(s.entries, k)
	} else {
		s.put(k, value, ttl)
	}
	return true, nil
}

func (s *memoryKV) DeleteExpired(context.Context, int) (int64, error) { return 0, nil }

// The ticks of one period can reach several processes, or one process after the job of the
// period completed: the shared slots enqueue the job once anyway.
func TestSharedSlotsEnqueueOncePerPeriod(t *testing.T) {
	clock := &stepClock{now: time.Date(2026, 10, 29, 12, 0, 10, 0, time.UTC)}
	kv := newMemoryKV(clock)
	// recordingQueue keeps no idempotency keys, as if every job completed at once.
	q := &recordingQueue{}
	newScheduler := func() *queue.Scheduler {
		s := queue.NewScheduler(q, clock, nil)
		s.Add(queue.Schedule{Name: "often", Every: time.Minute, Job: queue.NewJob{Type: "x"}})
		s.UseSharedSlots(kv)
		return s
	}
	a, b := newScheduler(), newScheduler()

	a.Tick(t.Context())
	b.Tick(t.Context())
	clock.now = clock.now.Add(30 * time.Second)
	a.Tick(t.Context())
	newScheduler().Tick(t.Context()) // a new process in the same period
	if len(q.jobs) != 1 {
		t.Fatalf("enqueued %d jobs in one period, want 1", len(q.jobs))
	}

	clock.now = clock.now.Add(30 * time.Second) // the next period
	b.Tick(t.Context())
	a.Tick(t.Context())
	if len(q.jobs) != 2 || q.jobs[0].IdempotencyKey == q.jobs[1].IdempotencyKey {
		t.Fatalf("jobs = %+v, want one more for the next period", q.jobs)
	}

	// A failed enqueue gives the slot back, so that the next tick of the period tries again.
	clock.now = clock.now.Add(time.Minute)
	q.err = errors.New("db down")
	a.Tick(t.Context())
	q.err = nil
	b.Tick(t.Context())
	b.Tick(t.Context())
	if len(q.jobs) != 3 {
		t.Fatalf("enqueued %d jobs, want the failed period's job once on the next tick", len(q.jobs))
	}
}

// dueFinder is a WakeFinder of fixed due jobs.
type dueFinder struct {
	ids       []string
	dueBefore time.Time
	err       error
}

func (f *dueFinder) DueJobs(_ context.Context, _, dueBefore time.Time, limit int) ([]string, error) {
	f.dueBefore = dueBefore
	return f.ids[:min(limit, len(f.ids))], f.err
}

func (f *dueFinder) NextJob(context.Context, string) (queue.Wake, bool, error) {
	return queue.Wake{}, false, nil
}

// recordingWaker records the signals it sends, or fails with err.
type recordingWaker struct {
	wakes []queue.Wake
	err   error
}

func (w *recordingWaker) Wake(_ context.Context, wakes []queue.Wake) error {
	if w.err != nil {
		return w.err
	}
	w.wakes = append(w.wakes, wakes...)
	return nil
}

func TestTriggerTicksTheSchedulerAndSweeps(t *testing.T) {
	clock := &stepClock{now: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
	q := &recordingQueue{}
	s := queue.NewScheduler(q, clock, nil)
	s.Add(queue.Schedule{Name: "often", Every: time.Minute, Job: queue.NewJob{Type: "x"}})
	finder := &dueFinder{ids: []string{testID(1), testID(2)}}
	waker := &recordingWaker{}
	sweeper := queue.NewWakeSweeper(queue.WakeSweepDeps{Finder: finder, Waker: waker, Clock: clock})

	func() {
		defer func() {
			if recover() == nil {
				t.Error("NewTrigger accepted a scheduler without shared slots")
			}
		}()
		queue.NewTrigger(s, sweeper)
	}()
	s.UseSharedSlots(newMemoryKV(clock))
	trigger := queue.NewTrigger(s, sweeper)

	if err := trigger.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(q.jobs) != 1 {
		t.Errorf("Tick enqueued %d jobs, want the due schedule's", len(q.jobs))
	}
	if want := clock.Now().Add(-queue.DefaultWakeGrace); !finder.dueBefore.Equal(want) {
		t.Errorf("swept the jobs due before %v, want %v (the grace period)", finder.dueBefore, want)
	}
	w := waker.wakes
	if len(w) != 2 || w[0].JobID != testID(1) || w[1].JobID != testID(2) || !w[0].At.Equal(clock.Now()) {
		t.Errorf("signals = %v, want the due jobs now", w)
	}

	// A sweep that cannot send its signals fails the tick, so that the platform retries it.
	waker.err = errors.New("queue down")
	if err := trigger.Tick(t.Context()); err == nil {
		t.Error("Tick = nil with a failing waker")
	}
	finder.err = errors.New("db down")
	if err := trigger.Tick(t.Context()); err == nil {
		t.Error("Tick = nil with a failing finder")
	}
}
