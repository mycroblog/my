// SPDX-License-Identifier: AGPL-3.0-only

// Package queue defines the job queue ports (ADR-0004) and the parts every queue adapter
// shares: the job model, the handler registry, the retry policy (exponential backoff with
// jitter), and the pull runner.
//
// Delivery is at-least-once: a job may run more than once (a worker can stop after running a
// handler and before recording the result), so handlers must be idempotent.
//
// # Ordering keys
//
// Jobs with the same non-empty OrderingKey run one at a time, in enqueue order. A Source hands
// out a job of a key only when no other job of that key is leased and no job of that key
// enqueued before it is still in the queue, whether due, waiting for its RunAt, waiting to be
// retried, or leased. Enqueue order is (CreatedAt, ID): the IDs are UUIDv7s, ordered within a
// millisecond by the generating process, so the jobs one process enqueues keep their order,
// while the order of jobs that different processes enqueue in the same millisecond is
// arbitrary. A dead job no longer holds back its key, and a requeued dead job keeps its
// CreatedAt and ID, so it runs before the later jobs of its key that have not started.
//
// So a job waits behind every earlier job of its key, including one delayed by RunAt or one
// backing off between retries, until that job completes or becomes a dead job: one failing job
// holds back its key for up to its retry policy's attempts. Choose keys as narrow as the order
// that matters, for example a delivery's target inbox rather than the whole remote server.
// Jobs with different keys, or without a key, do not wait for each other.
//
// The mutual exclusion lasts as long as the lease: when a handler is still running after its
// lease expired, the job may run again (at-least-once) and later jobs of its key may start.
// Handlers get a context that ends with the lease, so a handler that returns when it ends keeps
// the guarantee.
//
// The adapters live in internal/adapter/queue/*; their shared contract test suite is
// testkit.QueueContract in pkg/testkit.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
)

// Job is a unit of asynchronous work, as a handler receives it.
type Job struct {
	// ID is a UUIDv7.
	ID string
	// Type selects the handler, for example "federation.deliver".
	Type string
	// Payload is a small JSON document; large data stays in the database or blob storage and
	// the payload refers to it by ID. Adapters may reformat it (whitespace, key order) but
	// keep its meaning.
	Payload json.RawMessage
	// IdempotencyKey, when not empty, is unique among the jobs not yet completed: enqueuing a
	// job with the key of such a job does nothing.
	IdempotencyKey string
	// OrderingKey, when not empty, serializes the jobs that share it (for example the inbox
	// of one remote actor, or the activities about one object); see "Ordering keys" in the
	// package documentation.
	OrderingKey string
	// Attempt is the number of this run, from 1. A run counts as soon as a worker claims the
	// job, so runs cut short by a crash count too.
	Attempt int
	// RunAt is the earliest time the job may run.
	RunAt time.Time
	// LastError describes why the previous attempt did not finish, or is empty.
	LastError string
	// TraceParent is the W3C traceparent of the span that enqueued the job, or empty: the span
	// of each run is the root of a new trace with a link to it (ADR-0063 §2).
	TraceParent string
	CreatedAt   time.Time
}

// NewJob describes a job to enqueue.
type NewJob struct {
	Type           string
	Payload        json.RawMessage
	IdempotencyKey string
	OrderingKey    string
	// RunAt delays the job; the zero value means now. The job runs no earlier than RunAt;
	// adapters do not guarantee how much later.
	RunAt time.Time
	// TraceParent is the traceparent of the job (Job.TraceParent). Callers leave it empty: the
	// adapter fills it from the span of the context of Enqueue or EnqueueTx (TraceJobs). The
	// outbox relay passes on the one it stored.
	TraceParent string
}

// Build validates n and turns it into the Job that adapters store, with the given ID, the
// creation time now, and RunAt defaulting to now. A nil payload becomes JSON null.
func (n NewJob) Build(id string, now time.Time) (Job, error) {
	if n.Type == "" {
		return Job{}, errors.New("queue: job type is empty")
	}
	payload := n.Payload
	if payload == nil {
		payload = json.RawMessage("null")
	}
	if !json.Valid(payload) {
		return Job{}, fmt.Errorf("queue: payload of %s job is not valid JSON", n.Type)
	}
	now = kernel.Normalize(now)
	runAt := now
	if !n.RunAt.IsZero() {
		runAt = kernel.Normalize(n.RunAt)
	}
	return Job{
		ID:             id,
		Type:           n.Type,
		Payload:        payload,
		IdempotencyKey: n.IdempotencyKey,
		OrderingKey:    n.OrderingKey,
		RunAt:          runAt,
		TraceParent:    n.TraceParent,
		CreatedAt:      now,
	}, nil
}

// TraceJobs sets the TraceParent of the jobs that have none to the span of ctx (Tracer.Inject),
// for the adapters to call in Enqueue and EnqueueTx (ADR-0063 §2). A nil tracer, or a span that
// is not recorded, leaves them empty.
func TraceJobs(ctx context.Context, t kernel.Tracer, jobs []Job) {
	if kernel.IsNopTracer(t) {
		return
	}
	tp := t.Inject(ctx)
	if tp == "" {
		return
	}
	for i := range jobs {
		if jobs[i].TraceParent == "" {
			jobs[i].TraceParent = tp
		}
	}
}

