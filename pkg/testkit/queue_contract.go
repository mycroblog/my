// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// QueueHarness is one job queue adapter under test, on its own empty storage.
type QueueHarness struct {
	// Queue, Source, and DeadJobs are the adapter's views of the same queue.
	Queue    queue.JobQueue
	Source   queue.Source
	DeadJobs queue.DeadJobs
	// UnitOfWork and Outbox, when both set, enable the transactional enqueue cases: jobs
	// enqueued through Outbox in a UnitOfWork transaction must exist exactly when it commits.
	UnitOfWork uow.UnitOfWork
	Outbox     queue.Outbox
	// Relay, when set, moves the jobs written through Outbox into Queue. The contract calls it
	// before running jobs.
	Relay func(ctx context.Context) error
	// Restart, when set, simulates a process restart: it drops every connection and in-memory
	// state of the adapter and returns a harness on the same storage. Durable adapters set it.
	Restart func(t *testing.T) QueueHarness
	// MaxDelay is the adapter's delay limit, or 0 for none. With a limit, the contract checks
	// the re-enqueue chain.
	MaxDelay time.Duration
	// Inspector, when set, is the adapter's JobInspector, and the contract checks the listing
	// and the counts of its jobs (ADR-0064 §3).
	Inspector queue.JobInspector
	// Attempts, when set, is an AttemptLog that the contract's runners record into, and the
	// contract checks the records (ADR-0064 §1).
	Attempts queue.AttemptLog
}

// QueueFactory returns a harness on new, empty storage whose adapter reads time from clock.
// The factory registers the cleanup of everything it creates with t.Cleanup.
type QueueFactory func(t *testing.T, clock *Clock) QueueHarness

// The contract's runner settings. With FixedRandom(0.5) and Jitter 0.5, the backoff after
// failed run n is BaseDelay * 2^(n-1) * 0.75.
var (
	contractStart  = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	contractLease  = time.Minute
	contractPolicy = queue.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Minute, MaxDelay: time.Hour, Jitter: 0.5}
)

// FixedRandom returns a random source for queue.RunnerOptions.Random that always returns v,
// which makes backoff jitter deterministic.
func FixedRandom(v float64) func() float64 { return func() float64 { return v } }

// QueueContract verifies that the adapter built by newHarness satisfies the job queue
// contract, driven by queue.PullRunner: due jobs run in RunAt order and delayed jobs not
// before RunAt; idempotency keys deduplicate pending jobs; failed runs are retried with
// exponential backoff with jitter or after the requested delay; jobs that fail for good or
// use up their attempts become dead jobs, which are listed in pages, can be requeued or deleted one by one, and are
// deleted, the oldest first in bounded batches, once they failed before a given time; a job whose worker stopped
// is claimed again once its lease expires (at-least-once), and the stale worker's result is
// dropped; concurrent workers run each job once; jobs of one ordering key run one at a time,
// in enqueue order, even across retries, delays, and concurrent workers; a job keeps its TraceParent
// (ADR-0063 §2) through runs, retries, the dead jobs, and a requeue; and, where the harness enables them,
// transactional enqueue, survival of a restart, and the re-enqueue chain beyond a delay limit, and
// the listing of the jobs and the records of the attempts for the queue dashboard.
func QueueContract(t *testing.T, newHarness QueueFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, newHarness QueueFactory)
	}{
		{"RunsDueJobsInRunAtOrder", queueRunsDueJobsInOrder},
		{"RejectsInvalidJobsAtomically", queueRejectsInvalidJobs},
		{"DeduplicatesPendingJobsByIdempotencyKey", queueDeduplicates},
		{"RetriesWithExponentialBackoffAndJitter", queueRetriesWithBackoff},
		{"RetriesAfterTheRequestedDelay", queueRetriesAfterDelay},
		{"RetriesPanickingHandlers", queueRetriesPanics},
		{"MovesFailedJobsToDeadJobs", queueMovesFailedJobsToDead},
		{"MovesExhaustedJobsToDeadJobs", queueMovesExhaustedJobsToDead},
		{"RetriesJobsWithoutAHandler", queueRetriesUnknownTypes},
		{"RequeuesDeadJobs", queueRequeuesDeadJobs},
		{"DeletesDeadJobs", queueDeletesDeadJobs},
		{"PagesDeadJobsInFailureOrder", queuePagesDeadJobs},
		{"DeletesDeadJobsPastTheirRetention", queueDeletesOldDeadJobs},
		{"ReclaimsJobsOfAStoppedWorker", queueReclaimsAfterLeaseExpiry},
		{"RunsEachJobOnceUnderConcurrentWorkers", queueConcurrentWorkers},
		{"LeasesOneJobPerOrderingKeyAtATime", queueLeasesOnePerOrderingKey},
		{"KeepsEnqueueOrderWithinAnOrderingKey", queueKeepsOrderWithinKey},
		{"SerializesOrderingKeysUnderConcurrentWorkers", queueSerializesKeysUnderConcurrency},
		{"EnqueuesAtomicallyWithTheTransaction", queueTransactionalEnqueue},
		{"KeepsTheTraceParent", queueKeepsTraceParent},
		{"KeepsJobsAcrossARestart", queueSurvivesRestart},
		{"ReenqueuesJobsBeyondTheDelayLimit", queueReenqueueChain},
		{"InspectsJobsByState", queueInspectsJobsByState},
		{"RecordsUnsuccessfulAttempts", queueRecordsUnsuccessfulAttempts},
		{"EnqueuesFromConcurrentTransactions", queueConcurrentTransactions},
		{"RefusesARepeatedResult", queueRefusesRepeatedResults},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newHarness) })
	}
}

