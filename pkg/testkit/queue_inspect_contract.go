// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/queue"
)

// The cases of QueueContract for the queue dashboard (ADR-0064): JobInspector and AttemptLog,
// run when the harness sets them.

const contractOtherType = "testkit.other"

// queueInspectsJobsByState checks that JobInspector lists the pending jobs in RunAt order with
// the state derived from their current values (running while the lease holds, queued before the
// first run, retrying after it), filters them by state and type, pages them, and counts every
// state, dead jobs included; and that DeadJobs filters dead jobs by type.
func queueInspectsJobsByState(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness)
	if f.h.Inspector == nil {
		t.Skip("the adapter cannot list its jobs")
	}
	f.enqueue(probe(`{"N":"a"}`))
	later := probe(`{"N":"b"}`)
	later.RunAt = contractStart.Add(time.Hour)
	f.enqueue(later)
	f.enqueue(queue.NewJob{Type: contractOtherType, Payload: []byte(`{"N":"c"}`)})
	f.enqueue(probe(`{"N":"d"}`))

	a := f.wantClaim("w1", "a")
	c := f.wantClaim("w1", "c")
	if err := f.h.Source.Release(t.Context(), "w1", c, c.Attempt, f.clock.Now().Add(time.Minute), "boom"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	d := f.wantClaim("w1", "d")
	if err := f.h.Source.Bury(t.Context(), "w1", d, "bug", f.clock.Now()); err != nil {
		t.Fatalf("Bury: %v", err)
	}

	list := func(filter queue.JobFilter, after queue.JobPosition, limit int) []queue.PendingJob {
		t.Helper()
		jobs, err := f.h.Inspector.ListJobs(t.Context(), f.clock.Now(), filter, after, limit)
		if err != nil {
			t.Fatalf("ListJobs(%+v): %v", filter, err)
		}
		return jobs
	}
	all := list(queue.JobFilter{}, queue.JobPosition{}, 10)
	if len(all) != 3 || all[0].ID != a.ID || all[1].ID != c.ID || all[2].Type != contractJobType {
		t.Fatalf("ListJobs() = %+v, want a, c, b in RunAt order", all)
	}
	if got := all[0]; got.State != queue.JobStateRunning || got.Worker != "w1" || !got.LockedUntil.Equal(contractStart.Add(contractLease)) ||
		got.Attempt != 1 || !sameJSON(got.Payload, []byte(`{"N":"a"}`)) || !got.CreatedAt.Equal(contractStart) {
		t.Errorf("running job = %+v", got)
	}
	if got := all[1]; got.State != queue.JobStateRetrying || got.Attempt != 1 || got.LastError != "boom" || got.Worker != "" ||
		!got.RunAt.Equal(contractStart.Add(time.Minute)) {
		t.Errorf("retrying job = %+v", got)
	}
	if got := all[2]; got.State != queue.JobStateQueued || got.Attempt != 0 || !got.RunAt.Equal(contractStart.Add(time.Hour)) {
		t.Errorf("queued job = %+v", got)
	}

	for _, tc := range []struct {
		filter queue.JobFilter
		want   []string
	}{
		{queue.JobFilter{State: queue.JobStateRunning}, []string{a.ID}},
		{queue.JobFilter{State: queue.JobStateRetrying}, []string{c.ID}},
		{queue.JobFilter{State: queue.JobStateQueued}, []string{all[2].ID}},
		{queue.JobFilter{State: queue.JobStateDead}, nil},
		{queue.JobFilter{Type: contractOtherType}, []string{c.ID}},
		{queue.JobFilter{State: queue.JobStateQueued, Type: contractOtherType}, nil},
	} {
		got := list(tc.filter, queue.JobPosition{}, 10)
		var ids []string
		for _, j := range got {
			ids = append(ids, j.ID)
		}
		if strings.Join(ids, ",") != strings.Join(tc.want, ",") {
			t.Errorf("ListJobs(%+v) = %v, want %v", tc.filter, ids, tc.want)
		}
	}
	first := list(queue.JobFilter{}, queue.JobPosition{}, 2)
	rest := list(queue.JobFilter{}, first[1].Position(), 2)
	if len(first) != 2 || len(rest) != 1 || rest[0].ID != all[2].ID {
		t.Errorf("pages = %+v, %+v; want 2 then the last", first, rest)
	}
	if none := list(queue.JobFilter{}, queue.JobPosition{}, 0); len(none) != 0 {
		t.Errorf("ListJobs(limit 0) = %+v, want none", none)
	}

	summary := func() queue.JobSummary {
		t.Helper()
		s, err := f.h.Inspector.SummarizeJobs(t.Context(), f.clock.Now())
		if err != nil {
			t.Fatalf("SummarizeJobs: %v", err)
		}
		return s
	}
	want := []queue.JobCount{
		{State: queue.JobStateQueued, Type: contractJobType, Count: 1},
		{State: queue.JobStateRunning, Type: contractJobType, Count: 1},
		{State: queue.JobStateRetrying, Type: contractOtherType, Count: 1},
		{State: queue.JobStateDead, Type: contractJobType, Count: 1},
	}
	if s := summary(); !sameCounts(s.Counts, want) || !s.OldestReadyAt.IsZero() {
		t.Errorf("SummarizeJobs() = %+v, want %+v and no ready job", s, want)
	}
	// Once the lease of a expired, a is due again and waits to be retried.
	f.clock.Advance(2 * time.Minute)
	want = []queue.JobCount{
		{State: queue.JobStateQueued, Type: contractJobType, Count: 1},
		{State: queue.JobStateRetrying, Type: contractOtherType, Count: 1},
		{State: queue.JobStateRetrying, Type: contractJobType, Count: 1},
		{State: queue.JobStateDead, Type: contractJobType, Count: 1},
	}
	if s := summary(); !sameCounts(s.Counts, want) || !s.OldestReadyAt.Equal(contractStart) {
		t.Errorf("SummarizeJobs() after the lease = %+v, want %+v and the oldest ready at %v", s, want, contractStart)
	}

	for _, tc := range []struct {
		jobType string
		want    int
	}{{"", 1}, {contractJobType, 1}, {contractOtherType, 0}} {
		got, err := f.h.DeadJobs.ListDeadJobs(t.Context(), tc.jobType, queue.DeadJobPosition{}, 10)
		if err != nil || len(got) != tc.want {
			t.Errorf("ListDeadJobs(%q) = %+v, %v; want %d", tc.jobType, got, err, tc.want)
		}
	}
}

