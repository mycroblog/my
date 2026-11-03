// SPDX-License-Identifier: AGPL-3.0-only

package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	queuedb "github.com/mycroblog/my/internal/adapter/queue/db"
	"github.com/mycroblog/my/internal/adapter/queue/inline"
	"github.com/mycroblog/my/internal/adapter/telemetry/sysmetrics"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/interface/queuepush"
	"github.com/mycroblog/my/internal/platform/config"
)

// setUpQueue builds the job queue that cfg.Queue selects, the runner of the worker role, and the
// scheduler of the periodic jobs. The db queue is set with the database, before it, and so is the
// database queue of cloudtasks and sqs with its waker: there a push receiver runs the jobs of
// the signals instead of a runner (ADR-0120 §2).
func (a *App) setUpQueue(cfg *config.Config) error {
	runnerOpts := a.runnerOptions()
	switch cfg.Queue {
	case "db":
		// Set with the database above.
	case "cloudtasks", "sqs":
		// The jobs are in the database queue, whose operations are measured as queue.db.
		a.Jobs = sysmetrics.Jobs(a.Jobs, a.Meter, "queue.db")
		src := sysmetrics.Source(a.source, a.Meter, "queue.db")
		a.pushReceiver = queue.NewPushReceiver(src, a.Handlers, a.waker, a.wakeFinder, runnerOpts)
		a.Scheduler = queue.NewScheduler(a.Jobs, a.Clock, a.Logger)
		return nil
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
		a.runnerSource = sysmetrics.Source(a.source, a.Meter, "queue."+cfg.Queue)
		a.Runner = queue.NewPullRunner(a.runnerSource, a.Handlers, runnerOpts)
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
	a.dbQueue = q
	a.Jobs, a.Outbox, a.DeadJobs, a.JobInspector, a.source, a.wakeFinder = q, q, q, q, q, q
}

// setUpWorkerJobTypes limits the runner to the job types of worker_job_types or
// worker_job_types_except (ADR-0138), once every handler is registered. The configuration allows
// them only with the db queue, whose scheduler it leaves as it is.
func (a *App) setUpWorkerJobTypes(cfg *config.Config) error {
	if len(cfg.WorkerJobTypes) == 0 && len(cfg.WorkerJobTypesExcept) == 0 {
		return nil
	}
	if a.dbQueue == nil {
		return fmt.Errorf("worker_job_types needs queue = db, not %s", cfg.Queue)
	}
	run, skip, err := queue.SelectJobTypes(a.Handlers.Types(), cfg.WorkerJobTypes, cfg.WorkerJobTypesExcept)
	if err != nil {
		return fmt.Errorf("worker_job_types: %w", err)
	}
	a.skippedJobTypes = skip
	a.runnerSource = sysmetrics.Source(a.dbQueue.OnlyTypes(run), a.Meter, "queue."+cfg.Queue)
	a.Runner = queue.NewPullRunner(a.runnerSource, a.Handlers, a.runnerOptions())
	a.runJobs = a.Runner.RunUntil
	return nil
}

// ErrWorkerJobConcurrency is the failure of a worker_job_concurrency that the database's pool
// cannot serve.
var ErrWorkerJobConcurrency = errors.New("worker_job_concurrency is too high for the database's pool")

// sqliteJobConcurrencyWarning is the worker_job_concurrency above which a database without a pool
// logs a warning: SQLite writes one at a time, and the throughput fell at 128 (ADR-0140 §4).
const sqliteJobConcurrencyWarning = 32

// setUpWorkerJobConcurrency makes the worker role run worker_job_concurrency jobs at once
// (ADR-0140), after setUpWorkerJobTypes chose the runner's source. A process that does not poll the
// table has nothing to check: the settings refuse more than one there.
func (a *App) setUpWorkerJobConcurrency(cfg *config.Config) error {
	n := cfg.WorkerJobConcurrency
	if n == 0 || cfg.Role == config.RoleWeb || a.runnerSource == nil {
		return nil
	}
	// A slot waits for a connection to claim and to record, and a handler that takes a second one
	// while holding a transaction stalls until its deadline when the slots reach the maximum.
	if a.dbPoolMax > 0 && n >= a.dbPoolMax {
		return fmt.Errorf("%w: %d must be below the %d connections of the pool: raise pool_max_conns in database_url or lower the setting", ErrWorkerJobConcurrency, n, a.dbPoolMax)
	}
	if n > 1 {
		a.runJobs = queue.NewSlotRunner(a.runnerSource, a.Handlers, a.runnerOptions(), n).RunUntil
	}
	return nil
}

// setUpWakeRoutes checks the routes of the wake-up signals against the registered job types, which
// is known only now (ADR-0139 §1), and gives the sweep a read per route (§4).
func (a *App) setUpWakeRoutes() error {
	if a.wakeRouter == nil {
		return nil
	}
	groups, err := a.wakeRouter.TypesOfRoutes(a.Handlers.Types())
	if err != nil {
		return fmt.Errorf("queue routes: %w", err)
	}
	a.wakeSweeper.UseRoutes(groups)
	return nil
}

func (a *App) runnerOptions() queue.RunnerOptions {
	return queue.RunnerOptions{Clock: a.Clock, Logger: a.Logger, Meter: a.Meter, Tracer: a.Tracer, Attempts: a.JobAttempts, Errors: a.ErrorReporter}
}

// setUpPushEndpoints serves the endpoints of the push receiver and of the external trigger of a
// queue with a waker (ADR-0120 §4, §5), once the KV and the HTTP client are set: the scheduler
// claims its periods in the KV, which every process shares, and the platform's scheduler, not
// the process, ticks it.
func (a *App) setUpPushEndpoints(cfg *config.Config) {
	if a.pushReceiver == nil {
		return
	}
	a.Scheduler.UseSharedSlots(a.KV)
	a.wakeSweeper = queue.NewWakeSweeper(queue.WakeSweepDeps{Finder: a.wakeFinder, Waker: a.waker, Clock: a.Clock})
	a.trigger = queue.NewTrigger(a.Scheduler, a.wakeSweeper)
	if cfg.Queue == "sqs" {
		// The Lambda Web Adapter passes the SQS events of the signals and the ticks through.
		queuepush.NewLambda(queuepush.LambdaOptions{
			Receiver: a.pushReceiver, Trigger: a.trigger, Path: cfg.QueueSQSEventsPath, Budget: cfg.QueueSQSEventsBudget,
			Problem: a.writeStatusProblem, Logger: a.Logger,
		}).Register(internalRouter{a})
		return
	}
	if a.pushAuth == nil {
		a.pushAuth = newCloudTasksAuthenticator(cfg, a.HTTPClient, a.Clock)
	}
	queuepush.New(queuepush.Options{
		Receiver: a.pushReceiver, Trigger: a.trigger, Auth: a.pushAuth,
		BaseURL: cfg.QueueCloudTasksURL, Problem: a.writeStatusProblem, Logger: a.Logger,
	}).Register(internalRouter{a})
}

// internalRouter registers the endpoints that the platform's queue and scheduler call, which the
// roles that run jobs serve.
type internalRouter struct{ a *App }

func (r internalRouter) Handle(pattern string, h http.Handler) {
	r.a.internalRoutes = append(r.a.internalRoutes, route{pattern: pattern, handler: h, surface: surfaceQueue})
}
