// SPDX-License-Identifier: AGPL-3.0-only

package account_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type noTx struct{}

func (noTx) Emit(context.Context, uow.Event) error { return nil }

type directUOW struct{}

func (directUOW) Tx(_ context.Context, fn func(uow.Tx) error) error { return fn(noTx{}) }

// erasureRepo keeps the accounts to erase in ID order and what the erasure did to them.
type erasureRepo struct {
	targets  []account.ErasureTarget
	profiles map[string]bool
	erased   map[string]bool
}

func (r *erasureRepo) ListAccountsToErase(_ context.Context, _ time.Time, after string, limit int) ([]account.ErasureTarget, error) {
	var out []account.ErasureTarget
	for _, t := range r.targets {
		if t.AccountID > after && !r.erased[t.AccountID] && len(out) < limit {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *erasureRepo) EraseProfilesAndKeys(_ context.Context, _ uow.Tx, accountID string) error {
	r.profiles[accountID] = true
	return nil
}

func (r *erasureRepo) InsertErasure(_ context.Context, _ uow.Tx, accountID string, _ time.Time) error {
	r.erased[accountID] = true
	return nil
}

func (r *erasureRepo) AccountErased(_ context.Context, accountID string) (bool, error) {
	return r.erased[accountID], nil
}

// retainedEraser stands in for the erasure of statuses that the moderation records keep: each
// batch passes over limit retained items after the cursor (an item number) and keeps them.
type retainedEraser struct{ items map[string]int }

func (e retainedEraser) EraseAccountData(_ context.Context, t account.ErasureTarget, cursor string, limit int) (account.ErasureStep, error) {
	from := 0
	if cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil {
			return account.ErasureStep{}, err
		}
		from = n
	}
	next := min(from+limit, e.items[t.AccountID])
	return account.ErasureStep{Next: strconv.Itoa(next), Done: next >= e.items[t.AccountID], Kept: next > from}, nil
}

// laterEraser records the accounts it erased.
type laterEraser struct{ erased map[string]bool }

func (e laterEraser) EraseAccountData(_ context.Context, t account.ErasureTarget, _ string, _ int) (account.ErasureStep, error) {
	e.erased[t.AccountID] = true
	return account.ErasureStep{Done: true}, nil
}

type recordingQueue struct{ jobs []queue.NewJob }

func (q *recordingQueue) Enqueue(_ context.Context, jobs ...queue.NewJob) error {
	q.jobs = append(q.jobs, jobs...)
	return nil
}

// TestErasureGoesPastRetainedStatuses checks that the continuations of a run of the erasure go
// on from where the previous job stopped inside an account: an account with more retained
// statuses than one job's budget reaches its later erasers and its profile, and the next
// account is erased, with the kept statuses still keeping the account from being recorded as
// erased.
func TestErasureGoesPastRetainedStatuses(t *testing.T) {
	repo := &erasureRepo{
		targets:  []account.ErasureTarget{{AccountID: "a"}, {AccountID: "b"}},
		profiles: map[string]bool{}, erased: map[string]bool{},
	}
	later := laterEraser{erased: map[string]bool{}}
	jobs := &recordingQueue{}
	er := account.NewEraser(account.ErasureDeps{
		UnitOfWork: directUOW{}, Repository: repo, Jobs: jobs, Clock: fixedClock{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, Batch: 1,
		// "a" keeps more statuses than three jobs pass over with batches of one.
		Erasers: []account.DataEraser{retainedEraser{items: map[string]int{"a": 160}}, later},
	})
	reg := &queue.JobHandlerRegistry{}
	er.RegisterJobs(reg, queue.NewScheduler(nil, fixedClock{}, nil))
	h, _, ok := reg.Lookup(account.JobEraseDeletedAccounts)
	if !ok {
		t.Fatal("no erasure handler")
	}
	job := queue.Job{Type: account.JobEraseDeletedAccounts, Attempt: 1}
	for run := 0; ; run++ {
		if run == 10 {
			t.Fatalf("the erasure did not end after %d jobs; later erasers reached %v", run, later.erased)
		}
		if r := h(t.Context(), job); !r.IsDone() {
			t.Fatalf("erasure job = %+v, want done", r)
		}
		if len(jobs.jobs) == 0 {
			break
		}
		next := jobs.jobs[0]
		jobs.jobs = jobs.jobs[1:]
		job = queue.Job{Type: next.Type, Payload: next.Payload, Attempt: 1}
	}
	if !later.erased["a"] || !later.erased["b"] {
		t.Errorf("later erasers reached %v, want both accounts", later.erased)
	}
	if !repo.profiles["a"] || !repo.profiles["b"] {
		t.Errorf("profiles erased of %v, want both accounts", repo.profiles)
	}
	if repo.erased["a"] || !repo.erased["b"] {
		t.Errorf("accounts recorded as erased: %v, want only b (a keeps statuses)", repo.erased)
	}
}