// recorder is a handler that records every run and answers its n-th run with results[n-1],
// or with Done when results runs out.
type recorder struct {
	mu      sync.Mutex
	runs    []queue.Job
	results []queue.Result
}

func (r *recorder) handle(_ context.Context, j queue.Job) queue.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, j)
	if n := len(r.runs); n <= len(r.results) {
		return r.results[n-1]
	}
	return queue.Done()
}

func (r *recorder) snapshot() []queue.Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.runs)
}

type queueFixture struct {
	t     *testing.T
	h     QueueHarness
	clock *Clock
	reg   *queue.JobHandlerRegistry
	rec   *recorder
}

const contractJobType = "testkit.probe"

func newQueueFixture(t *testing.T, newHarness QueueFactory, results ...queue.Result) *queueFixture {
	t.Helper()
	clock := NewClock(contractStart)
	f := &queueFixture{t: t, h: newHarness(t, clock), clock: clock, reg: &queue.JobHandlerRegistry{}, rec: &recorder{results: results}}
	f.reg.Register(contractJobType, f.rec.handle)
	return f
}

func (f *queueFixture) runner(worker string) *queue.PullRunner {
	return queue.NewPullRunner(f.h.Source, f.reg, queue.RunnerOptions{
		Clock:    f.clock,
		Worker:   worker,
		Lease:    contractLease,
		Retry:    contractPolicy,
		Random:   FixedRandom(0.5),
		Attempts: f.h.Attempts,
	})
}

func (f *queueFixture) enqueue(jobs ...queue.NewJob) {
	f.t.Helper()
	if err := f.h.Queue.Enqueue(f.t.Context(), jobs...); err != nil {
		f.t.Fatalf("Enqueue: %v", err)
	}
}

// drain relays the outbox, when the harness has one, and runs every due job with a runner
// named "w1". It returns the number of jobs claimed.
func (f *queueFixture) drain() int {
	f.t.Helper()
	if f.h.Relay != nil {
		if err := f.h.Relay(f.t.Context()); err != nil {
			f.t.Fatalf("Relay: %v", err)
		}
	}
	n, err := f.runner("w1").Drain(f.t.Context())
	if err != nil {
		f.t.Fatalf("Drain: %v", err)
	}
	return n
}

func (f *queueFixture) wantRuns(n int) []queue.Job {
	f.t.Helper()
	runs := f.rec.snapshot()
	if len(runs) != n {
		f.t.Fatalf("handler ran %d times, want %d: %v", len(runs), n, runs)
	}
	return runs
}

func (f *queueFixture) dead() []queue.DeadJob {
	f.t.Helper()
	d, err := f.h.DeadJobs.ListDeadJobs(f.t.Context(), "", queue.DeadJobPosition{}, 100)
	if err != nil {
		f.t.Fatalf("ListDeadJobs: %v", err)
	}
	return d
}

