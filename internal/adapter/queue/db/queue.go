// SPDX-License-Identifier: AGPL-3.0-only

package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// Options configure a Queue. Clock and IDs are required.
type Options struct {
	Clock kernel.Clock
	IDs   kernel.IDGenerator
	// Tracer fills the TraceParent of the enqueued jobs from the span of the caller's context
	// (ADR-0063 §2); nil leaves it empty.
	Tracer kernel.Tracer
	// Waker, when set, makes Queue send a wake-up signal for each job it stores, once the
	// transaction that stores it commits (ADR-0120 §1): the jobs of a server whose external
	// queue (Cloud Tasks, SQS) delivers them to a queue.PushReceiver. A failed send is logged,
	// and the queue.WakeSweeper sends the signal again.
	Waker queue.Waker
	// Projectors are the registry of the database's UnitOfWork, whose commit hook sends the
	// signals of EnqueueTx. Required with Waker.
	Projectors *uow.Projectors
	// Logger receives the failed sends of Waker; the default is slog.Default().
	Logger *slog.Logger
}

func (o Options) check() {
	if o.Clock == nil || o.IDs == nil {
		panic("queue/db: Options.Clock and Options.IDs are required")
	}
	if o.Waker != nil && o.Projectors == nil {
		panic("queue/db: Options.Projectors is required with Options.Waker")
	}
}

// store runs the dialect's queries inside a transaction of the dialect's UnitOfWork.
type store interface {
	insertJob(ctx context.Context, tx uow.Tx, j queue.Job) error
	claim(ctx context.Context, tx uow.Tx, worker string, now, until time.Time) (queue.Job, bool, error)
	deleteClaimed(ctx context.Context, tx uow.Tx, id, worker string, attempt int) (int64, error)
	release(ctx context.Context, tx uow.Tx, id, worker string, attempt, newAttempt int, runAt time.Time, lastErr string) (int64, error)
	insertDeadFromClaim(ctx context.Context, tx uow.Tx, id, worker string, attempt int, lastErr string, failedAt time.Time) (int64, error)
	listDead(ctx context.Context, tx uow.Tx, jobType string, after queue.DeadJobPosition, limit int) ([]queue.DeadJob, error)
	requeueDead(ctx context.Context, tx uow.Tx, id string, runAt time.Time) error
	deleteDead(ctx context.Context, tx uow.Tx, id string) (int64, error)
	deleteDeadBefore(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error)
	backlog(ctx context.Context, tx uow.Tx, now time.Time) (queue.Backlog, error)
	listPending(ctx context.Context, tx uow.Tx, now time.Time, f queue.JobFilter, after queue.JobPosition, limit int) ([]queue.PendingJob, error)
	summarize(ctx context.Context, tx uow.Tx, now time.Time) (queue.JobSummary, error)
	insertAttempt(ctx context.Context, tx uow.Tx, a queue.Attempt) error
	listAttempts(ctx context.Context, tx uow.Tx, jobID string, after queue.AttemptPosition, limit int) ([]queue.Attempt, error)
	deleteAttemptsBefore(ctx context.Context, tx uow.Tx, before time.Time, limit int) (int64, error)
	jobLease(ctx context.Context, tx uow.Tx, id string) (lease, bool, error)
	claimID(ctx context.Context, tx uow.Tx, id, orderingKey, worker string, now, until time.Time) (queue.Job, bool, error)
	dueJobs(ctx context.Context, tx uow.Tx, now, dueBefore time.Time, limit int) ([]string, error)
	nextOfKey(ctx context.Context, tx uow.Tx, orderingKey string) (queue.Wake, bool, error)
}

// lease is what a claim by ID reads of a job before it claims it.
type lease struct {
	orderingKey string
	runAt       time.Time
	// lockedUntil is zero when the job was never leased or its lease ended.
	lockedUntil time.Time
}

// Queue is the database job queue.
type Queue struct {
	db   uow.UnitOfWork
	st   store
	opts Options
}

var (
	_ queue.JobQueue      = (*Queue)(nil)
	_ queue.Outbox        = (*Queue)(nil)
	_ queue.Source        = (*Queue)(nil)
	_ queue.DeadJobs      = (*Queue)(nil)
	_ queue.BacklogReader = (*Queue)(nil)
	_ queue.JobInspector  = (*Queue)(nil)
	_ queue.AttemptLog    = (*Queue)(nil)
	_ queue.WakeFinder    = (*Queue)(nil)
)

// eventWakeAfterCommit is the event type through which EnqueueTx hands the signals of its jobs
// to the commit hook.
const eventWakeAfterCommit = "queue.wake_after_commit"

type wakeAfterCommit struct {
	owner *Queue
	wakes []queue.Wake
}

