// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// Retention of the dead jobs (FR-803) and of the records of the attempts (ADR-0004 §6).
const (
	// JobPurgeDeadJobs deletes the dead jobs and the records of the attempts past their
	// retention; no payload.
	JobPurgeDeadJobs = "queue.purge_dead_jobs"
	// DeadJobPurgeInterval is the period of the dead job purge's schedule.
	DeadJobPurgeInterval = time.Hour
	// DefaultDeadJobRetention is how long a dead job is kept after it failed when the
	// configuration sets no period: long enough for an operator to notice and requeue it.
	DefaultDeadJobRetention = 30 * 24 * time.Hour
	// DefaultAttemptRetention is how long the record of an attempt is kept after it finished
	// when the configuration sets no period: the same as the dead jobs, so that a dead job keeps
	// its history while it is kept.
	DefaultAttemptRetention = 30 * 24 * time.Hour
	// DeadJobPurgeBatch is the number of dead jobs one DeleteDeadJobsBefore call (one short
	// transaction) deletes; deadJobPurgeMaxBatches bounds the batches of one run, and the next
	// run continues.
	DeadJobPurgeBatch      = 1000
	deadJobPurgeMaxBatches = 20
)

// DeadJobPurgeDeps are the ports of the dead job purge.
type DeadJobPurgeDeps struct {
	DeadJobs DeadJobs
	Clock    kernel.Clock
	// Retention is how long a dead job is kept after it failed; zero means
	// DefaultDeadJobRetention.
	Retention time.Duration
	// Attempts holds the records of the attempts; nil purges none. AttemptRetention is how long
	// a record is kept after its attempt finished; zero means DefaultAttemptRetention.
	Attempts         AttemptLog
	AttemptRetention time.Duration
	// Batch is the number of dead jobs or records per call; zero means DeadJobPurgeBatch.
	Batch int
}

// DeadJobPurger runs JobPurgeDeadJobs: it deletes the dead jobs that failed more than the
// retention ago, so that jobs nobody requeued do not pile up, and the records of the attempts
// that finished more than their retention ago.
type DeadJobPurger struct{ d DeadJobPurgeDeps }

// NewDeadJobPurger returns the dead job purge on d.
func NewDeadJobPurger(d DeadJobPurgeDeps) *DeadJobPurger {
	if d.Retention <= 0 {
		d.Retention = DefaultDeadJobRetention
	}
	if d.AttemptRetention <= 0 {
		d.AttemptRetention = DefaultAttemptRetention
	}
	if d.Batch <= 0 {
		d.Batch = DeadJobPurgeBatch
	}
	return &DeadJobPurger{d: d}
}

// RegisterJobs registers the JobPurgeDeadJobs handler and its hourly schedule.
func (p *DeadJobPurger) RegisterJobs(h *JobHandlerRegistry, s *Scheduler) {
	h.Register(JobPurgeDeadJobs, func(ctx context.Context, _ Job) Result {
		if _, err := p.Run(ctx); err != nil {
			return Retry(0, err)
		}
		return Done()
	})
	s.Add(Schedule{Name: JobPurgeDeadJobs, Every: DeadJobPurgeInterval, Job: NewJob{Type: JobPurgeDeadJobs}})
}

// Run deletes the dead jobs and the records of the attempts past their retention, at most
// deadJobPurgeMaxBatches batches of each, and returns how many dead jobs and records it deleted.
// It is safe to repeat.
func (p *DeadJobPurger) Run(ctx context.Context) (int64, error) {
	now := p.d.Clock.Now()
	total, err := purgeBatches(p.d.Batch, func(limit int) (int64, error) {
		return p.d.DeadJobs.DeleteDeadJobsBefore(ctx, now.Add(-p.d.Retention), limit)
	})
	if err != nil {
		return total, fmt.Errorf("queue: purge dead jobs: %w", err)
	}
	if p.d.Attempts == nil {
		return total, nil
	}
	n, err := purgeBatches(p.d.Batch, func(limit int) (int64, error) {
		return p.d.Attempts.DeleteAttemptsBefore(ctx, now.Add(-p.d.AttemptRetention), limit)
	})
	total += n
	if err != nil {
		return total, fmt.Errorf("queue: purge job attempts: %w", err)
	}
	return total, nil
}

// purgeBatches calls del with batch until it deletes fewer, at most deadJobPurgeMaxBatches
// times, and returns the total.
func purgeBatches(batch int, del func(limit int) (int64, error)) (int64, error) {
	var total int64
	for range deadJobPurgeMaxBatches {
		n, err := del(batch)
		if err != nil {
			return total, err
		}
		total += n
		if n < int64(batch) {
			break
		}
	}
	return total, nil
}