func probe(payload string) queue.NewJob {
	return queue.NewJob{Type: contractJobType, Payload: json.RawMessage(payload)}
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func payloadOf(t *testing.T, j queue.Job) string {
	t.Helper()
	var v struct{ N string }
	if err := json.Unmarshal(j.Payload, &v); err != nil {
		t.Fatalf("payload %s: %v", j.Payload, err)
	}
	return v.N
}

func queueRunsDueJobsInOrder(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	// The payload round-trips whatever a JSON column refuses, such as an escaped NUL.
	later := probe(`{"N": "later", "nested": {"b": [1, 2], "a": null, "s": "nul \u0000"}}`)
	later.RunAt = contractStart.Add(10 * time.Minute)
	later.IdempotencyKey = "key-later"
	later.OrderingKey = "order-1"
	first := probe(`{"N":"first"}`)
	first.RunAt = contractStart.Add(-time.Minute)
	f.enqueue(later, probe(`{"N":"now"}`), first)

	if n := f.drain(); n != 2 {
		t.Fatalf("first drain claimed %d jobs, want 2", n)
	}
	runs := f.wantRuns(2)
	if got := []string{payloadOf(t, runs[0]), payloadOf(t, runs[1])}; !slices.Equal(got, []string{"first", "now"}) {
		t.Fatalf("ran %v, want [first now] (RunAt order)", got)
	}
	if !runs[1].RunAt.Equal(contractStart) || runs[1].Attempt != 1 || runs[1].Type != contractJobType || runs[1].ID == "" {
		t.Errorf("job = %+v, want RunAt %v, Attempt 1, Type %s, and an ID", runs[1], contractStart, contractJobType)
	}

	f.clock.Advance(10*time.Minute - time.Millisecond)
	if n := f.drain(); n != 0 {
		t.Fatalf("drain before RunAt claimed %d jobs, want 0", n)
	}
	f.clock.Advance(time.Millisecond)
	if n := f.drain(); n != 1 {
		t.Fatalf("drain at RunAt claimed %d jobs, want 1", n)
	}
	j := f.wantRuns(3)[2]
	if !sameJSON(j.Payload, later.Payload) {
		t.Errorf("payload = %s, want %s", j.Payload, later.Payload)
	}
	if j.IdempotencyKey != "key-later" || j.OrderingKey != "order-1" || !j.RunAt.Equal(later.RunAt) || !j.CreatedAt.Equal(contractStart) {
		t.Errorf("job = %+v, want the enqueued keys, RunAt %v, and CreatedAt %v", j, later.RunAt, contractStart)
	}
	if n := f.drain(); n != 0 {
		t.Fatalf("completed jobs ran again: drain claimed %d", n)
	}
}

func queueRejectsInvalidJobs(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	for _, bad := range []queue.NewJob{
		{Type: "", Payload: json.RawMessage(`{}`)},
		{Type: contractJobType, Payload: json.RawMessage(`{"unterminated"`)},
	} {
		if err := f.h.Queue.Enqueue(t.Context(), probe(`{"N":"ok"}`), bad); err == nil {
			t.Errorf("Enqueue(%+v) succeeded, want an error", bad)
		}
	}
	f.drain()
	f.wantRuns(0)

	f.enqueue(queue.NewJob{Type: contractJobType})
	f.drain()
	if j := f.wantRuns(1)[0]; string(j.Payload) != "null" {
		t.Errorf("nil payload stored as %s, want null", j.Payload)
	}
}

func queueDeduplicates(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	keyed := func(n, key string) queue.NewJob {
		j := probe(fmt.Sprintf(`{"N":%q}`, n))
		j.IdempotencyKey = key
		return j
	}
	f.enqueue(keyed("a1", "a"), keyed("a2", "a"), keyed("b1", "b"))
	f.enqueue(keyed("a3", "a"), probe(`{"N":"plain"}`), probe(`{"N":"plain"}`))
	f.drain()
	var got []string
	for _, j := range f.wantRuns(4) {
		got = append(got, payloadOf(t, j))
	}
	slices.Sort(got)
	if want := []string{"a1", "b1", "plain", "plain"}; !slices.Equal(got, want) {
		t.Fatalf("ran %v, want %v (one job per pending key, every unkeyed job)", got, want)
	}

	// The key is unique among pending jobs only: once its job finished, it may be used again.
	f.enqueue(keyed("a4", "a"))
	f.drain()
	if j := f.wantRuns(5)[4]; payloadOf(t, j) != "a4" {
		t.Fatalf("ran %s, want a4", j.Payload)
	}
}

func queueRetriesWithBackoff(t *testing.T, newHarness QueueFactory) {
	boom := errors.New("temporarily unavailable")
	f := newQueueFixture(t, newHarness, queue.Retry(0, boom), queue.Retry(0, boom))
	f.enqueue(probe(`{"N":"x"}`))

	// Backoffs: 1m * 0.75 = 45s after run 1, 2m * 0.75 = 90s after run 2.
	for i, wait := range []time.Duration{0, 45 * time.Second, 90 * time.Second} {
		if wait > 0 {
			f.clock.Advance(wait - time.Millisecond)
			if n := f.drain(); n != 0 {
				t.Fatalf("run %d started %v early", i+1, time.Millisecond)
			}
			f.clock.Advance(time.Millisecond)
		}
		if n := f.drain(); n != 1 {
			t.Fatalf("run %d: drain claimed %d jobs, want 1", i+1, n)
		}
	}
	runs := f.wantRuns(3)
	for i, j := range runs {
		if j.Attempt != i+1 {
			t.Errorf("run %d has Attempt %d", i+1, j.Attempt)
		}
	}
	if runs[0].LastError != "" || runs[1].LastError != boom.Error() {
		t.Errorf("LastError = %q, %q, want empty, then %q", runs[0].LastError, runs[1].LastError, boom)
	}
	if d := f.dead(); len(d) != 0 {
		t.Errorf("dead jobs = %v, want none", d)
	}
}

func queueRetriesAfterDelay(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness, queue.Retry(7*time.Hour, nil))
	f.enqueue(probe(`{"N":"x"}`))
	f.drain()
	f.clock.Advance(7*time.Hour - time.Millisecond)
	f.drain()
	f.wantRuns(1)
	f.clock.Advance(time.Millisecond)
	f.drain()
	if j := f.wantRuns(2)[1]; j.Attempt != 2 {
		t.Errorf("retry has Attempt %d, want 2", j.Attempt)
	}
}

func queueRetriesPanics(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	calls := 0
	f.reg.Register("testkit.panic", func(context.Context, queue.Job) queue.Result {
		calls++
		if calls == 1 {
			panic("handler bug")
		}
		return queue.Done()
	})
	f.enqueue(queue.NewJob{Type: "testkit.panic"})
	f.drain()
	f.clock.Advance(contractPolicy.BaseDelay)
	f.drain()
	if calls != 2 {
		t.Fatalf("handler ran %d times, want 2 (a panic retries)", calls)
	}
}

func queueMovesFailedJobsToDead(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness, queue.Fail(errors.New("malformed activity")))
	j := probe(`{"N":"x"}`)
	j.IdempotencyKey = "k"
	f.enqueue(j)
	f.clock.Advance(time.Second)
	f.drain()
	f.wantRuns(1)
	dead := f.dead()
	if len(dead) != 1 {
		t.Fatalf("dead jobs = %v, want 1", dead)
	}
	d := dead[0]
	if d.Type != contractJobType || d.Attempt != 1 || d.LastError != "malformed activity" || d.IdempotencyKey != "k" ||
		!sameJSON(d.Payload, j.Payload) || !d.CreatedAt.Equal(contractStart) || !d.FailedAt.Equal(contractStart.Add(time.Second)) {
		t.Errorf("dead job = %+v", d)
	}
	f.clock.Advance(24 * time.Hour)
	if n := f.drain(); n != 0 {
		t.Fatal("a failed job ran again")
	}
	// A dead job no longer holds its idempotency key.
	f.enqueue(j)
	f.drain()
	f.wantRuns(2)
}

