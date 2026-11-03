// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// SlotRunner runs up to a fixed number of jobs at once in one process (ADR-0140). Each slot is a
// PullRunner under its own worker name that claims a job only when it is free, so a claimed job
// never waits for a slot and a job's lease means what it means with one runner.
//
// Slots claim in parallel while jobs are due. While none is due only one slot polls, so that an
// idle process costs the database what one runner costs; the others wait until a claim finds a job.
// Each claim wakes one waiting slot, which claims in turn, so that a backlog reaches every slot
// without a burst of empty claims for each job.
type SlotRunner struct {
	slots []*PullRunner

	mu sync.Mutex
	// polling reports that a slot holds the polling role.
	polling bool
	// wake lets one waiting slot try a claim. A send that finds it full loses nothing: the slot
	// it wakes wakes the next one if it finds a job.
	wake chan struct{}
}

var _ JobRunner = (*SlotRunner)(nil)

// NewSlotRunner returns a runner of n jobs at once. The worker name of each slot is opts.Worker,
// or a random name, with the number of the slot. It panics when opts.Clock is nil or n < 1.
func NewSlotRunner(src Source, reg *JobHandlerRegistry, opts RunnerOptions, n int) *SlotRunner {
	if n < 1 {
		panic("queue: a SlotRunner needs a slot")
	}
	first := NewPullRunner(src, reg, opts)
	s := &SlotRunner{slots: make([]*PullRunner, n), wake: make(chan struct{}, 1)}
	for i := range s.slots {
		slot := *first
		slot.opts.Worker = fmt.Sprintf("%s-%d", first.opts.Worker, i)
		s.slots[i] = &slot
	}
	return s
}

// Run implements JobRunner.
func (s *SlotRunner) Run(ctx context.Context) error {
	return s.RunUntil(ctx, ctx.Done())
}

// RunUntil runs jobs in every slot until stop is closed or ctx ends, and returns nil once every slot
// has stopped. Closing stop is a graceful stop: no slot claims another job, but the jobs in progress
// keep ctx and run to their end. Ending ctx also cancels the contexts of the jobs in progress.
func (s *SlotRunner) RunUntil(ctx context.Context, stop <-chan struct{}) error {
	var wg sync.WaitGroup
	s.polling = true // the first slot polls from the start, and the others wait for its first job
	for i, slot := range s.slots {
		wg.Add(1)
		kernel.Go(ctx, "queue.slot", slot.opts.Errors, func(ctx context.Context) {
			defer wg.Done()
			s.runSlot(ctx, stop, slot, i == 0)
		})
	}
	wg.Wait()
	return nil
}

// runSlot is one slot: it claims and runs jobs while they are due, then polls if no other slot does,
// and otherwise waits to be woken.
func (s *SlotRunner) runSlot(ctx context.Context, stop <-chan struct{}, r *PullRunner, polling bool) {
	if !polling && !s.waitForWake(ctx, stop) {
		return
	}
	for !stopped(ctx, stop) {
		ran, err := r.runOnce(ctx, func() {
			if polling {
				polling = false
				s.leavePolling()
			}
			s.wakeOne()
		})
		if stopped(ctx, stop) {
			return
		}
		if err != nil {
			r.opts.Logger.ErrorContext(ctx, "job runner", slog.String("worker", r.opts.Worker), slog.Any("error", err))
		}
		if ran && err == nil {
			continue
		}
		if !polling {
			polling = s.takePolling()
		}
		if polling {
			if !sleepUnlessStopped(ctx, stop, r.opts.PollInterval) {
				return
			}
		} else if !s.waitForWake(ctx, stop) {
			return
		}
	}
}

// takePolling gives the caller the polling role unless another slot holds it. A slot waits only
// while another polls, so that a due job always finds a slot unless all of them are busy.
func (s *SlotRunner) takePolling() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.polling {
		return false
	}
	s.polling = true
	return true
}

func (s *SlotRunner) leavePolling() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polling = false
}

// wakeOne lets a waiting slot try a claim. It runs after the polling role is left, so that the slot
// it wakes takes the role when it finds no job.
func (s *SlotRunner) wakeOne() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// waitForWake waits until a claim wakes the slot, and reports false when the runner stops first.
func (s *SlotRunner) waitForWake(ctx context.Context, stop <-chan struct{}) bool {
	select {
	case <-s.wake:
		return true
	case <-stop:
		return false
	case <-ctx.Done():
		return false
	}
}

func stopped(ctx context.Context, stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return ctx.Err() != nil
	}
}

// sleepUnlessStopped waits for d and reports false when the runner stops first.
func sleepUnlessStopped(ctx context.Context, stop <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-stop:
		return false
	case <-ctx.Done():
		return false
	}
}
