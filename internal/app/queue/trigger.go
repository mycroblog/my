// SPDX-License-Identifier: AGPL-3.0-only

package queue

import "context"

// Trigger is the external trigger of the periodic work of a server whose jobs arrive by
// wake-up signals (ADR-0120 §4): a platform scheduler (Cloud Scheduler, EventBridge Scheduler)
// calls Tick about once a minute, and the process runs no Scheduler of its own. The scheduler
// must claim its slots in a shared key-value store (Scheduler.UseSharedSlots), because the ticks
// of one period may reach several processes.
type Trigger struct {
	scheduler *Scheduler
	sweeper   *WakeSweeper
}

// NewTrigger returns the trigger of scheduler and sweeper. It panics when scheduler has no
// shared slots or either is nil.
func NewTrigger(scheduler *Scheduler, sweeper *WakeSweeper) *Trigger {
	if scheduler == nil || sweeper == nil {
		panic("queue: a trigger needs a Scheduler and a WakeSweeper")
	}
	scheduler.mu.Lock()
	shared := scheduler.kv != nil
	scheduler.mu.Unlock()
	if !shared {
		panic("queue: the scheduler of a trigger must use shared slots (Scheduler.UseSharedSlots)")
	}
	return &Trigger{scheduler: scheduler, sweeper: sweeper}
}

// Tick enqueues the jobs of the schedules whose period came (Scheduler.Tick) and then runs the
// sweep queue.wake once, in the request rather than as a job, so that a lost signal never holds
// back the sweep that recovers it. It returns the error of the sweep; the scheduler logs its own.
func (t *Trigger) Tick(ctx context.Context) error {
	t.scheduler.Tick(ctx)
	_, err := t.sweeper.Sweep(ctx)
	return err
}
