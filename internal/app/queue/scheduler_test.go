// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

type stepClock struct{ now time.Time }

func (c *stepClock) Now() time.Time { return c.now }

type recordingQueue struct {
	jobs []queue.NewJob
	err  error
}

func (q *recordingQueue) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, jobs...)
	return nil
}

func TestSchedulerEnqueuesOncePerSlot(t *testing.T) {
	clock := &stepClock{now: time.Date(2026, 10, 16, 12, 0, 30, 0, time.UTC)}
	q := &recordingQueue{}
	s := queue.NewScheduler(q, clock, nil)
	s.Add(queue.Schedule{Name: "cleanup", Every: time.Hour, Job: queue.NewJob{Type: "media.purge_unattached"}})
	s.Add(queue.Schedule{Name: "often", Every: time.Minute, Job: queue.NewJob{Type: "x"}})

	s.Tick(t.Context())
	if len(q.jobs) != 2 {
		t.Fatalf("first Tick enqueued %d jobs, want 2", len(q.jobs))
	}
	slot := clock.now.UnixMilli() / time.Hour.Milliseconds()
	if got, want := q.jobs[0].IdempotencyKey, "schedule:cleanup:"+strconv.FormatInt(slot, 10); got != want || q.jobs[0].Type != "media.purge_unattached" {
		t.Errorf("job = %+v, want key %q", q.jobs[0], want)
	}

	s.Tick(t.Context())
	if len(q.jobs) != 2 {
		t.Fatalf("a second Tick in the same slot enqueued again: %d jobs", len(q.jobs))
	}
	clock.now = clock.now.Add(time.Minute)
	s.Tick(t.Context())
	if len(q.jobs) != 3 || q.jobs[2].Type != "x" {
		t.Fatalf("after a minute: %+v", q.jobs)
	}
	clock.now = clock.now.Add(time.Hour)
	s.Tick(t.Context())
	if len(q.jobs) != 5 {
		t.Fatalf("after an hour: %d jobs, want 5", len(q.jobs))
	}
}

func TestSchedulerRetriesAFailedEnqueue(t *testing.T) {
	clock := &stepClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	q := &recordingQueue{err: errors.New("database is down")}
	s := queue.NewScheduler(q, clock, nil)
	s.Add(queue.Schedule{Name: "cleanup", Every: time.Hour, Job: queue.NewJob{Type: "media.purge_unattached"}})
	s.Tick(t.Context())
	q.err = nil
	s.Tick(t.Context())
	if len(q.jobs) != 1 {
		t.Fatalf("after recovery: %d jobs, want 1", len(q.jobs))
	}
}

func TestSchedulerRejectsBadSchedules(t *testing.T) {
	s := queue.NewScheduler(&recordingQueue{}, &stepClock{}, nil)
	s.Add(queue.Schedule{Name: "a", Every: time.Hour, Job: queue.NewJob{Type: "t"}})
	for name, sc := range map[string]queue.Schedule{
		"duplicate":   {Name: "a", Every: time.Hour, Job: queue.NewJob{Type: "t"}},
		"no name":     {Every: time.Hour, Job: queue.NewJob{Type: "t"}},
		"too often":   {Name: "b", Every: time.Millisecond, Job: queue.NewJob{Type: "t"}},
		"no job type": {Name: "c", Every: time.Hour},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: Add did not panic", name)
				}
			}()
			s.Add(sc)
		}()
	}
}

func TestSchedulerRunUntilStops(t *testing.T) {
	q := &recordingQueue{}
	s := queue.NewScheduler(q, &stepClock{now: time.Unix(0, 0)}, nil)
	s.Add(queue.Schedule{Name: "a", Every: time.Hour, Job: queue.NewJob{Type: "t"}})
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { s.RunUntil(t.Context(), stop, time.Millisecond); close(done) }()
	close(stop)
	<-done
	if len(q.jobs) != 1 {
		t.Errorf("RunUntil enqueued %d jobs, want 1 (the first tick)", len(q.jobs))
	}
}
