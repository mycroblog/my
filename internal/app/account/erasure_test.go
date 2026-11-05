// SPDX-License-Identifier: AGPL-3.0-only

package account_test

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	domainaccount "github.com/mycroblog/my/internal/domain/account"
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

// stubRecheck answers RecheckDeletion from a map and counts the questions.
type stubRecheck struct {
	got   map[string]account.DeletionRecheck
	asked map[string]int
}

func (r stubRecheck) RecheckDeletion(_ context.Context, id string) (account.DeletionRecheck, error) {
	r.asked[id]++
	return r.got[id], nil
}

// TestErasureAsksBeforeErasingARemoteActor: a confirmed actor goes, a restored one stays, a local
// account is not asked about, and an open question holds until ErasureRecheckLimit.
func TestErasureAsksBeforeErasingARemoteActor(t *testing.T) {
	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	repo := &erasureRepo{
		targets: []account.ErasureTarget{
			{AccountID: "confirmed", DeletedAt: now.Add(-account.ErasureDelay)},
			{AccountID: "local", UserID: "u", DeletedAt: now.Add(-account.ErasureDelay)},
			{AccountID: "open", DeletedAt: now.Add(-account.ErasureRecheckLimit + time.Second)},
			{AccountID: "overdue", DeletedAt: now.Add(-account.ErasureRecheckLimit)},
			{AccountID: "restored", DeletedAt: now.Add(-account.ErasureRecheckLimit)},
		},
		profiles: map[string]bool{}, erased: map[string]bool{},
	}
	recheck := stubRecheck{asked: map[string]int{}, got: map[string]account.DeletionRecheck{
		"confirmed": account.RecheckConfirmed, "restored": account.RecheckRestored,
		"open": account.RecheckInconclusive, "overdue": account.RecheckInconclusive,
	}}
	er := account.NewEraser(account.ErasureDeps{UnitOfWork: directUOW{}, Repository: repo, Clock: fixedClock{now}, Recheck: recheck})
	if _, err := er.Run(t.Context(), account.ErasureCursor{}); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"confirmed": true, "local": true, "open": false, "overdue": true, "restored": false} {
		if repo.erased[id] != want {
			t.Errorf("%s erased = %v, want %v", id, repo.erased[id], want)
		}
	}
	if recheck.asked["local"] != 0 {
		t.Errorf("asked about a local account %d times", recheck.asked["local"])
	}
}

// TestErasureAsksOnceForAnAccountItIsErasing: a job that goes on inside an account does not ask
// again, and the questions count against the transactions of the job.
func TestErasureAsksOnceForAnAccountItIsErasing(t *testing.T) {
	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	repo := &erasureRepo{targets: []account.ErasureTarget{{AccountID: "a"}}, profiles: map[string]bool{}, erased: map[string]bool{}}
	recheck := stubRecheck{asked: map[string]int{}, got: map[string]account.DeletionRecheck{"a": account.RecheckConfirmed}}
	er := account.NewEraser(account.ErasureDeps{
		UnitOfWork: directUOW{}, Repository: repo, Clock: fixedClock{now}, Batch: 1, Recheck: recheck,
		Erasers: []account.DataEraser{retainedEraser{items: map[string]int{"a": 160}}},
	})
	cur := account.ErasureCursor{}
	for run := 0; ; run++ {
		if run == 10 {
			t.Fatal("the erasure did not end")
		}
		res, err := er.Run(t.Context(), cur)
		if err != nil {
			t.Fatal(err)
		}
		if !res.More {
			break
		}
		cur = res.Next
	}
	if recheck.asked["a"] != 1 {
		t.Errorf("asked %d times, want 1", recheck.asked["a"])
	}
}