func queueMovesExhaustedJobsToDead(t *testing.T, newHarness QueueFactory) {
	boom := errors.New("remote is down")
	f := newQueueFixture(t, newHarness, queue.Retry(0, boom), queue.Retry(0, boom), queue.Retry(0, boom))
	f.enqueue(probe(`{"N":"x"}`))
	for range contractPolicy.MaxAttempts {
		f.drain()
		f.clock.Advance(contractPolicy.MaxDelay)
	}
	f.wantRuns(contractPolicy.MaxAttempts)
	dead := f.dead()
	if len(dead) != 1 || dead[0].Attempt != contractPolicy.MaxAttempts || !strings.Contains(dead[0].LastError, boom.Error()) {
		t.Fatalf("dead jobs = %+v, want one after %d attempts with error %q", dead, contractPolicy.MaxAttempts, boom)
	}
	if n := f.drain(); n != 0 {
		t.Fatal("an exhausted job ran again")
	}
}

func queueRetriesUnknownTypes(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	f.enqueue(queue.NewJob{Type: "testkit.unknown"})
	for range contractPolicy.MaxAttempts {
		if n := f.drain(); n != 1 {
			t.Fatalf("drain claimed %d jobs, want 1", n)
		}
		f.clock.Advance(contractPolicy.MaxDelay)
	}
	dead := f.dead()
	if len(dead) != 1 || !strings.Contains(dead[0].LastError, "testkit.unknown") {
		t.Fatalf("dead jobs = %+v, want the unknown job, naming its type", dead)
	}
}

func queueRequeuesDeadJobs(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness, queue.Fail(errors.New("bug")))
	f.enqueue(probe(`{"N":"x"}`))
	f.drain()
	dead := f.dead()
	if len(dead) != 1 {
		t.Fatalf("dead jobs = %v, want 1", dead)
	}
	if err := f.h.DeadJobs.RequeueDeadJob(t.Context(), dead[0].ID); err != nil {
		t.Fatalf("RequeueDeadJob: %v", err)
	}
	if d := f.dead(); len(d) != 0 {
		t.Fatalf("dead jobs after requeue = %v, want none", d)
	}
	f.drain()
	if j := f.wantRuns(2)[1]; j.Attempt != 1 || !sameJSON(j.Payload, dead[0].Payload) {
		t.Errorf("requeued job = %+v, want Attempt 1 and the same payload", j)
	}
	if err := f.h.DeadJobs.RequeueDeadJob(t.Context(), dead[0].ID); !errors.Is(err, queue.ErrDeadJobNotFound) {
		t.Errorf("RequeueDeadJob of a requeued job = %v, want ErrDeadJobNotFound", err)
	}
}

func queueDeletesDeadJobs(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness, queue.Fail(errors.New("bug")), queue.Fail(errors.New("bug")))
	f.enqueue(probe(`{"N":"x"}`), probe(`{"N":"y"}`))
	f.drain()
	dead := f.dead()
	if len(dead) != 2 {
		t.Fatalf("dead jobs = %v, want 2", dead)
	}
	if err := f.h.DeadJobs.DeleteDeadJob(t.Context(), dead[0].ID); err != nil {
		t.Fatalf("DeleteDeadJob: %v", err)
	}
	if d := f.dead(); len(d) != 1 || d[0].ID != dead[1].ID {
		t.Fatalf("dead jobs after delete = %+v, want only %s", d, dead[1].ID)
	}
	f.drain()
	f.wantRuns(2)
	if err := f.h.DeadJobs.DeleteDeadJob(t.Context(), dead[0].ID); !errors.Is(err, queue.ErrDeadJobNotFound) {
		t.Errorf("DeleteDeadJob of a deleted job = %v, want ErrDeadJobNotFound", err)
	}
	if err := f.h.DeadJobs.RequeueDeadJob(t.Context(), dead[0].ID); !errors.Is(err, queue.ErrDeadJobNotFound) {
		t.Errorf("RequeueDeadJob of a deleted job = %v, want ErrDeadJobNotFound", err)
	}
}

func queuePagesDeadJobs(t *testing.T, newHarness QueueFactory) {
	bug := queue.Fail(errors.New("bug"))
	f := newQueueFixture(t, newHarness, bug, bug, bug, bug)
	// Three jobs fail at the same time and one later.
	f.enqueue(probe(`{"N":"a"}`), probe(`{"N":"b"}`), probe(`{"N":"c"}`))
	f.drain()
	f.clock.Advance(time.Minute)
	f.enqueue(probe(`{"N":"d"}`))
	f.drain()
	all := f.dead()
	if len(all) != 4 || payloadOf(t, queue.Job{Payload: all[0].Payload}) != "d" {
		t.Fatalf("dead jobs = %+v, want 4, the latest failure first", all)
	}
	var paged []queue.DeadJob
	after := queue.DeadJobPosition{}
	for range len(all) {
		page, err := f.h.DeadJobs.ListDeadJobs(t.Context(), "", after, 2)
		if err != nil {
			t.Fatalf("ListDeadJobs(%+v): %v", after, err)
		}
		if len(page) == 0 {
			break
		}
		paged = append(paged, page...)
		after = page[len(page)-1].Position()
	}
	if len(paged) != len(all) {
		t.Fatalf("pages hold %d dead jobs, want %d", len(paged), len(all))
	}
	for i := range all {
		if paged[i].ID != all[i].ID {
			t.Fatalf("paged order = %+v, want %+v", paged, all)
		}
	}
	if none, err := f.h.DeadJobs.ListDeadJobs(t.Context(), "", queue.DeadJobPosition{}, 0); err != nil || len(none) != 0 {
		t.Errorf("ListDeadJobs(limit 0) = %+v, %v; want none", none, err)
	}
}

