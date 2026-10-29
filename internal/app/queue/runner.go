// SPDX-License-Identifier: AGPL-3.0-only

package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	randv2 "math/rand/v2"
	"runtime/debug"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
)

// JobRunner runs jobs until ctx ends. It has two forms (ADR-0004): a pull runner, which polls
// a queue (PullRunner, for the database queue and long-running SQS workers), and a push
// receiver, which handles the messages a platform delivers (Lambda SQS events, Cloud Tasks
// HTTP targets).
type JobRunner interface {
	// Run processes jobs until ctx ends, then returns nil. It returns an error only when it
	// cannot run at all.
	Run(ctx context.Context) error
}

// Source is the storage side of a pull queue, implemented by queue adapters and driven by
// PullRunner. Claims are leases: a claimed job is hidden from other workers until the lease
// expires, after which another worker may claim it again. That is how jobs survive a worker
// that stops while running them.
//
// A claim is identified by the job's ID, the worker, and the claimed Attempt. Complete,
// Release, and Bury act only while that claim still holds and return ErrLeaseLost otherwise.
type Source interface {
	// Claim leases the due job (RunAt <= now, not leased, not held back by its ordering key;
	// see "Ordering keys" in the package documentation) with the earliest RunAt to worker
	// until now+lease, increments its Attempt, and returns it. ok is false when no job is due.
	Claim(ctx context.Context, worker string, now time.Time, lease time.Duration) (j Job, ok bool, err error)
	// Complete removes the claimed job j.
	Complete(ctx context.Context, worker string, j Job) error
	// Release ends the claim of j and makes the job due again at runAt, with its Attempt set to
	// attempt and its LastError to lastErr. An adapter whose delays are limited makes the job
	// visible at NextVisible(now, runAt, limit) and keeps runAt as the job's RunAt.
	Release(ctx context.Context, worker string, j Job, attempt int, runAt time.Time, lastErr string) error
	// Bury moves the claimed job j to the dead jobs with lastErr, failed at failedAt.
	Bury(ctx context.Context, worker string, j Job, lastErr string, failedAt time.Time) error
	// ClaimJob leases the job id to worker until now+lease, like Claim but for one job named
	// by a wake-up signal (PushReceiver, ADR-0120 §2), and increments its Attempt. The result
	// tells why it did not: the job is not due before its RunAt (ClaimNotDue, with the job's
	// RunAt), an earlier job of its ordering key holds it back (ClaimHeldBack), or another
	// worker holds its lease or there is no such job, because it completed, became a dead job,
	// was never enqueued, or id is not a job ID (ClaimUnavailable).
	ClaimJob(ctx context.Context, id, worker string, now time.Time, lease time.Duration) (ClaimResult, error)
}

// ClaimOutcome is the outcome of Source.ClaimJob.
type ClaimOutcome int

const (
	// ClaimUnavailable: another worker holds the job's lease, or there is no such job.
	ClaimUnavailable ClaimOutcome = iota
	// Claimed: the worker holds the job's lease.
	Claimed
	// ClaimNotDue: the job's RunAt is after now.
	ClaimNotDue
	// ClaimHeldBack: the job's ordering key holds it back.
	ClaimHeldBack
)

// ClaimResult is the result of Source.ClaimJob. Job is the claimed job when Outcome is
// Claimed; when it is ClaimNotDue, only its ID and RunAt are set.
type ClaimResult struct {
	Outcome ClaimOutcome
	Job     Job
}

// Defaults applied to the zero fields of RunnerOptions.
const (
	DefaultLease         = 5 * time.Minute
	DefaultPollInterval  = time.Second
	DefaultRecordTimeout = 10 * time.Second
)

