// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"fmt"

	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/adapter/queue/inline"
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpQueue builds the job queue that cfg.Queue selects, the runner of the worker role, and the
// scheduler of the periodic jobs. The db queue is set with the database, before it.
func (a *App) setUpQueue(cfg *config.Config) error {
	runnerOpts := queue.RunnerOptions{Clock: a.Clock, Logger: a.Logger, Meter: a.Meter, Tracer: a.Tracer, Attempts: a.JobAttempts, Errors: a.ErrorReporter}
	switch cfg.Queue {
	case "db":
		// Set with the database above.
	case "inline":
		q := inline.New(inline.Options{Clock: a.Clock, IDs: a.IDs, Tracer: a.Tracer})
		a.Jobs, a.DeadJobs, a.JobInspector = q, q, q
		a.Runner = queue.NewPullRunner(q, a.Handlers, runnerOpts)
		q.RunSynchronously(a.Runner)
		a.runJobs = func(ctx context.Context, stop <-chan struct{}) error {
			return q.RunUntil(ctx, stop, runnerOpts.PollInterval)
		}
	default:
		return fmt.Errorf("queue %q is not implemented yet", cfg.Queue)
	}
	if a.Runner == nil {
		// The adapter operations of ADR-0056 §1; the inline queue runs the jobs in Enqueue.
		a.Jobs = sysmetrics.Jobs(a.Jobs, a.Meter, "queue."+cfg.Queue)
		a.Runner = queue.NewPullRunner(sysmetrics.Source(a.source, a.Meter, "queue."+cfg.Queue), a.Handlers, runnerOpts)
		a.runJobs = a.Runner.RunUntil
	}
	a.Scheduler = queue.NewScheduler(a.Jobs, a.Clock, a.Logger)
	return nil
}

// setUpDeadJobPurger registers the purge of the dead jobs and of the records of the runs that
// did not succeed after their retention.
func (a *App) setUpDeadJobPurger(cfg *config.Config) {
	queue.NewDeadJobPurger(queue.DeadJobPurgeDeps{
		DeadJobs: a.DeadJobs, Clock: a.Clock, Retention: cfg.DeadJobRetention,
		Attempts: a.JobAttempts, AttemptRetention: cfg.JobAttemptRetention,
	}).RegisterJobs(a.Handlers, a.Scheduler)
}

func (a *App) setDBQueue(q *queuedb.Queue) {
	a.Jobs, a.Outbox, a.DeadJobs, a.JobInspector, a.source = q, q, q, q, q
}
