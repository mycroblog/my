// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// PushReceiver is the push form of JobRunner (ADR-0004, ADR-0120 §2): the endpoint of an
// external queue hands it the job ID of each wake-up signal, and it claims that job by ID
// (Source.ClaimJob), runs it, and records the result as a PullRunner does (Complete, Release or
// Bury, the record of the attempt, and the report of a dead job or a panic).
//
// The receiver keeps the jobs moving with signals of its own: a released job is signaled at its
// new RunAt, a job that settled for good signals the next job of its ordering key, and a job
// that arrived before its RunAt (the delay limit of the external queue) is signaled again at it.
// Retries are the application's (RunAt), not the external queue's: Receive fails only when it
// could not reach the database, and only then should the external queue deliver the signal again.
type PushReceiver struct {
	run    *PullRunner
	waker  Waker
	finder WakeFinder
}

// NewPushReceiver returns a receiver of the signals for the jobs of src, run with the handlers
// in reg and the options of a PullRunner (PollInterval is unused). waker sends its signals and
// finder finds the next job of an ordering key. It panics when opts.Clock, waker or finder is nil.
func NewPushReceiver(src Source, reg *JobHandlerRegistry, waker Waker, finder WakeFinder, opts RunnerOptions) *PushReceiver {
	if waker == nil || finder == nil {
		panic("queue: a push receiver needs a Waker and a WakeFinder")
	}
	return &PushReceiver{run: NewPullRunner(src, reg, opts), waker: waker, finder: finder}
}

// Worker returns the name this receiver claims jobs under.
func (p *PushReceiver) Worker() string { return p.run.opts.Worker }

// Lease returns how long a claim of the receiver lasts, which bounds the run of one signal.
func (p *PushReceiver) Lease() time.Duration { return p.run.opts.Lease }

// Receive handles the signal for the job id: it runs the job when it can claim it and records
// the result. A signal for a job that is not due yet is sent again at its RunAt; one for a job
// that its ordering key holds back, that another worker holds, or that no longer exists is
// dropped. It returns an error only when the claim or the record of the result failed, so that
// the external queue delivers the signal again.
func (p *PushReceiver) Receive(ctx context.Context, id string) error {
	r := p.run
	leaseEnd := time.Now().Add(r.opts.Lease)
	now := r.opts.Clock.Now()
	c, err := r.src.ClaimJob(ctx, id, r.opts.Worker, now, r.opts.Lease)
	if err != nil {
		return err
	}
	switch c.Outcome {
	case Claimed:
	case ClaimNotDue:
		// Delivered early by a queue with a delay limit: the re-enqueue chain.
		WakeJobs(ctx, p.waker, r.opts.Logger, Wake{JobID: id, At: c.Job.RunAt})
		return nil
	default:
		// Held back: the job before it signals it once it settles. Leased or gone: nothing to do.
		return nil
	}
	j := c.Job
	s, err := r.runClaimed(ctx, j, now, leaseEnd)
	if errors.Is(err, ErrLeaseLost) {
		// The lease ran out and another worker claimed the job: that run's result wins.
		r.opts.Logger.WarnContext(ctx, "push receiver: result dropped", slog.String("worker", r.opts.Worker),
			slog.String(logJobID, j.ID), slog.Int(logAttempt, j.Attempt), slog.Any("error", err))
		return nil
	}
	if err != nil {
		return err
	}
	signalCtx, cancel := r.recordContext(ctx)
	defer cancel()
	if s.released {
		WakeJobs(signalCtx, p.waker, r.opts.Logger, Wake{JobID: j.ID, At: s.runAt})
		return nil
	}
	if j.OrderingKey == "" {
		return nil
	}
	next, ok, err := p.finder.NextJob(signalCtx, j.OrderingKey)
	if err != nil {
		// The result is recorded; the sweep signals the next job.
		r.opts.Logger.WarnContext(ctx, "push receiver: next job of the ordering key not found", slog.String(logJobID, j.ID), slog.Any("error", err))
		return nil
	}
	if ok {
		next.At = maxTime(next.At, r.opts.Clock.Now())
		WakeJobs(signalCtx, p.waker, r.opts.Logger, next)
	}
	return nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