// RunnerOptions configure a PullRunner. Clock is required; the other zero fields take defaults.
type RunnerOptions struct {
	Clock kernel.Clock
	// Worker names this runner in claims. It must be unique among the runners of one queue;
	// the default is random.
	Worker string
	// Lease is how long a claim hides a job from other workers. A handler gets a context that
	// ends at the lease, counted from before the claim, so keep it longer than the slowest
	// handler.
	Lease time.Duration
	// RecordTimeout bounds the recording of a run's result (Complete, Release, Bury and the
	// attempt record). The record goes on after the runner's context ends, so that the job does
	// not wait for its lease to expire, but no longer than this.
	RecordTimeout time.Duration
	// PollInterval is how long Run waits after finding no due job.
	PollInterval time.Duration
	// Retry is the default policy of handlers registered without one.
	Retry RetryPolicy
	// Random returns numbers in [0, 1) for backoff jitter; the default is math/rand/v2.
	// Tests pass a fixed function for deterministic delays.
	Random func() float64
	// Logger receives errors that Run survives; the default is slog.Default().
	Logger *slog.Logger
	// Meter records my.job.duration and my.job.wait (ADR-0055 §3); nil records nothing.
	Meter kernel.Meter
	// Tracer records the span of each run (JobSpan, ADR-0063 §2); nil records nothing.
	Tracer kernel.Tracer
	// Attempts records the runs that did not succeed (ADR-0064 §1); nil records nothing. A
	// failure to record is logged and does not change the job's result.
	Attempts AttemptLog
	// Errors receives the panics of the handlers and the jobs that become dead (ADR-0103 §2),
	// except those of RetryRemote; nil reports nothing.
	Errors kernel.ErrorReporter
}

// PullRunner claims jobs from a Source, runs their handlers, and records the results. It
// implements JobRunner. Several PullRunners (in one or several processes) may share a Source.
type PullRunner struct {
	src     Source
	reg     *JobHandlerRegistry
	opts    RunnerOptions
	metrics jobMetrics
}

var _ JobRunner = (*PullRunner)(nil)

