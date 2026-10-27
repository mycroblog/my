// SPDX-License-Identifier: AGPL-3.0-only

package inline_test

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/adapter/queue/inline"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/pkg/testkit"
)

func factory(maxDelay time.Duration) testkit.QueueFactory {
	return func(_ *testing.T, clock *testkit.Clock) testkit.QueueHarness {
		q := inline.New(inline.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock), MaxDelay: maxDelay})
		return testkit.QueueHarness{Queue: q, Source: q, DeadJobs: q, MaxDelay: maxDelay, Inspector: q}
	}
}

func TestQueueContract(t *testing.T) {
	testkit.QueueContract(t, factory(0))
}

// TestQueueContractWithDelayLimit emulates the 15-minute delay limit of SQS, which exercises
// the re-enqueue chain.
func TestQueueContractWithDelayLimit(t *testing.T) {
	testkit.QueueContract(t, factory(15*time.Minute))
}

func TestRunSynchronously(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	q := inline.New(inline.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)})
	var reg queue.JobHandlerRegistry
	var ran []string
	reg.Register("parent", func(ctx context.Context, _ queue.Job) queue.Result {
		ran = append(ran, "parent")
		// Enqueuing from a handler joins the running drain instead of recursing.
		if err := q.Enqueue(ctx, queue.NewJob{Type: "child"}); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	reg.Register("child", func(context.Context, queue.Job) queue.Result {
		ran = append(ran, "child")
		return queue.Done()
	})
	q.RunSynchronously(queue.NewPullRunner(q, &reg, queue.RunnerOptions{Clock: clock}))

	later := queue.NewJob{Type: "child", Payload: json.RawMessage(`{}`), RunAt: clock.Now().Add(time.Minute)}
	if err := q.Enqueue(t.Context(), queue.NewJob{Type: "parent"}, later); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if len(ran) != 2 || ran[0] != "parent" || ran[1] != "child" {
		t.Fatalf("Enqueue ran %v, want [parent child] before returning", ran)
	}
	clock.Advance(time.Minute)
	if err := q.Enqueue(t.Context()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if len(ran) != 3 {
		t.Fatalf("ran %v, want the delayed child once it is due", ran)
	}
}

// An Enqueue from another goroutine while a drain runs (a scheduled job, say) does not return
// before its own jobs ran.
func TestRunSynchronouslyWhileAnotherDrainRuns(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	q := inline.New(inline.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)})
	var reg queue.JobHandlerRegistry
	started, release := make(chan struct{}), make(chan struct{})
	reg.Register("slow", func(context.Context, queue.Job) queue.Result {
		close(started)
		<-release
		return queue.Done()
	})
	var mu sync.Mutex
	ran := false
	reg.Register("quick", func(context.Context, queue.Job) queue.Result {
		mu.Lock()
		defer mu.Unlock()
		ran = true
		return queue.Done()
	})
	q.RunSynchronously(queue.NewPullRunner(q, &reg, queue.RunnerOptions{Clock: clock}))

	slow := make(chan error, 1)
	go func() { slow <- q.Enqueue(context.Background(), queue.NewJob{Type: "slow"}) }()
	<-started
	quick := make(chan error, 1)
	go func() { quick <- q.Enqueue(t.Context(), queue.NewJob{Type: "quick"}) }()
	select {
	case err := <-quick:
		t.Fatalf("Enqueue returned (%v) while another drain held its job", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-quick; err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !ran {
		t.Errorf("Enqueue returned before its job ran")
	}
	if err := <-slow; err != nil {
		t.Errorf("Enqueue of the slow job: %v", err)
	}
}

// An Enqueue does not return before its jobs ran while RunUntil polls the queue: the loop never
// claims a job outside the drain of the Enqueue that added it.
func TestRunUntilWithSynchronousEnqueue(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	q := inline.New(inline.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)})
	var reg queue.JobHandlerRegistry
	var mu sync.Mutex
	ran := map[string]bool{}
	reg.Register("job", func(_ context.Context, j queue.Job) queue.Result {
		time.Sleep(10 * time.Microsecond) // widens the window in which another worker holds the job
		mu.Lock()
		defer mu.Unlock()
		ran[string(j.Payload)] = true
		return queue.Done()
	})
	q.RunSynchronously(queue.NewPullRunner(q, &reg, queue.RunnerOptions{Clock: clock}))

	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- q.RunUntil(t.Context(), stop, time.Nanosecond) }()
	for i := range 2000 {
		payload := json.RawMessage(strconv.Itoa(i))
		if err := q.Enqueue(t.Context(), queue.NewJob{Type: "job", Payload: payload}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		mu.Lock()
		ok := ran[string(payload)]
		mu.Unlock()
		if !ok {
			t.Fatalf("Enqueue %d returned before its job ran", i)
		}
	}
	close(stop)
	if err := <-done; err != nil {
		t.Errorf("RunUntil: %v", err)
	}
}

// An Enqueue whose synchronous run fails after the jobs were stored returns nil: the jobs are
// enqueued (here, already run), and JobQueue.Enqueue returns an error only when it enqueued none.
func TestEnqueueDoesNotReportErrorsOfTheRun(t *testing.T) {
	clock := testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	q := inline.New(inline.Options{Clock: clock, IDs: testkit.NewIDGenerator(clock)})
	var reg queue.JobHandlerRegistry
	ran := 0
	reg.Register("stolen", func(ctx context.Context, _ queue.Job) queue.Result {
		ran++
		// The lease runs out and another worker claims the job, so that recording this run's
		// result fails with ErrLeaseLost.
		clock.Advance(2 * time.Minute)
		if _, ok, err := q.Claim(ctx, "other", clock.Now(), time.Hour); err != nil || !ok {
			t.Errorf("Claim by another worker = %v, %v", ok, err)
		}
		return queue.Done()
	})
	q.RunSynchronously(queue.NewPullRunner(q, &reg, queue.RunnerOptions{Clock: clock, Lease: time.Minute}))
	if err := q.Enqueue(t.Context(), queue.NewJob{Type: "stolen"}); err != nil {
		t.Fatalf("Enqueue = %v, want nil once the job is enqueued", err)
	}
	if ran != 1 {
		t.Fatalf("ran %d times, want 1", ran)
	}
	jobs, err := q.ListJobs(t.Context(), clock.Now(), queue.JobFilter{State: queue.JobStateRunning}, queue.JobPosition{}, 10)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("running jobs = %v, %v, want the job the other worker holds", jobs, err)
	}
}