func sameCounts(got, want []queue.JobCount) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// queueRecordsUnsuccessfulAttempts checks that the runner records each run that did not succeed
// in the AttemptLog (and no success), that the records list the most recently finished first in
// pages, that recording a run again keeps one record, that a requeued dead job counts its
// attempts again, that long errors are cut, and that old records are deleted in bounded batches.
func queueRecordsUnsuccessfulAttempts(t *testing.T, newHarness QueueFactory) {
	f := newQueueFixture(t, newHarness, queue.Retry(0, errors.New("first")), queue.Fail(errors.New("second")), queue.Fail(errors.New("third")))
	if f.h.Attempts == nil {
		t.Skip("the harness has no attempt log")
	}
	f.enqueue(probe(`{"N":"x"}`))
	f.drain()
	f.clock.Advance(time.Hour)
	f.drain()
	dead := f.dead()
	if len(dead) != 1 {
		t.Fatalf("dead jobs = %v, want 1", dead)
	}
	id := dead[0].ID
	list := func(after queue.AttemptPosition, limit int) []queue.Attempt {
		t.Helper()
		got, err := f.h.Attempts.ListAttempts(t.Context(), id, after, limit)
		if err != nil {
			t.Fatalf("ListAttempts: %v", err)
		}
		return got
	}
	got := list(queue.AttemptPosition{}, 10)
	retried, failed := contractStart, contractStart.Add(time.Hour)
	want := []queue.Attempt{
		{JobID: id, JobType: contractJobType, Attempt: 2, Outcome: queue.OutcomeDead, Error: "second", Worker: "w1", StartedAt: failed, FinishedAt: failed},
		{JobID: id, JobType: contractJobType, Attempt: 1, Outcome: queue.OutcomeRetry, Error: "first", Worker: "w1", StartedAt: retried, FinishedAt: retried},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ListAttempts() = %+v, want %+v", got, want)
	}
	if err := f.h.Attempts.RecordAttempt(t.Context(), want[1]); err != nil {
		t.Fatalf("RecordAttempt again: %v", err)
	}
	if page := list(queue.AttemptPosition{}, 1); len(page) != 1 || page[0] != want[0] {
		t.Errorf("first page = %+v, want %+v", page, want[0])
	}
	if page := list(want[0].Position(), 5); len(page) != 1 || page[0] != want[1] {
		t.Errorf("second page = %+v, want only %+v (recorded once)", page, want[1])
	}

	// A requeued dead job counts its attempts again; its record of attempt 1 is a new one.
	if err := f.h.DeadJobs.RequeueDeadJob(t.Context(), id); err != nil {
		t.Fatalf("RequeueDeadJob: %v", err)
	}
	f.clock.Advance(time.Hour)
	f.drain()
	if got := list(queue.AttemptPosition{}, 10); len(got) != 3 || got[0].Attempt != 1 || got[0].Error != "third" || got[0].Outcome != queue.OutcomeDead {
		t.Errorf("ListAttempts() after the requeue = %+v, want a new record of attempt 1 first", got)
	}

	long := queue.Attempt{JobID: NamedID("long"), JobType: contractJobType, Attempt: 1, Outcome: queue.OutcomeRetry, Error: strings.Repeat("e", 3000),
		Worker: "w1", StartedAt: f.clock.Now(), FinishedAt: f.clock.Now()}
	if err := f.h.Attempts.RecordAttempt(t.Context(), long); err != nil {
		t.Fatalf("RecordAttempt(long): %v", err)
	}
	if got, err := f.h.Attempts.ListAttempts(t.Context(), NamedID("long"), queue.AttemptPosition{}, 1); err != nil || len(got) != 1 || len(got[0].Error) != queue.MaxAttemptErrorBytes {
		t.Errorf("long error kept %+v, %v; want %d bytes", got, err, queue.MaxAttemptErrorBytes)
	}

	// Records finished before the second hour go: the two of the first run.
	n, err := f.h.Attempts.DeleteAttemptsBefore(t.Context(), contractStart.Add(90*time.Minute), 1)
	if err != nil || n != 1 {
		t.Fatalf("DeleteAttemptsBefore(limit 1) = %d, %v; want 1", n, err)
	}
	n, err = f.h.Attempts.DeleteAttemptsBefore(t.Context(), contractStart.Add(90*time.Minute), 10)
	if err != nil || n != 1 {
		t.Fatalf("DeleteAttemptsBefore = %d, %v; want the other one", n, err)
	}
	if got := list(queue.AttemptPosition{}, 10); len(got) != 1 || got[0].Error != "third" {
		t.Errorf("ListAttempts() after the purge = %+v, want the record of the requeued run", got)
	}
	if n, err := f.h.Attempts.DeleteAttemptsBefore(t.Context(), f.clock.Now().Add(time.Hour), 0); err != nil || n != 0 {
		t.Errorf("DeleteAttemptsBefore(limit 0) = %d, %v; want 0", n, err)
	}
}
