// SPDX-License-Identifier: AGPL-3.0-only

package inline

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
)

// Options configure a Queue. Clock and IDs are required.
type Options struct {
	Clock kernel.Clock
	IDs   kernel.IDGenerator
	// MaxDelay limits how far in the future a job becomes visible (0 for no limit).
	MaxDelay time.Duration
	// Tracer fills the TraceParent of the enqueued jobs from the span of the caller's context
	// (ADR-0063 §2); nil leaves it empty.
	Tracer kernel.Tracer
}

type entry struct {
	job queue.Job
	// visibleAt is when the job may be claimed: its RunAt, or earlier under MaxDelay.
	visibleAt   time.Time
	lockedBy    string
	lockedUntil time.Time
}

// Queue is an in-memory job queue. It is safe for concurrent use.
type Queue struct {
	opts Options

	mu    sync.Mutex
	jobs  map[string]*entry
	byKey map[string]string // idempotency key -> job ID
	dead  []queue.DeadJob

	runner   *queue.PullRunner
	draining bool
	again    bool
	// drainMu is held by the Enqueue that runs a drain, so that an Enqueue from another
	// goroutine waits for it and then runs its own jobs, instead of returning before they ran.
	drainMu sync.Mutex
}

// drainKey marks the context of a drain: an Enqueue from a handler of that drain joins it.
type drainKey struct{ q *Queue }

var (
	_ queue.JobQueue     = (*Queue)(nil)
	_ queue.Source       = (*Queue)(nil)
	_ queue.DeadJobs     = (*Queue)(nil)
	_ queue.JobInspector = (*Queue)(nil)
)

// New returns an empty queue.
func New(opts Options) *Queue {
	if opts.Clock == nil || opts.IDs == nil {
		panic("queue/inline: Options.Clock and Options.IDs are required")
	}
	return &Queue{opts: opts, jobs: map[string]*entry{}, byKey: map[string]string{}}
}

// RunSynchronously makes every later Enqueue run the due jobs with r (a runner of this queue)
// before it returns. Jobs enqueued by a handler during such a run join the same run; an Enqueue
// from another goroutine during it waits for the run to end and then runs the due jobs.
func (q *Queue) RunSynchronously(r *queue.PullRunner) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.runner = r
}

// Enqueue implements queue.JobQueue.
func (q *Queue) Enqueue(ctx context.Context, jobs ...queue.NewJob) error {
	now := q.opts.Clock.Now()
	built := make([]queue.Job, len(jobs))
	for i, n := range jobs {
		j, err := n.Build(q.opts.IDs.NewID(), now)
		if err != nil {
			return err
		}
		j.Payload = slices.Clone(j.Payload)
		built[i] = j
	}
	queue.TraceJobs(ctx, q.opts.Tracer, built)

	q.mu.Lock()
	for _, j := range built {
		if j.IdempotencyKey != "" {
			if _, dup := q.byKey[j.IdempotencyKey]; dup {
				continue
			}
			q.byKey[j.IdempotencyKey] = j.ID
		}
		q.jobs[j.ID] = &entry{job: j, visibleAt: queue.NextVisible(now, j.RunAt, q.opts.MaxDelay)}
	}
	if q.runner == nil {
		q.mu.Unlock()
		return nil
	}
	if q.draining && ctx.Value(drainKey{q}) != nil {
		q.again = true
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()
	// The jobs are enqueued: an error of the run (a claim, or the record of a result) is not
	// an error of Enqueue, which reports only jobs it did not enqueue. A job whose result was
	// not recorded runs again once its lease expires, as with any queue.
	_ = q.RunDue(ctx)
	return nil
}

// RunDue runs the due jobs with the runner of RunSynchronously, like an Enqueue of no job: it
// waits for a drain that another goroutine runs, and a handler's Enqueue joins it. It does
// nothing before RunSynchronously.
func (q *Queue) RunDue(ctx context.Context) error {
	q.mu.Lock()
	r := q.runner
	q.mu.Unlock()
	if r == nil {
		return nil
	}
	q.drainMu.Lock()
	defer q.drainMu.Unlock()
	q.mu.Lock()
	q.draining = true
	q.mu.Unlock()
	return q.drain(context.WithValue(ctx, drainKey{q}, true), r)
}

// RunUntil runs the due jobs with RunDue every interval (queue.DefaultPollInterval when not
// positive) until stop is closed or ctx ends, then returns nil: it runs the jobs that become
// due with no Enqueue after them, such as retries. Use it instead of the runner's own RunUntil,
// which would claim the jobs of an Enqueue outside its drain and let that Enqueue return before
// they ran. A pass in progress when stop is closed runs to its end.
func (q *Queue) RunUntil(ctx context.Context, stop <-chan struct{}, interval time.Duration) error {
	if interval <= 0 {
		interval = queue.DefaultPollInterval
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-stop:
			return nil
		case <-timer.C:
		}
		// The runner records the errors of single jobs; those of the queue itself are those
		// of its claims, which the next pass retries.
		_ = q.RunDue(ctx)
		timer.Reset(interval)
	}
}