// TestErasureGoesOnAfterItsBudgetEndsAtAQuestion: an account that the budget left unasked is asked
// about by the next job.
func TestErasureGoesOnAfterItsBudgetEndsAtAQuestion(t *testing.T) {
	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	repo := &erasureRepo{profiles: map[string]bool{}, erased: map[string]bool{}}
	recheck := stubRecheck{asked: map[string]int{}, got: map[string]account.DeletionRecheck{}}
	for i := range 60 {
		id := "a" + strconv.Itoa(100+i)
		repo.targets = append(repo.targets, account.ErasureTarget{AccountID: id})
		recheck.got[id] = account.RecheckRestored
	}
	er := account.NewEraser(account.ErasureDeps{UnitOfWork: directUOW{}, Repository: repo, Clock: fixedClock{now}, Recheck: recheck})
	res, err := er.Run(t.Context(), account.ErasureCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.More || len(recheck.asked) >= 60 {
		t.Fatalf("first job: more = %v, asked about %d, want it to stop within its budget", res.More, len(recheck.asked))
	}
	if res, err = er.Run(t.Context(), res.Next); err != nil || res.More {
		t.Fatalf("second job: %+v, %v", res, err)
	}
	if len(recheck.asked) != 60 {
		t.Errorf("asked about %d accounts, want 60", len(recheck.asked))
	}
}

// lifecycleRepo holds only the lifecycle events that ReplayLifecycle reads.
type lifecycleRepo struct {
	account.Repository
	events []domainaccount.LifecycleEvent
}

func (r lifecycleRepo) ListLifecycleEvents(context.Context, uow.Tx) ([]domainaccount.LifecycleEvent, error) {
	return r.events, nil
}

// A rebuild must not hide an actor that was restored after a 410, nor miss one marked again.
func TestReplayLifecycleLeavesOutRestoredActors(t *testing.T) {
	ev := func(id, acct string, kind domainaccount.LifecycleKind, by domainaccount.Initiator) domainaccount.LifecycleEvent {
		return domainaccount.LifecycleEvent{ID: id, AccountID: acct, Kind: kind, InitiatedBy: by}
	}
	repo := lifecycleRepo{events: []domainaccount.LifecycleEvent{
		ev("1", "again", domainaccount.LifecycleRemoteDeleted, domainaccount.InitiatedBySystem),
		ev("2", "again", domainaccount.LifecycleReactivated, domainaccount.InitiatedBySystem),
		ev("3", "again", domainaccount.LifecycleRemoteDeleted, domainaccount.InitiatedByRemote),
		ev("4", "restored", domainaccount.LifecycleRemoteDeleted, domainaccount.InitiatedBySystem),
		ev("5", "restored", domainaccount.LifecycleReactivated, domainaccount.InitiatedBySystem),
	}}
	var replayed []string
	err := account.ReplayLifecycle(t.Context(), noTx{}, repo, func(e uow.Event) error {
		replayed = append(replayed, e.(account.LifecycleEventRecorded).Event.ID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1", "3"}; !slices.Equal(replayed, want) {
		t.Errorf("replayed %v, want %v", replayed, want)
	}
}

func TestLifecycleEventRestoresRemoteActorIsNotAReactivation(t *testing.T) {
	restore := account.LifecycleEventRecorded{Event: domainaccount.LifecycleEvent{Kind: domainaccount.LifecycleReactivated, InitiatedBy: domainaccount.InitiatedBySystem}}
	if restore.Reactivates() || !restore.RestoresRemoteActor() {
		t.Errorf("restore: Reactivates %v, RestoresRemoteActor %v", restore.Reactivates(), restore.RestoresRemoteActor())
	}
	own := account.LifecycleEventRecorded{Event: domainaccount.LifecycleEvent{Kind: domainaccount.LifecycleReactivated, InitiatedBy: domainaccount.InitiatedBySelf}}
	if !own.Reactivates() || own.RestoresRemoteActor() {
		t.Errorf("own: Reactivates %v, RestoresRemoteActor %v", own.Reactivates(), own.RestoresRemoteActor())
	}
}
