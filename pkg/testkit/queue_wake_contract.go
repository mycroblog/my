// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

// QueueWakeHarness is a database job queue adapter under test that sends wake-up signals
// (ADR-0120), on its own empty storage.
type QueueWakeHarness struct {
	// Queue, Source, DeadJobs and Finder are the adapter's views of the same queue.
	Queue    queue.JobQueue
	Source   queue.Source
	DeadJobs queue.DeadJobs
	Finder   queue.WakeFinder
	// UnitOfWork and Outbox enqueue jobs in transactions of the queue's database.
	UnitOfWork uow.UnitOfWork
	Outbox     queue.Outbox
}

// QueueWakeFactory returns a harness on new, empty storage whose adapter reads time from clock
// and signals its jobs through waker. The factory registers the cleanup of everything it creates
// with t.Cleanup.
type QueueWakeFactory func(t *testing.T, clock *Clock, waker queue.Waker) QueueWakeHarness

// QueueWakeContract verifies a database job queue that carries its jobs to an external queue as
// wake-up signals (ADR-0120): every stored job is signaled at its RunAt, by Enqueue once it
// returns and by EnqueueTx only once its transaction commits; Source.ClaimJob claims a job by ID
// and tells a job not due yet, one its ordering key holds back, and one leased or gone apart; the
// WakeSweeper signals again the due jobs whose signal was lost; and a queue.PushReceiver on the
// adapter runs a signaled job, records its result as a pull runner does, and signals the job
// again after a retry, the next job of its ordering key once it settles, and a job delivered
// before its RunAt.
func QueueWakeContract(t *testing.T, newHarness QueueWakeFactory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, newHarness QueueWakeFactory)
	}{
		{"SignalsEnqueuedJobs", wakeSignalsEnqueuedJobs},
		{"SignalsOnlyCommittedTransactions", wakeSignalsOnlyCommitted},
		{"ClaimsJobsByID", wakeClaimsJobsByID},
		{"FindsTheNextJobOfAnOrderingKey", wakeFindsNextJob},
		{"SweepsLostSignals", wakeSweepsLostSignals},
		{"ReceivesSignals", wakeReceivesSignals},
		{"ReceivesSignalsOfAnOrderingKey", wakeReceivesKeyedSignals},
		{"ReceivesEarlySignals", wakeReceivesEarlySignals},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newHarness) })
	}
}

type wakeFixture struct {
	t     *testing.T
	h     QueueWakeHarness
	clock *Clock
	waker *Waker
	reg   *queue.JobHandlerRegistry
	rec   *recorder
}

func newWakeFixture(t *testing.T, newHarness QueueWakeFactory, results ...queue.Result) *wakeFixture {
	t.Helper()
	clock := NewClock(contractStart)
	waker := &Waker{}
	f := &wakeFixture{t: t, h: newHarness(t, clock, waker), clock: clock, waker: waker, reg: &queue.JobHandlerRegistry{}, rec: &recorder{results: results}}
	f.reg.Register(contractJobType, f.rec.handle)
	return f
}

// enqueue enqueues one job and returns the ID of its signal.
func (f *wakeFixture) enqueue(j queue.NewJob) string {
	f.t.Helper()
	if err := f.h.Queue.Enqueue(f.t.Context(), j); err != nil {
		f.t.Fatalf("Enqueue: %v", err)
	}
	wakes := f.waker.Take()
	if len(wakes) != 1 {
		f.t.Fatalf("Enqueue sent %d signals, want 1: %v", len(wakes), wakes)
	}
	return wakes[0].JobID
}

func (f *wakeFixture) claim(id, worker string) queue.ClaimResult {
	f.t.Helper()
	r, err := f.h.Source.ClaimJob(f.t.Context(), id, worker, f.clock.Now(), contractLease)
	if err != nil {
		f.t.Fatalf("ClaimJob(%s, %s): %v", id, worker, err)
	}
	return r
}