func queueDeletesOldDeadJobs(t *testing.T, newHarness QueueFactory) {
	bug := queue.Fail(errors.New("bug"))
	f := newQueueFixture(t, newHarness, bug, bug, bug, bug)
	// Jobs a and b fail at contractStart, c a minute later, and d an hour later.
	f.enqueue(probe(`{"N":"a"}`), probe(`{"N":"b"}`))
	f.drain()
	f.clock.Advance(time.Minute)
	f.enqueue(probe(`{"N":"c"}`))
	f.drain()
	f.clock.Advance(time.Hour)
	f.enqueue(probe(`{"N":"d"}`))
	f.drain()
	if d := f.dead(); len(d) != 4 {
		t.Fatalf("dead jobs = %+v, want 4", d)
	}
	ctx := t.Context()
	cutoff := contractStart.Add(time.Hour) // after a, b, and c; before d
	left := func() []string {
		var out []string
		for _, d := range f.dead() {
			out = append(out, payloadOf(t, queue.Job{Payload: d.Payload}))
		}
		slices.Sort(out)
		return out
	}
	if n, err := f.h.DeadJobs.DeleteDeadJobsBefore(ctx, cutoff, 0); err != nil || n != 0 {
		t.Fatalf("DeleteDeadJobsBefore(limit 0) = %d, %v; want 0", n, err)
	}
	// A batch takes the oldest failures first: a and b before c.
	if n, err := f.h.DeadJobs.DeleteDeadJobsBefore(ctx, cutoff, 2); err != nil || n != 2 {
		t.Fatalf("DeleteDeadJobsBefore(limit 2) = %d, %v; want 2", n, err)
	}
	if got := left(); !slices.Equal(got, []string{"c", "d"}) {
		t.Fatalf("dead jobs after the first batch = %v, want [c d]", got)
	}
	if n, err := f.h.DeadJobs.DeleteDeadJobsBefore(ctx, cutoff, 10); err != nil || n != 1 {
		t.Fatalf("DeleteDeadJobsBefore(limit 10) = %d, %v; want 1", n, err)
	}
	// A job that failed at the cutoff or later stays, and repeating deletes nothing.
	if n, err := f.h.DeadJobs.DeleteDeadJobsBefore(ctx, contractStart.Add(time.Hour+time.Minute), 10); err != nil || n != 0 {
		t.Fatalf("DeleteDeadJobsBefore(d's failure) = %d, %v; want 0", n, err)
	}
	if got := left(); !slices.Equal(got, []string{"d"}) {
		t.Fatalf("dead jobs after the purge = %v, want [d]", got)
	}
}

func queueReclaimsAfterLeaseExpiry(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	started, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var attempts []int
	f.reg.Register("testkit.slow", func(_ context.Context, j queue.Job) queue.Result {
		mu.Lock()
		attempts = append(attempts, j.Attempt)
		mu.Unlock()
		if j.Attempt == 1 {
			close(started)
			<-release // the first worker hangs until the test lets it go
		}
		return queue.Done()
	})
	f.enqueue(queue.NewJob{Type: "testkit.slow"})

	stale := make(chan error, 1)
	go func() {
		_, err := f.runner("stale").RunOnce(t.Context())
		stale <- err
	}()
	<-started

	other := f.runner("other")
	if ran, err := other.RunOnce(t.Context()); ran || err != nil {
		t.Fatalf("RunOnce during the lease = %v, %v, want no job", ran, err)
	}
	f.clock.Advance(contractLease)
	if ran, err := other.RunOnce(t.Context()); !ran || err != nil {
		t.Fatalf("RunOnce after the lease expired = %v, %v, want the job", ran, err)
	}
	close(release)
	if err := <-stale; !errors.Is(err, queue.ErrLeaseLost) {
		t.Errorf("stale worker's RunOnce = %v, want ErrLeaseLost", err)
	}
	if !slices.Equal(attempts, []int{1, 2}) {
		t.Errorf("attempts = %v, want [1 2]", attempts)
	}
	f.clock.Advance(time.Hour)
	if n := f.drain(); n != 0 {
		t.Errorf("drain claimed %d jobs after completion, want 0", n)
	}
	if d := f.dead(); len(d) != 0 {
		t.Errorf("dead jobs = %v, want none", d)
	}
}

