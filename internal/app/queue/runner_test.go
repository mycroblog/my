// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// slowClaimSource hands out one job after a delay, as a slow database would.
type slowClaimSource struct {
	oneJobSource
	delay time.Duration
}

func (s *slowClaimSource) Claim(ctx context.Context, w string, now time.Time, lease time.Duration) (queue.Job, bool, error) {
	time.Sleep(s.delay)
	return s.oneJobSource.Claim(ctx, w, now, lease)
}

// The handler's context ends no later than the lease, which starts before the claim: time spent
// claiming is taken from the handler, so that no other worker can claim the job while it runs.
func TestRunnerHandlerDeadlineEndsWithTheLease(t *testing.T) {
	const lease = time.Second
	src := &slowClaimSource{
		oneJobSource: oneJobSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, completed: make(chan struct{})},
		delay:        100 * time.Millisecond,
	}
	var deadline time.Time
	reg := &queue.JobHandlerRegistry{}
	reg.Register("slow", func(ctx context.Context, _ queue.Job) queue.Result {
		deadline, _ = ctx.Deadline()
		return queue.Done()
	})
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, Lease: lease})
	before := time.Now()
	if _, err := r.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	// The claim took 100ms: a deadline counted from after it would end that much later.
	if latest := before.Add(lease + src.delay/2); deadline.IsZero() || deadline.After(latest) {
		t.Errorf("handler deadline = %v, want no later than %v (the lease taken before the claim)", deadline, latest)
	}
}

// stalledSource never finishes recording a result until its context ends.
type stalledSource struct {
	oneJobSource
}

func (s *stalledSource) Complete(ctx context.Context, _ string, _ queue.Job) error {
	<-ctx.Done()
	return ctx.Err()
}

// Recording a result is bounded by RecordTimeout, even when the runner's context has ended (a
// shutdown): a stalled write does not hold the runner.
func TestRunnerBoundsTheRecordOfAResult(t *testing.T) {
	src := &stalledSource{oneJobSource{emptySource: emptySource{claims: make(chan struct{}, 1)}, completed: make(chan struct{})}}
	ctx, cancel := context.WithCancel(t.Context())
	reg := &queue.JobHandlerRegistry{}
	reg.Register("slow", func(context.Context, queue.Job) queue.Result {
		cancel() // the shutdown starts while the job runs
		return queue.Done()
	})
	r := queue.NewPullRunner(src, reg, queue.RunnerOptions{Clock: clock{}, RecordTimeout: 50 * time.Millisecond})
	done := make(chan error, 1)
	go func() {
		_, err := r.RunOnce(ctx)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("RunOnce = %v, want the record's deadline", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunOnce did not return: the record of the result is not bounded")
	}
}