// JobQueue enqueues jobs. Enqueue is external I/O: never call it inside UnitOfWork.Tx; use
// Outbox there instead (ADR-0003).
type JobQueue interface {
	// Enqueue adds jobs atomically: either all of them are enqueued or, on error, none. Jobs
	// whose IdempotencyKey matches a job not yet completed, or an earlier job of the same
	// call, are skipped. An adapter that also runs the jobs before returning (the inline
	// queue) does not report the errors of that run: the jobs are enqueued by then. Like a
	// commit, a call whose connection fails may leave the outcome unknown; repeating it with
	// the same idempotency keys is safe.
	Enqueue(ctx context.Context, jobs ...NewJob) error
}

// Outbox enqueues jobs as part of a database transaction (Transactional Outbox, ADR-0004):
// the jobs exist exactly when the transaction commits, together with the domain writes made
// through the same tx.
//
// An implementation either writes the jobs table of the database queue directly or writes
// outbox_events, which a relay later moves into the configured JobQueue.
type Outbox interface {
	// EnqueueTx records jobs in tx, with the same deduplication as JobQueue.Enqueue (for
	// outbox_events, deduplication happens when the relay enqueues them), also between
	// concurrent transactions. It is safe to repeat when UnitOfWork.Tx retries the
	// transaction.
	//
	// A job takes its place in enqueue order when EnqueueTx builds it, but no worker sees it
	// before its transaction commits. Jobs of one ordering key that concurrent transactions
	// enqueue still run one at a time, but a job whose transaction commits after a later job
	// of its key was claimed runs after that job. Commands that need the order of their jobs
	// across transactions serialize the transactions themselves, for example by updating one
	// row of the entity the jobs are about.
	EnqueueTx(ctx context.Context, tx uow.Tx, jobs ...NewJob) error
}

// DeadJob is a job that failed for good: its handler returned Fail, or it used up its
// attempts. Dead jobs stay until an operator requeues them or the dead job purge deletes them
// after their retention (DeadJobPurger, FR-803).
type DeadJob struct {
	ID             string
	Type           string
	Payload        json.RawMessage
	IdempotencyKey string
	OrderingKey    string
	// Attempt is the number of runs the job had.
	Attempt   int
	LastError string
	// TraceParent is the job's Job.TraceParent, which a requeued job keeps.
	TraceParent string
	CreatedAt   time.Time
	FailedAt    time.Time
}

// Position returns the position of d in the order of DeadJobs.ListDeadJobs.
func (d DeadJob) Position() DeadJobPosition { return DeadJobPosition{FailedAt: d.FailedAt, ID: d.ID} }

// DeadJobPosition is a position in the order of DeadJobs.ListDeadJobs: most recently failed
// first, and by descending ID among the jobs that failed at the same time. The zero value is
// the position before the first dead job.
type DeadJobPosition struct {
	FailedAt time.Time
	ID       string
}

// DeadJobs gives the admin API and the CLI access to dead jobs.
type DeadJobs interface {
	// ListDeadJobs returns up to limit dead jobs of the type jobType (every type when empty),
	// most recently failed first, starting after the position after (the zero value for the
	// first); none when limit <= 0.
	ListDeadJobs(ctx context.Context, jobType string, after DeadJobPosition, limit int) ([]DeadJob, error)
	// RequeueDeadJob enqueues the dead job id again with a fresh attempt count, to run now,
	// and removes it from the dead jobs. When a job with the same IdempotencyKey is pending,
	// that job stands in for it. It returns ErrDeadJobNotFound when there is no such dead job.
	RequeueDeadJob(ctx context.Context, id string) error
	// DeleteDeadJob deletes the dead job id without running it again. It returns
	// ErrDeadJobNotFound when there is no such dead job.
	DeleteDeadJob(ctx context.Context, id string) error
	// DeleteDeadJobsBefore deletes at most limit dead jobs that failed before t, the oldest
	// first, and returns how many it deleted; none when limit <= 0. It is safe to repeat.
	DeleteDeadJobsBefore(ctx context.Context, t time.Time, limit int) (int64, error)
}

var (
	// ErrDeadJobNotFound is returned by DeadJobs.RequeueDeadJob and DeadJobs.DeleteDeadJob
	// for an unknown ID.
	ErrDeadJobNotFound = errors.New("queue: dead job not found")
	// ErrLeaseLost is returned by Source methods when the worker no longer holds the job:
	// its lease expired and another worker claimed it. The other worker's result wins.
	ErrLeaseLost = errors.New("queue: job lease lost")
	// ErrNoHandler is recorded as the error of a job whose type has no registered handler.
	// Such a job is retried, in case a newer worker that knows the type picks it up.
	ErrNoHandler = errors.New("queue: no handler registered for job type")
)

// Backlog is the state of the jobs of a queue at one time, for the queue gauges of the
// metrics snapshot (ADR-0055 §3, §4).
type Backlog struct {
	// Ready counts the due jobs that no worker holds, by job type.
	Ready map[string]int64
	// OldestReadyAt is the earliest due time among the ready jobs; zero when none is ready.
	OldestReadyAt time.Time
	// Dead counts the dead jobs, as DeadJobs lists them.
	Dead int64
}

// BacklogReader reads the backlog of a queue that the server can count: the database queue.
// The queues of the platforms (Cloud Tasks, SQS) have metrics of their own. Adapters pass
// testkit.QueueBacklogContract.
type BacklogReader interface {
	Backlog(ctx context.Context, now time.Time) (Backlog, error)
}