func queueConcurrentWorkers(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	const jobs, workers = 40, 4
	batch := make([]queue.NewJob, jobs)
	for i := range batch {
		batch[i] = probe(fmt.Sprintf(`{"N":"j%02d"}`, i))
	}
	f.enqueue(batch...)
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := range workers {
		r := f.runner(fmt.Sprintf("w%d", w))
		wg.Go(func() {
			_, err := r.Drain(t.Context())
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Drain: %v", err)
		}
	}
	seen := map[string]int{}
	for _, j := range f.rec.snapshot() {
		seen[payloadOf(t, j)]++
	}
	if len(seen) != jobs {
		t.Errorf("ran %d distinct jobs, want %d", len(seen), jobs)
	}
	for n, c := range seen {
		if c != 1 {
			t.Errorf("job %s ran %d times, want once", n, c)
		}
	}
}

func keyedProbe(n, key string) queue.NewJob {
	j := probe(fmt.Sprintf(`{"N":%q}`, n))
	j.OrderingKey = key
	return j
}

// wantClaim claims a job as worker and checks its payload; want "" expects no job.
func (f *queueFixture) wantClaim(worker, want string) queue.Job {
	f.t.Helper()
	j, ok, err := f.h.Source.Claim(f.t.Context(), worker, f.clock.Now(), contractLease)
	switch {
	case err != nil:
		f.t.Fatalf("Claim(%s): %v", worker, err)
	case want == "" && ok:
		f.t.Fatalf("Claim(%s) = %s, want no job", worker, j.Payload)
	case want != "" && !ok:
		f.t.Fatalf("Claim(%s) found no job, want %s", worker, want)
	case ok && payloadOf(f.t, j) != want:
		f.t.Fatalf("Claim(%s) = %s, want %s", worker, j.Payload, want)
	}
	return j
}

func (f *queueFixture) complete(worker string, j queue.Job) {
	f.t.Helper()
	if err := f.h.Source.Complete(f.t.Context(), worker, j); err != nil {
		f.t.Fatalf("Complete(%s): %v", worker, err)
	}
}

func (f *queueFixture) relay() {
	f.t.Helper()
	if f.h.Relay != nil {
		if err := f.h.Relay(f.t.Context()); err != nil {
			f.t.Fatalf("Relay: %v", err)
		}
	}
}

func queueLeasesOnePerOrderingKey(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	f.enqueue(keyedProbe("a1", "a"), keyedProbe("a2", "a"), keyedProbe("b1", "b"), probe(`{"N":"plain"}`))
	f.relay()

	a1 := f.wantClaim("w1", "a1")
	f.wantClaim("w2", "b1") // a2 waits for a1
	f.wantClaim("w3", "plain")
	f.wantClaim("w4", "")

	// The leased job holds back its key for jobs enqueued later too.
	f.enqueue(keyedProbe("a3", "a"))
	f.relay()
	f.wantClaim("w4", "")

	f.complete("w1", a1)
	a2 := f.wantClaim("w4", "a2")
	f.wantClaim("w1", "")

	// When the lease of a2 expires, a2 is claimed again before a3.
	f.clock.Advance(contractLease)
	again := f.wantClaim("w1", "a2")
	if again.Attempt != a2.Attempt+1 {
		t.Errorf("reclaimed a2 has Attempt %d, want %d", again.Attempt, a2.Attempt+1)
	}
	f.wantClaim("w2", "b1") // its lease expired too; key b does not wait for key a
	f.wantClaim("w3", "plain")
	f.wantClaim("w4", "")
	f.complete("w1", again)
	f.wantClaim("w4", "a3")
}

func queueKeepsOrderWithinKey(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	f.reg.Register("testkit.flaky", func(_ context.Context, j queue.Job) queue.Result {
		if j.Attempt == 1 {
			return queue.Retry(0, errors.New("remote is down"))
		}
		return queue.Done()
	})
	f.reg.Register("testkit.broken", func(context.Context, queue.Job) queue.Result {
		return queue.Fail(errors.New("malformed activity"))
	})
	ran := func(want ...string) {
		t.Helper()
		got := []string{}
		for _, j := range f.rec.snapshot() {
			got = append(got, payloadOf(t, j))
		}
		if !slices.Equal(got, want) {
			t.Fatalf("ran %v, want %v", got, want)
		}
	}

	// A job backing off between retries holds back the later jobs of its key, but not others.
	f.enqueue(queue.NewJob{Type: "testkit.flaky", OrderingKey: "k"}, keyedProbe("k2", "k"), keyedProbe("other", "o"))
	f.drain()
	ran("other")
	f.clock.Advance(45*time.Second - time.Millisecond) // the backoff after run 1
	f.drain()
	ran("other")
	f.clock.Advance(time.Millisecond)
	f.drain()
	ran("other", "k2")

	// A job delayed by RunAt holds back the jobs of its key enqueued after it.
	delayed := keyedProbe("delayed", "k")
	delayed.RunAt = f.clock.Now().Add(10 * time.Minute)
	f.enqueue(delayed)
	f.clock.Advance(time.Millisecond)
	f.enqueue(keyedProbe("after delayed", "k"))
	f.drain()
	ran("other", "k2")
	f.clock.Advance(10 * time.Minute)
	f.drain()
	ran("other", "k2", "delayed", "after delayed")

	// A dead job no longer holds back its key.
	f.enqueue(queue.NewJob{Type: "testkit.broken", OrderingKey: "k"}, keyedProbe("after dead", "k"))
	f.drain()
	ran("other", "k2", "delayed", "after delayed", "after dead")
	if d := f.dead(); len(d) != 1 || d[0].OrderingKey != "k" {
		t.Fatalf("dead jobs = %+v, want the broken job with its ordering key", d)
	}
}

func queueSerializesKeysUnderConcurrency(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	const keys, perKey, workers = 3, 8, 4
	var (
		mu       sync.Mutex
		inFlight = map[string]int{}
		order    = map[string][]int{}
		overlaps []string
	)
	f.reg.Register("testkit.ordered", func(_ context.Context, j queue.Job) queue.Result {
		var v struct{ Seq int }
		if err := json.Unmarshal(j.Payload, &v); err != nil {
			return queue.Fail(err)
		}
		mu.Lock()
		inFlight[j.OrderingKey]++
		if inFlight[j.OrderingKey] > 1 {
			overlaps = append(overlaps, fmt.Sprintf("%s#%d", j.OrderingKey, v.Seq))
		}
		order[j.OrderingKey] = append(order[j.OrderingKey], v.Seq)
		mu.Unlock()
		time.Sleep(time.Millisecond) // give the other workers time to try the key
		mu.Lock()
		inFlight[j.OrderingKey]--
		mu.Unlock()
		return queue.Done()
	})
	var batch []queue.NewJob
	for seq := range perKey {
		for k := range keys {
			batch = append(batch, queue.NewJob{
				Type: "testkit.ordered", OrderingKey: fmt.Sprintf("key-%d", k),
				Payload: json.RawMessage(fmt.Sprintf(`{"Seq":%d}`, seq)),
			})
		}
		batch = append(batch, probe(fmt.Sprintf(`{"N":"plain-%d"}`, seq)))
	}
	f.enqueue(batch...)
	f.relay()

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := range workers {
		r := f.runner(fmt.Sprintf("w%d", w))
		wg.Go(func() {
			_, err := r.Drain(t.Context())
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Drain: %v", err)
		}
	}
	// A worker stops when every remaining job waits for a key that another worker holds; the
	// worker that completes the last job of a key runs on, so nothing should be left.
	if n := f.drain(); n != 0 {
		t.Errorf("a final drain claimed %d jobs, want 0", n)
	}
	if len(overlaps) > 0 {
		t.Errorf("jobs ran while another job of their key ran: %v", overlaps)
	}
	want := make([]int, perKey)
	for i := range want {
		want[i] = i
	}
	for k := range keys {
		key := fmt.Sprintf("key-%d", k)
		if got := order[key]; !slices.Equal(got, want) {
			t.Errorf("jobs of %s ran in order %v, want %v", key, got, want)
		}
	}
	if n := len(f.rec.snapshot()); n != perKey {
		t.Errorf("ran %d unkeyed jobs, want %d", n, perKey)
	}
}

func queueTransactionalEnqueue(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	if f.h.UnitOfWork == nil || f.h.Outbox == nil {
		t.Skip("the adapter does not enqueue inside database transactions")
	}
	boom := errors.New("domain write failed")
	err := f.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		if err := f.h.Outbox.EnqueueTx(t.Context(), tx, probe(`{"N":"rolled back"}`)); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Tx = %v, want %v", err, boom)
	}
	f.drain()
	f.wantRuns(0)

	attempts := 0
	err = f.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		attempts++
		if err := f.h.Outbox.EnqueueTx(t.Context(), tx, probe(`{"N":"committed", "s":"nul \u0000"}`)); err != nil {
			return err
		}
		if attempts == 1 {
			return uow.MarkRetryable(errors.New("conflict"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
	f.drain()
	if j := f.wantRuns(1)[0]; payloadOf(t, j) != "committed" {
		t.Fatalf("ran %s, want the committed job once", j.Payload)
	}

	keyed := probe(`{"N":"keyed"}`)
	keyed.IdempotencyKey = "tx-key"
	for range 2 {
		if err := f.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
			return f.h.Outbox.EnqueueTx(t.Context(), tx, keyed)
		}); err != nil {
			t.Fatalf("Tx = %v", err)
		}
	}
	f.drain()
	f.wantRuns(2)
}

func queueSurvivesRestart(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	if f.h.Restart == nil {
		t.Skip("the harness does not simulate a restart")
	}
	f.enqueue(probe(`{"N":"claimed"}`))
	// A worker claims the job and the process stops before it records a result.
	if _, ok, err := f.h.Source.Claim(t.Context(), "crashed", f.clock.Now(), contractLease); !ok || err != nil {
		t.Fatalf("Claim = %v, %v, want a job", ok, err)
	}
	f.enqueue(probe(`{"N":"waiting"}`))

	f.h = f.h.Restart(t)
	if n := f.drain(); n != 1 {
		t.Fatalf("after the restart, drain claimed %d jobs, want only the unclaimed one", n)
	}
	f.clock.Advance(contractLease)
	if n := f.drain(); n != 1 {
		t.Fatalf("after the lease expired, drain claimed %d jobs, want the crashed worker's job", n)
	}
	runs := f.wantRuns(2)
	if payloadOf(t, runs[0]) != "waiting" || payloadOf(t, runs[1]) != "claimed" || runs[1].Attempt != 2 {
		t.Errorf("runs = %+v, want waiting, then claimed with Attempt 2", runs)
	}
}

func queueReenqueueChain(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	limit := f.h.MaxDelay
	if limit <= 0 {
		t.Skip("the adapter has no delay limit")
	}
	j := probe(`{"N":"far"}`)
	j.RunAt = contractStart.Add(2*limit + limit/2)
	f.enqueue(j)

	for hop := 1; hop <= 2; hop++ {
		f.clock.Advance(limit - time.Millisecond)
		if n := f.drain(); n != 0 {
			t.Fatalf("hop %d: the job became visible before the delay limit", hop)
		}
		f.clock.Advance(time.Millisecond)
		if n := f.drain(); n != 1 {
			t.Fatalf("hop %d: drain claimed %d jobs, want the job to be re-enqueued", hop, n)
		}
		f.wantRuns(0)
	}
	f.clock.Advance(limit/2 - time.Millisecond)
	f.drain()
	f.wantRuns(0)
	f.clock.Advance(time.Millisecond)
	f.drain()
	if run := f.wantRuns(1)[0]; run.Attempt != 1 || !run.RunAt.Equal(j.RunAt) {
		t.Errorf("job = %+v, want Attempt 1 (re-enqueues do not count) and RunAt %v", run, j.RunAt)
	}
}

// contractTraceParent is the TraceParent of the jobs of queueKeepsTraceParent.
const contractTraceParent = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"

func queueKeepsTraceParent(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness, queue.Retry(0, errors.New("again")), queue.Fail(errors.New("bug")))
	traced := probe(`{"N":"traced"}`)
	traced.TraceParent = contractTraceParent
	f.enqueue(traced)
	f.drain()
	f.clock.Advance(time.Hour)
	f.drain()
	dead := f.dead()
	if len(dead) != 1 || dead[0].TraceParent != contractTraceParent {
		t.Fatalf("dead jobs = %+v, want one with TraceParent %s", dead, contractTraceParent)
	}
	if err := f.h.DeadJobs.RequeueDeadJob(t.Context(), dead[0].ID); err != nil {
		t.Fatalf("RequeueDeadJob: %v", err)
	}
	f.drain()
	for i, j := range f.wantRuns(3) {
		if j.TraceParent != contractTraceParent {
			t.Errorf("run %d TraceParent = %q, want %q", i+1, j.TraceParent, contractTraceParent)
		}
	}
	f.enqueue(probe(`{"N":"untraced"}`))
	f.drain()
	if j := f.wantRuns(4)[3]; j.TraceParent != "" {
		t.Errorf("TraceParent of a job enqueued without a span = %q, want empty", j.TraceParent)
	}
	if f.h.UnitOfWork == nil || f.h.Outbox == nil {
		return
	}
	if err := f.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return f.h.Outbox.EnqueueTx(t.Context(), tx, traced)
	}); err != nil {
		t.Fatalf("Tx = %v", err)
	}
	f.drain()
	if j := f.wantRuns(5)[4]; j.TraceParent != contractTraceParent {
		t.Errorf("TraceParent of a job enqueued in a transaction = %q, want %q", j.TraceParent, contractTraceParent)
	}
}