// drain runs due jobs until a pass ends with no new enqueue in between.
func (q *Queue) drain(ctx context.Context, r *queue.PullRunner) error {
	for {
		_, err := r.Drain(ctx)
		q.mu.Lock()
		if err != nil || !q.again {
			q.draining, q.again = false, false
			q.mu.Unlock()
			return err
		}
		q.again = false
		q.mu.Unlock()
	}
}

// Claim implements queue.Source.
func (q *Queue) Claim(_ context.Context, worker string, now time.Time, lease time.Duration) (queue.Job, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var next *entry
	for _, e := range q.jobs {
		if e.visibleAt.After(now) || e.leased(now) || q.heldBack(e, now) {
			continue
		}
		if next == nil || e.visibleAt.Before(next.visibleAt) ||
			(e.visibleAt.Equal(next.visibleAt) && strings.Compare(e.job.ID, next.job.ID) < 0) {
			next = e
		}
	}
	if next == nil {
		return queue.Job{}, false, nil
	}
	next.lockedBy = worker
	next.lockedUntil = now.Add(lease)
	next.job.Attempt++
	j := next.job
	j.Payload = slices.Clone(j.Payload)
	return j, true, nil
}

func (e *entry) leased(now time.Time) bool { return e.lockedBy != "" && e.lockedUntil.After(now) }

// enqueuedBefore reports whether e comes before o in enqueue order, (CreatedAt, ID).
func (e *entry) enqueuedBefore(o *entry) bool {
	if c := e.job.CreatedAt.Compare(o.job.CreatedAt); c != 0 {
		return c < 0
	}
	return e.job.ID < o.job.ID
}

// heldBack reports whether the ordering key of e holds it back: another job of the key is
// leased or was enqueued before it.
func (q *Queue) heldBack(e *entry, now time.Time) bool {
	key := e.job.OrderingKey
	if key == "" {
		return false
	}
	for _, o := range q.jobs {
		if o != e && o.job.OrderingKey == key && (o.leased(now) || o.enqueuedBefore(e)) {
			return true
		}
	}
	return false
}

// claimed returns the entry of j while worker still holds its claim.
func (q *Queue) claimed(worker string, j queue.Job) (*entry, error) {
	e, ok := q.jobs[j.ID]
	if !ok || e.lockedBy != worker || e.job.Attempt != j.Attempt {
		return nil, queue.ErrLeaseLost
	}
	return e, nil
}

func (q *Queue) remove(e *entry) {
	delete(q.jobs, e.job.ID)
	if k := e.job.IdempotencyKey; k != "" && q.byKey[k] == e.job.ID {
		delete(q.byKey, k)
	}
}

// Complete implements queue.Source.
func (q *Queue) Complete(_ context.Context, worker string, j queue.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	e, err := q.claimed(worker, j)
	if err != nil {
		return err
	}
	q.remove(e)
	return nil
}

// Release implements queue.Source.
func (q *Queue) Release(_ context.Context, worker string, j queue.Job, attempt int, runAt time.Time, lastErr string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	e, err := q.claimed(worker, j)
	if err != nil {
		return err
	}
	runAt = kernel.Normalize(runAt)
	e.job.Attempt = attempt
	e.job.RunAt = runAt
	e.job.LastError = lastErr
	e.visibleAt = queue.NextVisible(q.opts.Clock.Now(), runAt, q.opts.MaxDelay)
	e.lockedBy, e.lockedUntil = "", time.Time{}
	return nil
}

// Bury implements queue.Source.
func (q *Queue) Bury(_ context.Context, worker string, j queue.Job, lastErr string, failedAt time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	e, err := q.claimed(worker, j)
	if err != nil {
		return err
	}
	q.remove(e)
	q.dead = append(q.dead, queue.DeadJob{
		ID:             e.job.ID,
		Type:           e.job.Type,
		Payload:        e.job.Payload,
		IdempotencyKey: e.job.IdempotencyKey,
		OrderingKey:    e.job.OrderingKey,
		TraceParent:    e.job.TraceParent,
		Attempt:        e.job.Attempt,
		LastError:      lastErr,
		CreatedAt:      e.job.CreatedAt,
		FailedAt:       kernel.Normalize(failedAt),
	})
	return nil
}

