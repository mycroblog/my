// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Wake is a wake-up signal (ADR-0120 §1): it asks a push receiver to claim the job JobID no
// earlier than At. It carries nothing else: the job, its payload and its traceparent stay in the
// database queue.
type Wake struct {
	JobID string
	At    time.Time
}

// Waker sends wake-up signals through an external queue (Cloud Tasks, SQS), whose deliveries
// reach a PushReceiver. The jobs themselves live in the database queue, which signals each job
// it stores once its transaction commits.
//
// Signals are hints: a duplicate is harmless, because Source.ClaimJob claims a job once, and a
// lost one is sent again by the WakeSweeper. An implementation whose delays are limited sends a
// Wake whose At is beyond its limit at NextVisible(now, At, limit); the receiver signals the job
// again when it arrives early (the re-enqueue chain).
type Waker interface {
	// Wake sends one signal per element of wakes. An error means some of them may not have been
	// sent; the callers log it and leave the rest to the WakeSweeper.
	Wake(ctx context.Context, wakes []Wake) error
}

// WakeFinder reads the jobs of the database queue that need a signal: those whose signal was lost
// and the next job of an ordering key. queue/db implements it.
type WakeFinder interface {
	// DueJobs returns the IDs of at most limit jobs that a pull runner could claim at now and that
	// were due at dueBefore: RunAt <= dueBefore, not leased at now, and not held back by their
	// ordering key; the earliest RunAt first. None when limit <= 0.
	DueJobs(ctx context.Context, now, dueBefore time.Time, limit int) ([]string, error)
	// NextJob returns the first job of orderingKey in enqueue order, as a Wake at its RunAt; ok
	// is false when no job of the key is left.
	NextJob(ctx context.Context, orderingKey string) (w Wake, ok bool, err error)
}

// Defaults of the WakeSweeper.
const (
	// DefaultWakeGrace is how long after its RunAt a job counts as having lost its signal: long
	// enough for the signal sent at the commit to arrive and claim it.
	DefaultWakeGrace = 2 * time.Minute
	// DefaultWakeBatch bounds the signals of one sweep; the next sweep sends the rest.
	DefaultWakeBatch = 500
)

// WakeSweepDeps are the ports and settings of a WakeSweeper. Finder, Waker and Clock are
// required; the zero settings take the defaults.
type WakeSweepDeps struct {
	Finder WakeFinder
	Waker  Waker
	Clock  kernel.Clock
	// Grace is DefaultWakeGrace when zero.
	Grace time.Duration
	// Batch is DefaultWakeBatch when zero.
	Batch int
}

// WakeSweeper is the sweep queue.wake (ADR-0120 §3): it signals again the jobs that have been due
// for longer than the grace period and that no worker holds, which recovers the signals lost
// between a commit and its send, the sends that failed, and the jobs of a worker that stopped
// while it held their lease. Duplicates of signals still on their way are harmless.
type WakeSweeper struct{ d WakeSweepDeps }

// NewWakeSweeper returns the sweep on d. It panics when a required port is nil.
func NewWakeSweeper(d WakeSweepDeps) *WakeSweeper {
	if d.Finder == nil || d.Waker == nil || d.Clock == nil {
		panic("queue: WakeSweepDeps.Finder, Waker and Clock are required")
	}
	if d.Grace <= 0 {
		d.Grace = DefaultWakeGrace
	}
	if d.Batch <= 0 {
		d.Batch = DefaultWakeBatch
	}
	return &WakeSweeper{d: d}
}

// Sweep signals at most one batch of the jobs whose signal is overdue and returns how many it
// signaled. It is safe to repeat and to run concurrently.
func (s *WakeSweeper) Sweep(ctx context.Context) (int, error) {
	now := s.d.Clock.Now()
	ids, err := s.d.Finder.DueJobs(ctx, now, now.Add(-s.d.Grace), s.d.Batch)
	if err != nil {
		return 0, fmt.Errorf("queue: wake sweep: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	wakes := make([]Wake, len(ids))
	for i, id := range ids {
		wakes[i] = Wake{JobID: id, At: now}
	}
	if err := s.d.Waker.Wake(ctx, wakes); err != nil {
		return 0, fmt.Errorf("queue: wake sweep: %w", err)
	}
	return len(wakes), nil
}

// WakeJobs signals the jobs with w and logs a failure, which the WakeSweeper recovers from.
func WakeJobs(ctx context.Context, w Waker, logger *slog.Logger, wakes ...Wake) {
	if w == nil || len(wakes) == 0 {
		return
	}
	if err := w.Wake(ctx, wakes); err != nil {
		if logger == nil {
			logger = slog.Default()
		}
		logger.WarnContext(ctx, "job wake-up signal not sent", slog.String(logJobID, wakes[0].JobID),
			slog.Int("signals", len(wakes)), slog.Any("error", err))
	}
}