func (f *wakeFixture) wantClaim(id, worker string, want queue.ClaimOutcome) queue.ClaimResult {
	f.t.Helper()
	r := f.claim(id, worker)
	if r.Outcome != want {
		f.t.Fatalf("ClaimJob(%s, %s) = %v, want %v", id, worker, r.Outcome, want)
	}
	if want == queue.Claimed && r.Job.ID != id {
		f.t.Fatalf("ClaimJob(%s) claimed %s", id, r.Job.ID)
	}
	return r
}

func (f *wakeFixture) receiver() *queue.PushReceiver {
	return queue.NewPushReceiver(f.h.Source, f.reg, f.waker, f.h.Finder, queue.RunnerOptions{
		Clock:  f.clock,
		Worker: "push",
		Lease:  contractLease,
		Retry:  contractPolicy,
		Random: FixedRandom(0.5),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func (f *wakeFixture) receive(id string) {
	f.t.Helper()
	if err := f.receiver().Receive(f.t.Context(), id); err != nil {
		f.t.Fatalf("Receive(%s): %v", id, err)
	}
}

func (f *wakeFixture) wantWakes(want ...queue.Wake) {
	f.t.Helper()
	got := f.waker.Take()
	if len(got) != len(want) {
		f.t.Fatalf("signals = %v, want %v", got, want)
	}
	for i := range want {
		if got[i].JobID != want[i].JobID || !got[i].At.Equal(want[i].At) {
			f.t.Fatalf("signals = %v, want %v", got, want)
		}
	}
}

func wakeSignalsEnqueuedJobs(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness)
	later := probe(`{"N":"later"}`)
	later.RunAt = contractStart.Add(time.Hour)
	if err := f.h.Queue.Enqueue(t.Context(), probe(`{"N":"now"}`), later); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	wakes := f.waker.Take()
	if len(wakes) != 2 || !wakes[0].At.Equal(contractStart) || !wakes[1].At.Equal(later.RunAt) {
		t.Fatalf("signals = %v, want one at now and one at the RunAt of the delayed job", wakes)
	}
	r := f.wantClaim(wakes[0].JobID, "w1", queue.Claimed)
	if payloadOf(t, r.Job) != "now" {
		t.Errorf("the first signal claimed %s, want the job due now", r.Job.Payload)
	}

	// A dead job requeued by an operator is signaled to run now.
	f.clock.Advance(time.Minute)
	if err := f.h.Source.Bury(t.Context(), "w1", r.Job, "broken", f.clock.Now()); err != nil {
		t.Fatalf("Bury: %v", err)
	}
	if err := f.h.DeadJobs.RequeueDeadJob(t.Context(), r.Job.ID); err != nil {
		t.Fatalf("RequeueDeadJob: %v", err)
	}
	f.wantWakes(queue.Wake{JobID: r.Job.ID, At: f.clock.Now()})
	f.wantClaim(r.Job.ID, "w1", queue.Claimed)
}

// unknownJobID is a job ID that no contract case enqueues.
const unknownJobID = "01900000-0000-7000-8000-000000000000"

func wakeSignalsOnlyCommitted(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness)
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
	f.wantWakes()

	attempts := 0
	err = f.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		attempts++
		if err := f.h.Outbox.EnqueueTx(t.Context(), tx, probe(fmt.Sprintf(`{"N":"attempt %d"}`, attempts))); err != nil {
			return err
		}
		if n := f.waker.Len(); n != 0 {
			t.Errorf("%d signals sent before the commit", n)
		}
		if attempts == 1 {
			return uow.MarkRetryable(errors.New("conflict"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Tx = %v", err)
	}
	wakes := f.waker.Take()
	if len(wakes) != 1 {
		t.Fatalf("signals = %v, want the one of the committed attempt", wakes)
	}
	if r := f.wantClaim(wakes[0].JobID, "w1", queue.Claimed); payloadOf(t, r.Job) != "attempt 2" {
		t.Errorf("the signal claimed %s, want the job of the committed attempt", r.Job.Payload)
	}
}

func wakeClaimsJobsByID(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness)
	a := f.enqueue(probe(`{"N":"a"}`))
	claimed := f.wantClaim(a, "w1", queue.Claimed)
	if claimed.Job.Attempt != 1 || payloadOf(t, claimed.Job) != "a" || claimed.Job.Type != contractJobType {
		t.Errorf("claimed %+v, want job a at Attempt 1", claimed.Job)
	}
	f.wantClaim(a, "w2", queue.ClaimUnavailable) // leased
	if _, ok, err := f.h.Source.Claim(t.Context(), "w2", f.clock.Now(), contractLease); err != nil || ok {
		t.Fatalf("Claim = %v, %v, want no job while a is leased", ok, err)
	}

	later := probe(`{"N":"later"}`)
	later.RunAt = contractStart.Add(time.Hour)
	l := f.enqueue(later)
	if r := f.wantClaim(l, "w1", queue.ClaimNotDue); r.Job.ID != l || !r.Job.RunAt.Equal(later.RunAt) {
		t.Errorf("not due = %+v, want the job's RunAt %v", r.Job, later.RunAt)
	}

	k1, k2 := f.enqueue(keyedProbe("k1", "k")), f.enqueue(keyedProbe("k2", "k"))
	f.wantClaim(k2, "w1", queue.ClaimHeldBack) // k1 was enqueued before it
	first := f.wantClaim(k1, "w1", queue.Claimed)
	f.wantClaim(k2, "w2", queue.ClaimHeldBack) // k1 is leased
	if err := f.h.Source.Complete(t.Context(), "w1", first.Job); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	f.wantClaim(k1, "w1", queue.ClaimUnavailable) // completed
	second := f.wantClaim(k2, "w2", queue.Claimed)

	// A lease that expired can be claimed again.
	f.clock.Advance(contractLease)
	if again := f.wantClaim(k2, "w3", queue.Claimed); again.Job.Attempt != second.Job.Attempt+1 {
		t.Errorf("reclaimed k2 at Attempt %d, want %d", again.Job.Attempt, second.Job.Attempt+1)
	}
	// Once its RunAt came, the delayed job is claimed.
	f.clock.Set(later.RunAt)
	f.wantClaim(l, "w1", queue.Claimed)

	f.wantClaim(unknownJobID, "w1", queue.ClaimUnavailable) // never enqueued
	f.wantClaim("not-a-job-id", "w1", queue.ClaimUnavailable)
}

func wakeFindsNextJob(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness)
	if _, ok, err := f.h.Finder.NextJob(t.Context(), "k"); err != nil || ok {
		t.Fatalf("NextJob of an empty key = %v, %v", ok, err)
	}
	k2job := keyedProbe("k2", "k")
	k2job.RunAt = contractStart.Add(time.Hour)
	k1, k2 := f.enqueue(keyedProbe("k1", "k")), f.enqueue(k2job)
	f.enqueue(keyedProbe("other", "j"))
	next, ok, err := f.h.Finder.NextJob(t.Context(), "k")
	if err != nil || !ok || next.JobID != k1 || !next.At.Equal(contractStart) {
		t.Fatalf("NextJob = %v, %v, %v, want k1 at now", next, ok, err)
	}
	c := f.wantClaim(k1, "w1", queue.Claimed)
	if err := f.h.Source.Complete(t.Context(), "w1", c.Job); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	next, ok, err = f.h.Finder.NextJob(t.Context(), "k")
	if err != nil || !ok || next.JobID != k2 || !next.At.Equal(k2job.RunAt) {
		t.Fatalf("NextJob = %v, %v, %v, want k2 at its RunAt", next, ok, err)
	}
}

func wakeSweepsLostSignals(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness)
	sweeper := queue.NewWakeSweeper(queue.WakeSweepDeps{Finder: f.h.Finder, Waker: f.waker, Clock: f.clock, Batch: 2})
	sweep := func() []string {
		t.Helper()
		n, err := sweeper.Sweep(t.Context())
		if err != nil {
			t.Fatalf("Sweep: %v", err)
		}
		wakes := f.waker.Take()
		if n != len(wakes) {
			t.Fatalf("Sweep = %d, but sent %d signals", n, len(wakes))
		}
		ids := make([]string, len(wakes))
		for i, w := range wakes {
			if !w.At.Equal(f.clock.Now()) {
				t.Errorf("swept signal at %v, want now", w.At)
			}
			ids[i] = w.JobID
		}
		return ids
	}

	// The signals of these jobs are lost; the enqueue itself succeeds.
	f.waker.Fail(errors.New("external queue down"))
	// The first job (by RunAt, then ID) is the one the worker below claims.
	for _, j := range []queue.NewJob{probe(`{"N":"leased"}`), probe(`{"N":"lost"}`), keyedProbe("k1", "k"), keyedProbe("k2", "k")} {
		if err := f.h.Queue.Enqueue(t.Context(), j); err != nil {
			t.Fatalf("Enqueue with a failing waker = %v, want nil", err)
		}
	}
	err := f.h.UnitOfWork.Tx(t.Context(), func(tx uow.Tx) error {
		return f.h.Outbox.EnqueueTx(t.Context(), tx, probe(`{"N":"lost in tx"}`))
	})
	if err != nil {
		t.Fatalf("Tx with a failing waker = %v, want nil", err)
	}
	f.waker.Fail(nil)

	// A worker holds one job; its lease outlasts the grace period.
	if _, ok, err := f.h.Source.Claim(t.Context(), "busy", f.clock.Now(), time.Hour); err != nil || !ok {
		t.Fatalf("Claim = %v, %v", ok, err)
	}
	if ids := sweep(); len(ids) != 0 {
		t.Fatalf("swept %v within the grace period, want none", ids)
	}
	f.clock.Advance(queue.DefaultWakeGrace)
	first := sweep()
	second := sweep() // the batch is 2, and the jobs swept are still due: the next sweep sends them again
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("sweeps = %v, %v, want a batch of 2 each", first, second)
	}

	// Run every job the sweep finds: the lost ones and k2 after k1, but not the leased one.
	ran := map[string]bool{}
	for range 4 {
		ids, err := f.h.Finder.DueJobs(t.Context(), f.clock.Now(), f.clock.Now(), 100)
		if err != nil {
			t.Fatalf("DueJobs: %v", err)
		}
		for _, id := range ids {
			c := f.wantClaim(id, "w1", queue.Claimed)
			ran[payloadOf(t, c.Job)] = true
			if err := f.h.Source.Complete(t.Context(), "w1", c.Job); err != nil {
				t.Fatalf("Complete: %v", err)
			}
		}
	}
	for _, want := range []string{"lost", "k1", "k2", "lost in tx"} {
		if !ran[want] {
			t.Errorf("the sweep did not recover %s (recovered %v)", want, ran)
		}
	}
	if ran["leased"] {
		t.Error("the sweep signaled a leased job")
	}
	if ids, err := f.h.Finder.DueJobs(t.Context(), f.clock.Now(), f.clock.Now(), 0); err != nil || len(ids) != 0 {
		t.Errorf("DueJobs with limit 0 = %v, %v, want none", ids, err)
	}
}