func (wakeAfterCommit) EventType() string { return eventWakeAfterCommit }

func newQueue(d uow.UnitOfWork, st store, opts Options) *Queue {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	q := &Queue{db: d, st: st, opts: opts}
	if opts.Waker != nil {
		opts.Projectors.OnCommit(eventWakeAfterCommit, q.committed)
	}
	return q
}

func (q *Queue) committed(ctx context.Context, e uow.Event) {
	ev, ok := e.(wakeAfterCommit)
	if !ok || ev.owner != q {
		return
	}
	queue.WakeJobs(context.WithoutCancel(ctx), q.opts.Waker, q.opts.Logger, ev.wakes...)
}

// wakesOf returns the signals of jobs, each at its RunAt.
func wakesOf(jobs []queue.Job) []queue.Wake {
	w := make([]queue.Wake, len(jobs))
	for i, j := range jobs {
		w[i] = queue.Wake{JobID: j.ID, At: j.RunAt}
	}
	return w
}

// build turns new jobs into stored jobs with fresh IDs, traced from the span of ctx.
func build(ctx context.Context, opts Options, jobs []queue.NewJob) ([]queue.Job, error) {
	now := opts.Clock.Now()
	out := make([]queue.Job, len(jobs))
	for i, n := range jobs {
		j, err := n.Build(opts.IDs.NewID(), now)
		if err != nil {
			return nil, err
		}
		out[i] = j
	}
	queue.TraceJobs(ctx, opts.Tracer, out)
	return out, nil
}

// Enqueue implements queue.JobQueue in a transaction of its own.
func (q *Queue) Enqueue(ctx context.Context, jobs ...queue.NewJob) error {
	built, err := build(ctx, q.opts, jobs)
	if err != nil {
		return err
	}
	if err := q.db.Tx(ctx, func(tx uow.Tx) error { return q.insert(ctx, tx, built) }); err != nil {
		return err
	}
	// A job that an idempotency key skipped has no row; its signal finds nothing to claim.
	queue.WakeJobs(ctx, q.opts.Waker, q.opts.Logger, wakesOf(built)...)
	return nil
}

// EnqueueTx implements queue.Outbox: it inserts the jobs in tx, so they commit or roll back
// with the caller's domain writes. tx must come from the database this queue lives in.
func (q *Queue) EnqueueTx(ctx context.Context, tx uow.Tx, jobs ...queue.NewJob) error {
	// IDs are generated per attempt: a retried transaction rolled the previous rows back.
	built, err := build(ctx, q.opts, jobs)
	if err != nil {
		return err
	}
	if err := q.insert(ctx, tx, built); err != nil {
		return err
	}
	if q.opts.Waker == nil || len(built) == 0 {
		return nil
	}
	// The signals go once the transaction commits; a rolled-back or retried attempt sends none.
	return tx.Emit(ctx, wakeAfterCommit{owner: q, wakes: wakesOf(built)})
}

func (q *Queue) insert(ctx context.Context, tx uow.Tx, jobs []queue.Job) error {
	for _, j := range jobs {
		if err := q.st.insertJob(ctx, tx, j); err != nil {
			return fmt.Errorf("queue/db: enqueue %s: %w", j.Type, err)
		}
	}
	return nil
}

// Claim implements queue.Source.
func (q *Queue) Claim(ctx context.Context, worker string, now time.Time, lease time.Duration) (queue.Job, bool, error) {
	var (
		j  queue.Job
		ok bool
	)
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		j, ok, err = q.st.claim(ctx, tx, worker, now, now.Add(lease))
		return err
	})
	if err != nil {
		return queue.Job{}, false, fmt.Errorf("queue/db: claim: %w", err)
	}
	return j, ok, nil
}

// ClaimJob implements queue.Source. It reads the job's lease, then claims it with the statements
// of Claim (on PostgreSQL the advisory lock of its ordering key and the claim of the job), and
// reads the lease again when the claim leased nothing, to tell why.
func (q *Queue) ClaimJob(ctx context.Context, id, worker string, now time.Time, d time.Duration) (queue.ClaimResult, error) {
	if !kernel.ValidID(id) {
		return queue.ClaimResult{Outcome: queue.ClaimUnavailable}, nil
	}
	now = kernel.Normalize(now)
	var r queue.ClaimResult
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		l, found, err := q.st.jobLease(ctx, tx, id)
		if err != nil {
			return err
		}
		if r = unclaimed(id, now, l, found); r.Outcome != queue.Claimed {
			return nil
		}
		j, ok, err := q.st.claimID(ctx, tx, id, l.orderingKey, worker, now, now.Add(d))
		if err != nil || ok {
			r = queue.ClaimResult{Outcome: queue.Claimed, Job: j}
			return err
		}
		if l, found, err = q.st.jobLease(ctx, tx, id); err != nil {
			return err
		}
		if r = unclaimed(id, now, l, found); r.Outcome == queue.Claimed {
			// Due and unleased, yet not claimed: its ordering key holds it back.
			r.Outcome = queue.ClaimHeldBack
		}
		return nil
	})
	if err != nil {
		return queue.ClaimResult{}, fmt.Errorf("queue/db: claim job: %w", err)
	}
	return r, nil
}