// ListDeadJobs implements queue.DeadJobs.
func (q *Queue) ListDeadJobs(_ context.Context, jobType string, after queue.DeadJobPosition, limit int) ([]queue.DeadJob, error) {
	if limit <= 0 {
		return nil, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	out := slices.Clone(q.dead)
	if jobType != "" {
		out = slices.DeleteFunc(out, func(d queue.DeadJob) bool { return d.Type != jobType })
	}
	slices.SortStableFunc(out, func(a, b queue.DeadJob) int {
		if c := b.FailedAt.Compare(a.FailedAt); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	if after.ID != "" {
		out = slices.DeleteFunc(out, func(d queue.DeadJob) bool {
			c := d.FailedAt.Compare(after.FailedAt)
			return c > 0 || (c == 0 && d.ID >= after.ID)
		})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// DeleteDeadJob implements queue.DeadJobs.
func (q *Queue) DeleteDeadJob(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	i := slices.IndexFunc(q.dead, func(d queue.DeadJob) bool { return d.ID == id })
	if i < 0 {
		return queue.ErrDeadJobNotFound
	}
	q.dead = slices.Delete(q.dead, i, i+1)
	return nil
}

// DeleteDeadJobsBefore implements queue.DeadJobs.
func (q *Queue) DeleteDeadJobsBefore(_ context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	old := make([]queue.DeadJob, 0)
	for _, d := range q.dead {
		if d.FailedAt.Before(before) {
			old = append(old, d)
		}
	}
	slices.SortStableFunc(old, func(a, b queue.DeadJob) int {
		if c := a.FailedAt.Compare(b.FailedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(old) > limit {
		old = old[:limit]
	}
	gone := make(map[string]bool, len(old))
	for _, d := range old {
		gone[d.ID] = true
	}
	q.dead = slices.DeleteFunc(q.dead, func(d queue.DeadJob) bool { return gone[d.ID] })
	return int64(len(old)), nil
}

// RequeueDeadJob implements queue.DeadJobs.
func (q *Queue) RequeueDeadJob(_ context.Context, id string) error {
	now := q.opts.Clock.Now()
	q.mu.Lock()
	defer q.mu.Unlock()
	i := slices.IndexFunc(q.dead, func(d queue.DeadJob) bool { return d.ID == id })
	if i < 0 {
		return queue.ErrDeadJobNotFound
	}
	d := q.dead[i]
	q.dead = slices.Delete(q.dead, i, i+1)
	if d.IdempotencyKey != "" {
		if _, dup := q.byKey[d.IdempotencyKey]; dup {
			return nil
		}
		q.byKey[d.IdempotencyKey] = d.ID
	}
	q.jobs[d.ID] = &entry{
		job: queue.Job{
			ID:             d.ID,
			Type:           d.Type,
			Payload:        d.Payload,
			IdempotencyKey: d.IdempotencyKey,
			OrderingKey:    d.OrderingKey,
			TraceParent:    d.TraceParent,
			RunAt:          now,
			CreatedAt:      d.CreatedAt,
		},
		visibleAt: now,
	}
	return nil
}

// pending returns the job of e as JobInspector shows it at now.
func (e *entry) pending(now time.Time) queue.PendingJob {
	j := queue.PendingJob{
		ID:             e.job.ID,
		Type:           e.job.Type,
		Payload:        slices.Clone(e.job.Payload),
		IdempotencyKey: e.job.IdempotencyKey,
		OrderingKey:    e.job.OrderingKey,
		State:          queue.StateAt(now, e.job.Attempt, e.lockedUntil),
		Attempt:        e.job.Attempt,
		RunAt:          e.job.RunAt,
		LastError:      e.job.LastError,
		CreatedAt:      e.job.CreatedAt,
	}
	if j.State == queue.JobStateRunning {
		j.Worker, j.LockedUntil = e.lockedBy, e.lockedUntil
	}
	return j
}

// ListJobs implements queue.JobInspector.
func (q *Queue) ListJobs(_ context.Context, now time.Time, f queue.JobFilter, after queue.JobPosition, limit int) ([]queue.PendingJob, error) {
	if limit <= 0 || f.State == queue.JobStateDead {
		return nil, nil
	}
	q.mu.Lock()
	out := make([]queue.PendingJob, 0, len(q.jobs))
	for _, e := range q.jobs {
		j := e.pending(now)
		if (f.State != "" && j.State != f.State) || (f.Type != "" && j.Type != f.Type) {
			continue
		}
		if after.ID != "" {
			if c := j.RunAt.Compare(after.RunAt); c < 0 || (c == 0 && j.ID <= after.ID) {
				continue
			}
		}
		out = append(out, j)
	}
	q.mu.Unlock()
	slices.SortFunc(out, func(a, b queue.PendingJob) int {
		if c := a.RunAt.Compare(b.RunAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// SummarizeJobs implements queue.JobInspector.
func (q *Queue) SummarizeJobs(_ context.Context, now time.Time) (queue.JobSummary, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	type key struct {
		state queue.JobState
		typ   string
	}
	counts := map[key]int64{}
	var s queue.JobSummary
	for _, e := range q.jobs {
		state := queue.StateAt(now, e.job.Attempt, e.lockedUntil)
		counts[key{state, e.job.Type}]++
		if state != queue.JobStateRunning && !e.job.RunAt.After(now) && (s.OldestReadyAt.IsZero() || e.job.RunAt.Before(s.OldestReadyAt)) {
			s.OldestReadyAt = e.job.RunAt
		}
	}
	for _, d := range q.dead {
		counts[key{queue.JobStateDead, d.Type}]++
	}
	s.Counts = make([]queue.JobCount, 0, len(counts))
	for k, n := range counts {
		s.Counts = append(s.Counts, queue.JobCount{State: k.state, Type: k.typ, Count: n})
	}
	s.Counts = queue.SortJobCounts(s.Counts)
	return s, nil
}