func wakeReceivesSignals(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness, queue.Retry(0, errors.New("flaky")), queue.Retry(0, errors.New("flaky")), queue.Fail(errors.New("broken")))
	panicking := false
	f.reg.Register("testkit.panics", func(context.Context, queue.Job) queue.Result {
		if !panicking {
			panicking = true
			panic("handler bug")
		}
		return queue.Done()
	})

	// A retry releases the job and signals it at the backoff's RunAt.
	id := f.enqueue(probe(`{"N":"retried"}`))
	f.receive(id)
	f.wantRuns(1)
	backoff := contractPolicy.Backoff(1, FixedRandom(0.5))
	f.wantWakes(queue.Wake{JobID: id, At: contractStart.Add(backoff)})
	// An early duplicate of the signal finds the job not due and sends it again.
	f.receive(id)
	f.wantRuns(1)
	f.wantWakes(queue.Wake{JobID: id, At: contractStart.Add(backoff)})

	// The second run retries again; the third fails for good.
	f.clock.Advance(backoff)
	f.receive(id)
	f.wantWakes(queue.Wake{JobID: id, At: f.clock.Now().Add(contractPolicy.Backoff(2, FixedRandom(0.5)))})
	f.clock.Advance(contractPolicy.Backoff(2, FixedRandom(0.5)))
	f.receive(id)
	f.wantRuns(3)
	f.wantWakes() // dead, and no ordering key
	if dead, err := f.h.DeadJobs.ListDeadJobs(t.Context(), "", queue.DeadJobPosition{}, 10); err != nil || len(dead) != 1 || dead[0].ID != id {
		t.Fatalf("dead jobs = %v, %v, want the failed job", dead, err)
	}
	f.receive(id) // a late duplicate finds nothing
	f.wantRuns(3)

	// A panic is a retry.
	p := f.enqueue(queue.NewJob{Type: "testkit.panics", Payload: json.RawMessage(`{}`)})
	f.receive(p)
	wakes := f.waker.Take()
	if len(wakes) != 1 || wakes[0].JobID != p || !wakes[0].At.After(f.clock.Now()) {
		t.Fatalf("signals after a panic = %v, want the job again later", wakes)
	}
	f.clock.Set(wakes[0].At)
	f.receive(p)
	f.wantWakes()
	if r := f.claim(p, "w1"); r.Outcome != queue.ClaimUnavailable {
		t.Errorf("the job that panicked once is %v after its retry, want completed", r.Outcome)
	}
}