// unclaimed returns why the job id with lease l cannot be claimed at now, or Claimed when
// nothing but its ordering key may hold it back.
func unclaimed(id string, now time.Time, l lease, found bool) queue.ClaimResult {
	switch {
	case !found || l.lockedUntil.After(now):
		return queue.ClaimResult{Outcome: queue.ClaimUnavailable}
	case l.runAt.After(now):
		return queue.ClaimResult{Outcome: queue.ClaimNotDue, Job: queue.Job{ID: id, RunAt: l.runAt}}
	}
	return queue.ClaimResult{Outcome: queue.Claimed}
}

// DueJobs implements queue.WakeFinder with the condition of a claim, without its row locks.
func (q *Queue) DueJobs(ctx context.Context, now, dueBefore time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	var ids []string
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		ids, err = q.st.dueJobs(ctx, tx, kernel.Normalize(now), kernel.Normalize(dueBefore), limit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("queue/db: due jobs: %w", err)
	}
	return ids, nil
}

// NextJob implements queue.WakeFinder.
func (q *Queue) NextJob(ctx context.Context, orderingKey string) (queue.Wake, bool, error) {
	var (
		w  queue.Wake
		ok bool
	)
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		w, ok, err = q.st.nextOfKey(ctx, tx, orderingKey)
		return err
	})
	if err != nil {
		return queue.Wake{}, false, fmt.Errorf("queue/db: next job: %w", err)
	}
	return w, ok, nil
}

// Complete implements queue.Source.
func (q *Queue) Complete(ctx context.Context, worker string, j queue.Job) error {
	return q.db.Tx(ctx, func(tx uow.Tx) error {
		n, err := q.st.deleteClaimed(ctx, tx, j.ID, worker, j.Attempt)
		return claimResult("complete", n, err)
	})
}

// Release implements queue.Source. The database has no delay limit, so runAt is used as is.
func (q *Queue) Release(ctx context.Context, worker string, j queue.Job, attempt int, runAt time.Time, lastErr string) error {
	return q.db.Tx(ctx, func(tx uow.Tx) error {
		n, err := q.st.release(ctx, tx, j.ID, worker, j.Attempt, attempt, kernel.Normalize(runAt), lastErr)
		return claimResult("release", n, err)
	})
}

// Bury implements queue.Source.
func (q *Queue) Bury(ctx context.Context, worker string, j queue.Job, lastErr string, failedAt time.Time) error {
	return q.db.Tx(ctx, func(tx uow.Tx) error {
		n, err := q.st.insertDeadFromClaim(ctx, tx, j.ID, worker, j.Attempt, lastErr, kernel.Normalize(failedAt))
		if err := claimResult("bury", n, err); err != nil {
			return err
		}
		n, err = q.st.deleteClaimed(ctx, tx, j.ID, worker, j.Attempt)
		return claimResult("bury", n, err)
	})
}

func claimResult(op string, rows int64, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("queue/db: %s: %w", op, err)
	case rows == 0:
		return fmt.Errorf("queue/db: %s: %w", op, queue.ErrLeaseLost)
	}
	return nil
}

// ListDeadJobs implements queue.DeadJobs.
func (q *Queue) ListDeadJobs(ctx context.Context, jobType string, after queue.DeadJobPosition, limit int) ([]queue.DeadJob, error) {
	if limit <= 0 {
		return nil, nil
	}
	var out []queue.DeadJob
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		out, err = q.st.listDead(ctx, tx, jobType, after, limit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("queue/db: list dead jobs: %w", err)
	}
	return out, nil
}

// RequeueDeadJob implements queue.DeadJobs.
func (q *Queue) RequeueDeadJob(ctx context.Context, id string) error {
	now := q.opts.Clock.Now()
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		if err := q.st.requeueDead(ctx, tx, id, now); err != nil {
			return err
		}
		n, err := q.st.deleteDead(ctx, tx, id)
		if err == nil && n == 0 {
			return queue.ErrDeadJobNotFound
		}
		return err
	})
	if err != nil && !errors.Is(err, queue.ErrDeadJobNotFound) {
		return fmt.Errorf("queue/db: requeue dead job: %w", err)
	}
	if err == nil {
		// When a pending job of the same idempotency key stood in for it, the signal finds
		// nothing to claim.
		queue.WakeJobs(ctx, q.opts.Waker, q.opts.Logger, queue.Wake{JobID: id, At: kernel.Normalize(now)})
	}
	return err
}