// queueConcurrentTransactions enqueues from concurrent transactions, each with a job of its
// own and a job with an idempotency key that every transaction shares: every committed job of
// its own runs once, and the shared key yields one job, whichever transactions commit first.
func queueConcurrentTransactions(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	if f.h.UnitOfWork == nil || f.h.Outbox == nil {
		t.Skip("the adapter does not enqueue inside database transactions")
	}
	const writers = 6
	var (
		ready sync.WaitGroup
		wg    sync.WaitGroup
	)
	start := make(chan struct{})
	errs := make(chan error, writers)
	ready.Add(writers)
	for w := range writers {
		wg.Go(func() {
			ready.Done()
			<-start
			shared := probe(`{"N":"shared"}`)
			shared.IdempotencyKey = "concurrent-shared"
			errs <- f.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
				return f.h.Outbox.EnqueueTx(t.Context(), tx, probe(fmt.Sprintf(`{"N":"own-%d"}`, w)), shared)
			})
		})
	}
	ready.Wait()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Tx = %v", err)
		}
	}
	f.drain()
	var got []string
	for _, j := range f.rec.snapshot() {
		got = append(got, payloadOf(t, j))
	}
	slices.Sort(got)
	want := []string{"shared"}
	for w := range writers {
		want = append(want, fmt.Sprintf("own-%d", w))
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

// queueRefusesRepeatedResults records the result of a claim twice, as a runner that cannot
// tell whether its first write committed might: the repeat reports ErrLeaseLost and changes no
// other job.
func queueRefusesRepeatedResults(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	f.enqueue(probe(`{"N":"done"}`), probe(`{"N":"released"}`))
	f.relay()
	ctx := t.Context()
	claim := func() queue.Job {
		t.Helper()
		j, ok, err := f.h.Source.Claim(ctx, "w1", f.clock.Now(), contractLease)
		if err != nil || !ok {
			t.Fatalf("Claim = %v, %v", ok, err)
		}
		return j
	}
	done, released := claim(), claim()
	if err := f.h.Source.Complete(ctx, "w1", done); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if err := f.h.Source.Complete(ctx, "w1", done); !errors.Is(err, queue.ErrLeaseLost) {
		t.Errorf("repeated Complete = %v, want ErrLeaseLost", err)
	}
	runAt := f.clock.Now().Add(time.Minute)
	if err := f.h.Source.Release(ctx, "w1", released, released.Attempt, runAt, "retry"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := f.h.Source.Release(ctx, "w1", released, released.Attempt, runAt, "retry"); !errors.Is(err, queue.ErrLeaseLost) {
		t.Errorf("repeated Release = %v, want ErrLeaseLost", err)
	}
	if err := f.h.Source.Bury(ctx, "w1", released, "dead", f.clock.Now()); !errors.Is(err, queue.ErrLeaseLost) {
		t.Errorf("Bury after Release = %v, want ErrLeaseLost", err)
	}
	f.clock.Advance(time.Minute)
	f.drain()
	if runs := f.wantRuns(1); payloadOf(t, runs[0]) != "released" {
		t.Errorf("ran %s, want the released job", runs[0].Payload)
	}
	if d := f.dead(); len(d) != 0 {
		t.Errorf("dead jobs = %v, want none", d)
	}
}
