// SPDX-License-Identifier: AGPL-3.0-only

package queue_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// listSource hands out the jobs added to it, and counts the claims and the workers that made them.
type listSource struct {
	emptySource

	mu      sync.Mutex
	jobs    []queue.Job
	claims  int
	workers map[string]bool
}

func (s *listSource) add(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for range n {
		s.jobs = append(s.jobs, queue.Job{ID: "job", Type: "test", Attempt: 1})
	}
}

func (s *listSource) claimCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claims
}

func (s *listSource) Claim(_ context.Context, worker string, _ time.Time, _ time.Duration) (queue.Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims++
	if s.workers == nil {
		s.workers = map[string]bool{}
	}
	if len(s.jobs) == 0 {
		return queue.Job{}, false, nil
	}
	s.workers[worker] = true
	j := s.jobs[0]
	s.jobs = s.jobs[1:]
	return j, true, nil
}

// slotGate holds the handlers until they are released, and records how many ran at once.
type slotGate struct {
	mu      sync.Mutex
	running int
	most    int
	started chan struct{}
	release chan struct{}
}

func newSlotGate() *slotGate {
	return &slotGate{started: make(chan struct{}, 256), release: make(chan struct{})}
}

func (g *slotGate) handler(ctx context.Context, _ queue.Job) queue.Result {
	g.mu.Lock()
	g.running++
	g.most = max(g.most, g.running)
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		g.running--
		g.mu.Unlock()
	}()
	g.started <- struct{}{}
	select {
	case <-g.release:
		return queue.Done()
	case <-ctx.Done():
		return queue.Retry(0, ctx.Err())
	}
}

func (g *slotGate) waitStarted(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-g.started:
		case <-time.After(10 * time.Second):
			t.Fatal("a job did not start")
		}
	}
}

func (g *slotGate) mostAtOnce() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.most
}

func startSlots(t *testing.T, src queue.Source, g *slotGate, n int, poll time.Duration) (stop chan struct{}, cancel context.CancelFunc, done chan error) {
	t.Helper()
	reg := &queue.JobHandlerRegistry{}
	reg.Register("test", g.handler)
	r := queue.NewSlotRunner(src, reg, queue.RunnerOptions{Clock: clock{}, PollInterval: poll}, n)
	ctx, cancel := context.WithCancel(t.Context())
	stop, done = make(chan struct{}), make(chan error, 1)
	go func() { done <- r.RunUntil(ctx, stop) }()
	return stop, cancel, done
}

func waitStopped(t *testing.T, done chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunUntil = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the runner did not stop")
	}
}

func TestSlotRunnerRunsAsManyJobsAsSlots(t *testing.T) {
	t.Parallel()
	const slots = 4
	src, g := &listSource{}, newSlotGate()
	src.add(10)
	stop, cancel, done := startSlots(t, src, g, slots, time.Hour)
	defer cancel()

	g.waitStarted(t, slots)
	// Every slot is busy: nothing claims the jobs that are left, so none of them waits for a slot
	// with its lease running.
	busy := src.claimCount()
	time.Sleep(50 * time.Millisecond)
	if got := src.claimCount(); got != busy {
		t.Errorf("claims with every slot busy went from %d to %d", busy, got)
	}
	close(g.release)
	g.waitStarted(t, 6)
	close(stop)
	waitStopped(t, done)
	if got := g.mostAtOnce(); got != slots {
		t.Errorf("most jobs at once = %d, want %d", got, slots)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.workers) != slots {
		t.Errorf("claims came from %d workers, want one name per slot: %v", len(src.workers), src.workers)
	}
}

func TestSlotRunnerPollsWithOneSlotWhileIdle(t *testing.T) {
	t.Parallel()
	const poll = 20 * time.Millisecond
	src, g := &listSource{}, newSlotGate()
	stop, cancel, done := startSlots(t, src, g, 64, poll)
	defer cancel()

	time.Sleep(20 * poll)
	close(stop)
	waitStopped(t, done)
	// A poll per interval from one slot: well under what 64 slots polling would make.
	if got := src.claimCount(); got < 2 || got > 30 {
		t.Errorf("claims while idle = %d, want about 20", got)
	}
}

// A slot keeps polling while another runs a job, so that a job that comes later does not wait.
func TestSlotRunnerPollsWhileASlotIsBusy(t *testing.T) {
	t.Parallel()
	src, g := &listSource{}, newSlotGate()
	src.add(1)
	stop, cancel, done := startSlots(t, src, g, 3, 5*time.Millisecond)
	defer cancel()

	g.waitStarted(t, 1)
	src.add(1)
	g.waitStarted(t, 1)
	src.add(1)
	g.waitStarted(t, 1)
	close(g.release)
	close(stop)
	waitStopped(t, done)
}

// Jobs after an idle time reach every slot, not only the one that polls.
func TestSlotRunnerWakesTheSlotsForABacklog(t *testing.T) {
	t.Parallel()
	const slots = 8
	src, g := &listSource{}, newSlotGate()
	stop, cancel, done := startSlots(t, src, g, slots, 5*time.Millisecond)
	defer cancel()

	time.Sleep(30 * time.Millisecond)
	src.add(slots)
	g.waitStarted(t, slots)
	close(g.release)
	close(stop)
	waitStopped(t, done)
}

func TestSlotRunnerStop(t *testing.T) {
	t.Parallel()
	t.Run("closing stop lets the jobs in progress finish and claims no more", func(t *testing.T) {
		t.Parallel()
		src, g := &listSource{}, newSlotGate()
		src.add(5)
		stop, cancel, done := startSlots(t, src, g, 2, time.Hour)
		defer cancel()
		g.waitStarted(t, 2)
		close(stop)
		select {
		case <-done:
			t.Fatal("the runner stopped with jobs in progress")
		case <-time.After(50 * time.Millisecond):
		}
		claims := src.claimCount()
		close(g.release)
		waitStopped(t, done)
		if got := src.claimCount(); got != claims {
			t.Errorf("claims after stop went from %d to %d", claims, got)
		}
		src.mu.Lock()
		defer src.mu.Unlock()
		if len(src.jobs) != 3 {
			t.Errorf("%d jobs are left, want 3", len(src.jobs))
		}
	})
	t.Run("ending the context cancels the jobs in progress", func(t *testing.T) {
		t.Parallel()
		src, g := &listSource{}, newSlotGate()
		src.add(2)
		_, cancel, done := startSlots(t, src, g, 2, time.Hour)
		g.waitStarted(t, 2)
		cancel()
		waitStopped(t, done)
	})
	t.Run("waiting slots stop too", func(t *testing.T) {
		t.Parallel()
		src, g := &listSource{}, newSlotGate()
		stop, cancel, done := startSlots(t, src, g, 16, time.Hour)
		defer cancel()
		close(stop)
		waitStopped(t, done)
	})
}