// NewPullRunner returns a runner of the jobs in src with the handlers in reg. It panics when
// opts.Clock is nil.
func NewPullRunner(src Source, reg *JobHandlerRegistry, opts RunnerOptions) *PullRunner {
	if opts.Clock == nil {
		panic("queue: RunnerOptions.Clock is required")
	}
	if opts.Worker == "" {
		var b [8]byte
		_, _ = rand.Read(b[:])
		opts.Worker = "worker-" + hex.EncodeToString(b[:])
	}
	if opts.Lease <= 0 {
		opts.Lease = DefaultLease
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = DefaultPollInterval
	}
	if opts.RecordTimeout <= 0 {
		opts.RecordTimeout = DefaultRecordTimeout
	}
	opts.Retry = opts.Retry.withDefaults()
	if opts.Random == nil {
		opts.Random = randv2.Float64
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Errors == nil {
		opts.Errors = kernel.NopErrorReporter{}
	}
	if reg == nil {
		reg = &JobHandlerRegistry{}
	}
	opts.Tracer = kernel.TracerOrNop(opts.Tracer)
	return &PullRunner{src: src, reg: reg, opts: opts, metrics: newJobMetrics(opts.Meter)}
}

// Worker returns the name this runner claims jobs under.
func (r *PullRunner) Worker() string { return r.opts.Worker }

// Run implements JobRunner: it runs due jobs one after another and polls every PollInterval
// while none is due. Errors of single jobs are logged, not returned.
func (r *PullRunner) Run(ctx context.Context) error {
	return r.RunUntil(ctx, ctx.Done())
}

// RunUntil runs jobs like Run until stop is closed or ctx ends, then returns nil. Closing stop
// is a graceful stop: the runner claims no more jobs, but the job in progress keeps ctx and
// runs to its end. Ending ctx also cancels the context of the job in progress.
func (r *PullRunner) RunUntil(ctx context.Context, stop <-chan struct{}) error {
	stopped := func() bool {
		select {
		case <-stop:
			return true
		default:
			return ctx.Err() != nil
		}
	}
	for {
		if stopped() {
			return nil
		}
		ran, err := r.RunOnce(ctx)
		if stopped() {
			return nil
		}
		if err != nil {
			r.opts.Logger.ErrorContext(ctx, "job runner", slog.String("worker", r.opts.Worker), slog.Any("error", err))
		}
		if ran && err == nil {
			continue
		}
		timer := time.NewTimer(r.opts.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-stop:
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// Drain runs due jobs until none is left and returns how many it claimed. Jobs that a handler
// retries become due later and are not run again by the same Drain unless the clock moves.
func (r *PullRunner) Drain(ctx context.Context) (int, error) {
	n := 0
	for {
		ran, err := r.RunOnce(ctx)
		if ran {
			n++
		}
		if err != nil || !ran {
			return n, err
		}
	}
}

// RunOnce claims at most one due job, runs it, and records the result. It reports whether it
// claimed a job. When the lease ran out while the handler ran and another worker claimed the
// job, the result is dropped and the error matches ErrLeaseLost.
func (r *PullRunner) RunOnce(ctx context.Context) (bool, error) {
	// The lease runs from now, before the claim: the handler's deadline is taken from the
	// monotonic clock first, so that it never ends after the lease the Source records.
	leaseEnd := time.Now().Add(r.opts.Lease)
	now := r.opts.Clock.Now()
	j, ok, err := r.src.Claim(ctx, r.opts.Worker, now, r.opts.Lease)
	if err != nil || !ok {
		return false, err
	}
	_, err = r.runClaimed(ctx, j, now, leaseEnd)
	return true, err
}

// settlement is how runClaimed recorded the result of a claimed job: released to run again at
// runAt, or settled for good (completed or buried).
type settlement struct {
	released bool
	runAt    time.Time
}

// runClaimed runs the job j that this runner claimed at now, with a handler context that ends
// at leaseEnd, and records the result: the part of a run that the pull runner and the push
// receiver share. When the lease ran out while the handler ran and another worker claimed the
// job, the result is dropped and the error matches ErrLeaseLost.
func (r *PullRunner) runClaimed(ctx context.Context, j Job, now, leaseEnd time.Time) (settlement, error) {
	// The result must be recorded even when ctx ended during the handler; otherwise the job
	// would wait for its lease to expire. RecordTimeout bounds it.
	recordCtx, cancelRecord := r.recordContext(ctx)
	defer func() { cancelRecord() }()

	if j.RunAt.After(now) {
		// Delivered early by a queue with a delay limit: re-enqueue without running (the
		// re-enqueue chain) and give the attempt back.
		return settlement{released: true, runAt: j.RunAt}, r.src.Release(recordCtx, r.opts.Worker, j, j.Attempt-1, j.RunAt, j.LastError)
	}

	handler, policy, found := r.reg.Lookup(j.Type)
	policy = policy.Or(r.opts.Retry)
	jobType := j.Type
	if !found {
		jobType = JobTypeOther // a type no handler knows is not a finite label value
	}
	// Each run is the root of a new trace, linked to the span that enqueued the job (ADR-0063 §2).
	tracer := r.opts.Tracer
	ctx, span := tracer.Start(tracer.Linked(ctx, j.TraceParent), JobSpan.Named(jobType),
		kernel.NewAttrs(kernel.StringAttr(LabelJobType, jobType), kernel.IntAttr(AttrJobAttempt, int64(j.Attempt))))
	mctx, end := r.metrics.meter.Begin(ctx)
	defer end()
	wait := max(now.Sub(j.RunAt), 0)
	r.metrics.waited(mctx, jobType, wait)
	started := time.Now() // the monotonic clock: Clock.Now has millisecond precision
	var res Result
	if found {
		res = r.call(mctx, handler, j, jobType, leaseEnd)
	} else {
		res = Retry(0, fmt.Errorf("%w: %s", ErrNoHandler, j.Type))
	}
	// The record's deadline starts once the handler returned, under the span of the run.
	cancelRecord()
	recordCtx, cancelRecord = r.recordContext(ctx)

	done := r.opts.Clock.Now()
	ran := func(outcome string) {
		d := time.Since(started)
		r.metrics.ran(mctx, jobType, outcome, d)
		r.logRun(mctx, j, jobType, outcome, res.Err(), d, wait)
		span.Set(kernel.StringAttr(LabelOutcome, outcome))
	}
	// The span ends once the result is recorded, so that it covers the result's transaction.
	defer func() {
		errorType := ""
		if !res.IsDone() {
			if errorType = kernel.ErrorType(res.Err()); errorType == "" {
				errorType = "_OTHER"
			}
		}
		span.End(errorType)
	}()
	switch {
	case res.IsDone():
		ran(OutcomeSuccess)
		return settlement{}, r.src.Complete(recordCtx, r.opts.Worker, j)
	case res.IsFail():
		ran(OutcomeDead)
		lastErr := errorText(res.Err(), "failed")
		r.reportDead(recordCtx, j, jobType, res)
		r.recordAttempt(recordCtx, j, OutcomeDead, lastErr, now, done)
		return settlement{}, r.src.Bury(recordCtx, r.opts.Worker, j, lastErr, done)
	case j.Attempt >= policy.MaxAttempts:
		ran(OutcomeDead)
		lastErr := errorText(res.Err(), "retried") + fmt.Sprintf(" (gave up after %d attempts)", j.Attempt)
		r.reportDead(recordCtx, j, jobType, res)
		r.recordAttempt(recordCtx, j, OutcomeDead, lastErr, now, done)
		return settlement{}, r.src.Bury(recordCtx, r.opts.Worker, j, lastErr, done)
	default:
		ran(OutcomeRetry)
		after := res.After()
		if after <= 0 {
			after = policy.Backoff(j.Attempt, r.opts.Random)
		}
		lastErr := errorText(res.Err(), "retried")
		r.recordAttempt(recordCtx, j, OutcomeRetry, lastErr, now, done)
		runAt := done.Add(after)
		return settlement{released: true, runAt: runAt}, r.src.Release(recordCtx, r.opts.Worker, j, j.Attempt, runAt, lastErr)
	}
}

// recordAttempt stores the record of a run that did not succeed (ADR-0064 §1). The record is
// written before the result, so that a job listed as retrying or dead has the record of its
// last run; a failure to write it is logged and leaves the result alone.
func (r *PullRunner) recordAttempt(ctx context.Context, j Job, outcome, lastErr string, started, finished time.Time) {
	if r.opts.Attempts == nil {
		return
	}
	err := r.opts.Attempts.RecordAttempt(ctx, Attempt{
		JobID: j.ID, JobType: j.Type, Attempt: j.Attempt, Outcome: outcome, Error: TruncateAttemptError(lastErr),
		Worker: r.opts.Worker, StartedAt: kernel.Normalize(started), FinishedAt: kernel.Normalize(finished),
	})
	if err != nil {
		r.opts.Logger.ErrorContext(ctx, "job runner: record attempt", slog.String("worker", r.opts.Worker),
			slog.String(logJobID, j.ID), slog.Int(logAttempt, j.Attempt), slog.Any("error", err))
	}
}

// Keys of the job log that are not labels of the job metrics (ADR-0055 §5).
const (
	logJobID    = "job_id"
	logAttempt  = "attempt"
	logDuration = "duration_seconds"
	logWait     = "wait_seconds"
)

// logRun writes the one line of a job run (ADR-0055 §5): the labels of my.job.duration under
// the same names and values, its duration and the my.job.wait in seconds, so that a log-based
// metric counts the runs as the metrics do, and the job's ID and attempt for NFR-OPS-1. With
// metrics_exporter = emf, the line also carries those two metrics. A success is info, a retry a
// warning, and a dead job an error.
func (r *PullRunner) logRun(ctx context.Context, j Job, jobType, outcome string, err error, d, wait time.Duration) {
	level := slog.LevelInfo
	switch outcome {
	case OutcomeRetry:
		level = slog.LevelWarn
	case OutcomeDead:
		level = slog.LevelError
	}
	attrs := []slog.Attr{
		slog.String(LabelJobType, jobType),
		slog.String(LabelOutcome, outcome),
		slog.Float64(logDuration, d.Seconds()),
		slog.Float64(logWait, wait.Seconds()),
		slog.String(logJobID, j.ID),
		slog.Int(logAttempt, j.Attempt),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	kernel.LogUnit(ctx, r.opts.Logger, level, "job", r.metrics.meter, attrs, JobDuration, JobWait)
}

// reportDead reports the job j of jobType that becomes dead with res (ADR-0103 §2), unless a
// remote server caused it (RetryRemote).
func (r *PullRunner) reportDead(ctx context.Context, j Job, jobType string, res Result) {
	if res.IsRemote() {
		return
	}
	err := res.Err()
	if err == nil {
		err = errDeadWithoutCause
	}
	ev := kernel.NewErrorEvent(err, jobType)
	ev.JobID, ev.Attempt = j.ID, j.Attempt
	r.opts.Errors.Report(ctx, ev)
}

// errDeadWithoutCause stands for the cause of a dead job whose handler gave none.
var errDeadWithoutCause = errors.New("queue: the job failed without an error")

// recordContext returns the context that records a result: ctx without its cancellation,
// bounded by RecordTimeout.
func (r *PullRunner) recordContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), r.opts.RecordTimeout)
}

// call runs h with a context that ends at leaseEnd, the end of the job's lease, turning a panic
// into a retry after reporting it at the job's type (ADR-0103 §2).
func (r *PullRunner) call(ctx context.Context, h Handler, j Job, jobType string, leaseEnd time.Time) (res Result) {
	ctx, cancel := context.WithDeadline(ctx, leaseEnd)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			ev := kernel.PanicEvent(p, debug.Stack(), jobType)
			ev.JobID, ev.Attempt = j.ID, j.Attempt
			r.opts.Errors.Report(ctx, ev)
			res = Retry(0, fmt.Errorf("handler panicked: %v", p))
		}
	}()
	return h(ctx, j)
}

func errorText(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	return err.Error()
}