func (f *wakeFixture) wantRuns(n int) []queue.Job {
	f.t.Helper()
	runs := f.rec.snapshot()
	if len(runs) != n {
		f.t.Fatalf("handler ran %d times, want %d", len(runs), n)
	}
	return runs
}

func wakeReceivesKeyedSignals(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness, queue.Done(), queue.Fail(errors.New("broken")))
	k1, k2 := f.enqueue(keyedProbe("k1", "k")), f.enqueue(keyedProbe("k2", "k"))
	k3job := keyedProbe("k3", "k")
	k3job.RunAt = contractStart.Add(time.Hour)
	k3 := f.enqueue(k3job)

	// The signal of k2 arrives first: k1 holds it back, and the signal is dropped.
	f.receive(k2)
	f.wantRuns(0)
	f.wantWakes()
	// k1 completes and signals k2.
	f.receive(k1)
	f.wantWakes(queue.Wake{JobID: k2, At: f.clock.Now()})
	// k2 becomes dead and signals k3 at its RunAt.
	f.receive(k2)
	f.wantRuns(2)
	f.wantWakes(queue.Wake{JobID: k3, At: k3job.RunAt})
}

func wakeReceivesEarlySignals(t *testing.T, newHarness QueueWakeFactory) {
	f := newWakeFixture(t, newHarness)
	far := probe(`{"N":"far"}`)
	far.RunAt = contractStart.Add(48 * time.Hour)
	id := f.enqueue(far)
	// An external queue with a delay limit delivers the signal before the RunAt.
	f.clock.Advance(15 * time.Minute)
	f.receive(id)
	f.wantRuns(0)
	f.wantWakes(queue.Wake{JobID: id, At: far.RunAt})
	f.clock.Set(far.RunAt)
	f.receive(id)
	if run := f.wantRuns(1)[0]; run.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1: an early signal does not count", run.Attempt)
	}
}
