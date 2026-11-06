// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// fakeDeadJobs holds dead jobs by failure time and records the cutoffs it was asked for.
type fakeDeadJobs struct {
	failed  []time.Time
	cutoffs []time.Time
	err     error
}

func (f *fakeDeadJobs) ListDeadJobs(context.Context, string, queue.DeadJobPosition, int) ([]queue.DeadJob, error) {
	return nil, nil
}

func (f *fakeDeadJobs) GetDeadJob(context.Context, string) (queue.DeadJob, error) {
	return queue.DeadJob{}, queue.ErrDeadJobNotFound
}

func (f *fakeDeadJobs) RequeueDeadJob(context.Context, string) error { return nil }

func (f *fakeDeadJobs) DeleteDeadJob(context.Context, string) error { return nil }

func (f *fakeDeadJobs) DeleteDeadJobsBefore(_ context.Context, t time.Time, limit int) (int64, error) {
	f.cutoffs = append(f.cutoffs, t)
	if f.err != nil {
		return 0, f.err
	}
	var n int64
	kept := f.failed[:0]
	for _, at := range f.failed {
		if at.Before(t) && n < int64(limit) {
			n++
			continue
		}
		kept = append(kept, at)
	}
	f.failed = kept
	return n, nil
}

func TestDeadJobPurgerDeletesPastRetentionInBatches(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := &fakeDeadJobs{}
	for i := range 5 {
		f.failed = append(f.failed, now.Add(-31*24*time.Hour-time.Duration(i)*time.Minute))
	}
	recent := now.Add(-29 * 24 * time.Hour)
	f.failed = append(f.failed, recent)
	p := queue.NewDeadJobPurger(queue.DeadJobPurgeDeps{DeadJobs: f, Clock: &stepClock{now: now}, Batch: 2})
	n, err := p.Run(t.Context())
	if err != nil || n != 5 {
		t.Fatalf("Run = %d, %v; want 5", n, err)
	}
	if len(f.failed) != 1 || !f.failed[0].Equal(recent) {
		t.Errorf("left %v, want only the job within the retention", f.failed)
	}
	if want := now.Add(-queue.DefaultDeadJobRetention); len(f.cutoffs) != 3 || !f.cutoffs[0].Equal(want) {
		t.Errorf("cutoffs = %v, want 3 batches before %v", f.cutoffs, want)
	}
}

func TestDeadJobPurgerUsesTheConfiguredRetention(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	f := &fakeDeadJobs{}
	p := queue.NewDeadJobPurger(queue.DeadJobPurgeDeps{DeadJobs: f, Clock: &stepClock{now: now}, Retention: 48 * time.Hour})
	if _, err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(f.cutoffs) != 1 || !f.cutoffs[0].Equal(now.Add(-48*time.Hour)) {
		t.Errorf("cutoffs = %v, want one before %v", f.cutoffs, now.Add(-48*time.Hour))
	}
	f.err = errors.New("db down")
	if _, err := p.Run(t.Context()); !errors.Is(err, f.err) {
		t.Errorf("Run error = %v, want %v", err, f.err)
	}
}

// fakeAttempts holds records of attempts by finish time and records the cutoffs it was asked for.
type fakeAttempts struct {
	finished []time.Time
	cutoffs  []time.Time
}

func (f *fakeAttempts) RecordAttempt(context.Context, queue.Attempt) error { return nil }

func (f *fakeAttempts) ListAttempts(context.Context, string, queue.AttemptPosition, int) ([]queue.Attempt, error) {
	return nil, nil
}

func (f *fakeAttempts) DeleteAttemptsBefore(_ context.Context, t time.Time, limit int) (int64, error) {
	f.cutoffs = append(f.cutoffs, t)
	var n int64
	kept := f.finished[:0]
	for _, at := range f.finished {
		if at.Before(t) && n < int64(limit) {
			n++
			continue
		}
		kept = append(kept, at)
	}
	f.finished = kept
	return n, nil
}

func TestDeadJobPurgerDeletesAttemptsPastTheirOwnRetention(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	dead := &fakeDeadJobs{failed: []time.Time{now.Add(-3 * 24 * time.Hour)}}
	attempts := &fakeAttempts{finished: []time.Time{now.Add(-3 * 24 * time.Hour), now.Add(-time.Hour)}}
	p := queue.NewDeadJobPurger(queue.DeadJobPurgeDeps{
		DeadJobs: dead, Clock: &stepClock{now: now}, Attempts: attempts, AttemptRetention: 48 * time.Hour,
	})
	n, err := p.Run(t.Context())
	if err != nil || n != 1 {
		t.Fatalf("Run = %d, %v; want 1 record", n, err)
	}
	if len(dead.failed) != 1 {
		t.Errorf("dead jobs left %v, want the one within the default retention", dead.failed)
	}
	if len(attempts.finished) != 1 || !attempts.finished[0].Equal(now.Add(-time.Hour)) {
		t.Errorf("attempts left %v, want the recent one", attempts.finished)
	}
	if len(attempts.cutoffs) != 1 || !attempts.cutoffs[0].Equal(now.Add(-48*time.Hour)) {
		t.Errorf("attempt cutoffs = %v, want one before %v", attempts.cutoffs, now.Add(-48*time.Hour))
	}

	attempts = &fakeAttempts{}
	p = queue.NewDeadJobPurger(queue.DeadJobPurgeDeps{DeadJobs: &fakeDeadJobs{}, Clock: &stepClock{now: now}, Attempts: attempts})
	if _, err := p.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := now.Add(-queue.DefaultAttemptRetention); len(attempts.cutoffs) != 1 || !attempts.cutoffs[0].Equal(want) {
		t.Errorf("attempt cutoffs = %v, want one before %v", attempts.cutoffs, want)
	}
}

func TestDeadJobPurgerRegistersAnHourlySchedule(t *testing.T) {
	h := &queue.JobHandlerRegistry{}
	s := queue.NewScheduler(&recordingQueue{}, &stepClock{}, nil)
	queue.NewDeadJobPurger(queue.DeadJobPurgeDeps{DeadJobs: &fakeDeadJobs{}, Clock: &stepClock{}}).RegisterJobs(h, s)
	got := s.Schedules()
	if len(got) != 1 || got[0].Job.Type != queue.JobPurgeDeadJobs || got[0].Every != queue.DeadJobPurgeInterval {
		t.Errorf("schedules = %+v, want the hourly dead job purge", got)
	}
}