// DeleteDeadJob implements queue.DeadJobs.
func (q *Queue) DeleteDeadJob(ctx context.Context, id string) error {
	var n int64
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		n, err = q.st.deleteDead(ctx, tx, id)
		return err
	})
	switch {
	case err != nil:
		return fmt.Errorf("queue/db: delete dead job: %w", err)
	case n == 0:
		return queue.ErrDeadJobNotFound
	}
	return nil
}

// DeleteDeadJobsBefore implements queue.DeadJobs, in one short transaction.
func (q *Queue) DeleteDeadJobsBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	var n int64
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		n, err = q.st.deleteDeadBefore(ctx, tx, kernel.Normalize(before), limit)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("queue/db: delete dead jobs: %w", err)
	}
	return n, nil
}

// Backlog implements queue.BacklogReader.
func (q *Queue) Backlog(ctx context.Context, now time.Time) (queue.Backlog, error) {
	var b queue.Backlog
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		b, err = q.st.backlog(ctx, tx, kernel.Normalize(now))
		return err
	})
	if err != nil {
		return queue.Backlog{}, fmt.Errorf("queue/db: backlog: %w", err)
	}
	return b, nil
}

// ListJobs implements queue.JobInspector.
func (q *Queue) ListJobs(ctx context.Context, now time.Time, f queue.JobFilter, after queue.JobPosition, limit int) ([]queue.PendingJob, error) {
	if limit <= 0 || f.State == queue.JobStateDead {
		return nil, nil
	}
	var out []queue.PendingJob
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		out, err = q.st.listPending(ctx, tx, kernel.Normalize(now), f, queue.JobPosition{RunAt: kernel.Normalize(after.RunAt), ID: after.ID}, limit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("queue/db: list jobs: %w", err)
	}
	return out, nil
}

// SummarizeJobs implements queue.JobInspector.
func (q *Queue) SummarizeJobs(ctx context.Context, now time.Time) (queue.JobSummary, error) {
	var s queue.JobSummary
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		s, err = q.st.summarize(ctx, tx, kernel.Normalize(now))
		return err
	})
	if err != nil {
		return queue.JobSummary{}, fmt.Errorf("queue/db: summarize jobs: %w", err)
	}
	return s, nil
}

// pendingJob sets the state of j at now.
func pendingJob(now time.Time, j queue.PendingJob) queue.PendingJob {
	j.State = queue.StateAt(now, j.Attempt, j.LockedUntil)
	if j.State != queue.JobStateRunning {
		// An expired lease is no longer the job's: another worker may claim it.
		j.Worker, j.LockedUntil = "", time.Time{}
	}
	return j
}

func sortCounts(c []queue.JobCount) []queue.JobCount { return queue.SortJobCounts(c) }

// RecordAttempt implements queue.AttemptLog, in a transaction of its own. The records live in
// the application database whatever queue the server runs, so the server uses a Queue as the
// attempt log with every queue adapter (ADR-0064 §1).
func (q *Queue) RecordAttempt(ctx context.Context, a queue.Attempt) error {
	a.Error = queue.TruncateAttemptError(a.Error)
	a.StartedAt, a.FinishedAt = kernel.Normalize(a.StartedAt), kernel.Normalize(a.FinishedAt)
	err := q.db.Tx(ctx, func(tx uow.Tx) error { return q.st.insertAttempt(ctx, tx, a) })
	if err != nil {
		return fmt.Errorf("queue/db: record attempt: %w", err)
	}
	return nil
}

// ListAttempts implements queue.AttemptLog.
func (q *Queue) ListAttempts(ctx context.Context, jobID string, after queue.AttemptPosition, limit int) ([]queue.Attempt, error) {
	if limit <= 0 {
		return nil, nil
	}
	var out []queue.Attempt
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		out, err = q.st.listAttempts(ctx, tx, jobID, queue.AttemptPosition{FinishedAt: kernel.Normalize(after.FinishedAt), Attempt: after.Attempt}, limit)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("queue/db: list attempts: %w", err)
	}
	return out, nil
}

// DeleteAttemptsBefore implements queue.AttemptLog, in one short transaction.
func (q *Queue) DeleteAttemptsBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	var n int64
	err := q.db.Tx(ctx, func(tx uow.Tx) error {
		var err error
		n, err = q.st.deleteAttemptsBefore(ctx, tx, kernel.Normalize(before), limit)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("queue/db: delete attempts: %w", err)
	}
	return n, nil
}
