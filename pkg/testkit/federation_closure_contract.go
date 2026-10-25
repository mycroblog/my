// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appinstance "github.com/mycroblog/my/internal/app/instance"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// fedClosure is the service closure of the closure's contract: whether the server delivers the
// closure's Deletes, and how many times delivery_completed was recorded.
type fedClosure struct {
	mu        sync.Mutex
	closing   bool
	completed int
}

func (c *fedClosure) ClosureRetracts(context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closing, nil
}

func (c *fedClosure) complete(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completed++
	c.closing = false
	return nil
}

// fedClosureAccounts pages the local accounts of fedAccounts, and tells the moved ones.
type fedClosureAccounts struct {
	*fedAccounts
	moved map[string]string
}

func (a fedClosureAccounts) LocalAccountIDs(_ context.Context, after string, limit int) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []string
	for id, actor := range a.actors {
		if actor.Account.IsLocal() && id > after {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids[:min(limit, len(ids))], nil
}

func (a fedClosureAccounts) MoveState(_ context.Context, accountID string) (appaccount.MoveState, error) {
	target, ok := a.moved[accountID]
	if !ok {
		return appaccount.MoveState{}, fmt.Errorf("move of %s: %w", accountID, appaccount.ErrNotFound)
	}
	return appaccount.MoveState{AccountID: accountID, TargetAccountID: target}, nil
}

func (a fedClosureAccounts) FindMove(_ context.Context, id string) (appaccount.Move, error) {
	return appaccount.Move{}, fmt.Errorf("move %s: %w", id, appaccount.ErrNotFound)
}

// fedDeliversTheClosureDeletes runs the closure's deliveries (ADR-0088 §3) over the repository:
// a page of two accounts at a time, the moved account last, the instance actor and the deleted
// account left out; with the federation mode off, the Deletes still reach the followers and the
// shared inboxes, but not a suspended domain; a page waits for the fan-outs of the earlier ones
// and for the backlog; running a page again records nothing twice; and delivery_completed is
// recorded once every delivery has ended.
func fedDeliversTheClosureDeletes(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	closure := &fedClosure{closing: true}
	a, b, c, moved := e.local(), e.local(), e.local(), e.local()
	gone := e.local()
	e.accounts.mu.Lock()
	actor := e.accounts.actors[gone]
	actor.Lifecycle.State = account.StateDeleted
	e.accounts.actors[gone] = actor
	e.accounts.mu.Unlock()
	e.instanceActor()
	target := e.remote("new.example", true)
	accounts := fedClosureAccounts{fedAccounts: e.accounts, moved: map[string]string{moved: target}}
	e.follow(e.remote("a.example", true), a)
	e.follow(e.remote("blocked.example", true), a)
	e.domains["blocked.example"] = true
	e.follow(e.remote("a.example", false), moved)

	handlers := &queue.JobHandlerRegistry{}
	appfederation.NewFanOut(appfederation.FanOutDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Federation, Outbox: e.outbox,
		Statuses: e.statuses, Accounts: e.accounts, Graph: e.graph, Domains: e.domains, Parts: e.parts,
		Suspensions: e.suspensions, Moves: accounts,
		Modes: appfederation.FixedMode(federation.ModeOff), Closure: closure,
		Clock: e.clock, IDs: e.ids, Host: contractHost, AccountDomain: contractAccountDomain, Batch: 2,
	}).RegisterJobs(handlers)
	deletes := appfederation.NewClosureDeletes(appfederation.ClosureDeletesDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Federation, Outbox: e.outbox, Accounts: accounts, Moves: accounts,
		Closure: closure, Complete: closure.complete,
		Clock: e.clock, IDs: e.ids, Host: contractHost, Page: 2, Backlog: 100, Wait: time.Minute,
	})
	deletes.RegisterJobs(handlers, queue.NewScheduler(e.jobs, e.clock, nil))
	page, _, _ := handlers.Lookup(appinstance.JobClosureFanOut)
	fanOut, _, _ := handlers.Lookup(appfederation.JobFanOut)

	// step runs once the jobs of type typ that are pending now, and keeps the others pending; it
	// returns how many it ran.
	step := func(typ string) int {
		t.Helper()
		h := fanOut
		if typ == appinstance.JobClosureFanOut {
			h = page
		}
		e.jobs.mu.Lock()
		now := e.jobs.pending
		e.jobs.pending = nil
		e.jobs.mu.Unlock()
		ran := 0
		for _, nj := range now {
			if nj.Type != typ {
				e.jobs.mu.Lock()
				e.jobs.pending = append(e.jobs.pending, nj)
				e.jobs.mu.Unlock()
				continue
			}
			j, err := nj.Build(e.ids.NewID(), e.clock.Now())
			if err != nil {
				t.Fatal(err)
			}
			if r := h(ctx, j); !r.IsDone() {
				t.Fatalf("%s %s = %+v (%v), want Done", nj.Type, j.Payload, r, r.Err())
			}
			ran++
		}
		return ran
	}
	fanOuts := func() {
		t.Helper()
		for n := 0; step(appfederation.JobFanOut) > 0; n++ {
			if n > 100 {
				t.Fatal("the fan-outs do not end")
			}
		}
	}
	// pages returns the pending closure pages and how many times each has waited.
	pages := func() (jobs []queue.NewJob, waits []int) {
		t.Helper()
		e.jobs.mu.Lock()
		defer e.jobs.mu.Unlock()
		for _, nj := range e.jobs.pending {
			if nj.Type != appinstance.JobClosureFanOut {
				continue
			}
			var p struct {
				Waits int `json:"waits"`
			}
			if err := json.Unmarshal(nj.Payload, &p); err != nil {
				t.Fatal(err)
			}
			jobs, waits = append(jobs, nj), append(waits, p.Waits)
		}
		return jobs, waits
	}
	deleteOf := func(id string) (federation.OutboundActivity, bool) {
		t.Helper()
		all, err := e.h.Federation.ListOutboundActivitiesOf(ctx, federation.OutboundAccount, id)
		if err != nil {
			t.Fatal(err)
		}
		var out []federation.OutboundActivity
		for _, act := range all {
			if act.Type == federation.ActivityDelete {
				out = append(out, act)
			}
		}
		if len(out) > 1 {
			t.Fatalf("%d Deletes of %s, want at most 1", len(out), id)
		}
		if len(out) == 0 {
			return federation.OutboundActivity{}, false
		}
		return out[0], true
	}

	// The first page records the Deletes of the first two accounts in ID order, which did not
	// move, and enqueues their fan-outs and the next page.
	if err := e.jobs.Enqueue(ctx, closureFirstPage(t)); err != nil {
		t.Fatal(err)
	}
	step(appinstance.JobClosureFanOut)
	for id, want := range map[string]bool{a: true, b: true, c: false, moved: false} {
		if _, ok := deleteOf(id); ok != want {
			t.Errorf("after the first page, a Delete of %s = %v, want %v", id, ok, want)
		}
	}
	if jobs, waits := pages(); len(jobs) != 1 || waits[0] != 0 {
		t.Fatalf("after the first page: pages %v, want the next one", waits)
	}
	// The next page waits for the fan-outs of the first.
	step(appinstance.JobClosureFanOut)
	if jobs, waits := pages(); len(jobs) != 1 || waits[0] != 1 || jobs[0].RunAt.IsZero() {
		t.Fatalf("the page before the fan-outs: pages %v, want one that waits", waits)
	}
	if _, ok := deleteOf(c); ok {
		t.Fatal("a page that waits recorded a Delete")
	}
	progress, err := deletes.Progress(ctx)
	if err != nil || progress.Accounts != 2 || progress.FanOuts != 2 {
		t.Fatalf("progress before the fan-outs = %+v, %v; want 2 accounts and 2 fan-outs", progress, err)
	}

	// The fan-out sends the Deletes in mode off: to the followers and the shared inboxes, not to
	// the suspended domain.
	fanOuts()
	del, _ := deleteOf(a)
	if got := e.inboxes(t, del.ID); !slices.Equal(got, []string{"https://a.example/inbox", "https://new.example/inbox"}) {
		t.Errorf("inboxes of the closure's Delete of a = %v, want a.example and new.example, not blocked.example", got)
	}
	if progress, err = deletes.Progress(ctx); err != nil || progress.FanOuts != 0 || progress.Pending() == 0 {
		t.Fatalf("progress after the fan-outs = %+v, %v", progress, err)
	}
	// Over the backlog, the page waits again.
	tight := appfederation.NewClosureDeletes(appfederation.ClosureDeletesDeps{
		UnitOfWork: e.h.UnitOfWork, Repository: e.h.Federation, Outbox: e.outbox, Accounts: accounts, Moves: accounts,
		Closure: closure, Clock: e.clock, IDs: e.ids, Host: contractHost, Page: 2, Backlog: progress.Pending() - 1,
	})
	tightHandlers := &queue.JobHandlerRegistry{}
	tight.RegisterJobs(tightHandlers, queue.NewScheduler(e.jobs, e.clock, nil))
	tightPage, _, _ := tightHandlers.Lookup(appinstance.JobClosureFanOut)
	waiting, _ := pages()
	j, err := waiting[0].Build(e.ids.NewID(), e.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r := tightPage(ctx, j); !r.IsDone() {
		t.Fatalf("page over the backlog = %+v", r)
	}
	if _, waits := pages(); !slices.Equal(waits, []int{1, 2}) {
		t.Fatalf("pages after the page over the backlog = %v, want it again later", waits)
	}
	if _, ok := deleteOf(c); ok {
		t.Fatal("a page over the backlog recorded a Delete")
	}
	e.jobs.mu.Lock()
	e.jobs.pending = e.jobs.pending[:len(e.jobs.pending)-1]
	e.jobs.mu.Unlock()

	// Under the backlog, the pages go on to the end: the last account that did not move, then
	// the moved one, in a pass of its own.
	for n := 0; step(appinstance.JobClosureFanOut) > 0; n++ {
		if n > 50 {
			t.Fatal("the pages do not end")
		}
		fanOuts()
	}
	for _, id := range []string{a, b, c, moved} {
		if _, ok := deleteOf(id); !ok {
			t.Errorf("no closure Delete of %s", id)
		}
	}
	if _, ok := deleteOf(gone); ok {
		t.Error("the deleted account got a closure Delete")
	}
	movedDel, _ := deleteOf(moved)
	for _, id := range []string{a, b, c} {
		if d, _ := deleteOf(id); d.ID >= movedDel.ID {
			t.Errorf("the Delete of %s was recorded after the moved account's", id)
		}
	}

	// Running the first page again records nothing more.
	if err := e.jobs.Enqueue(ctx, closurePageAgain(t)); err != nil {
		t.Fatal(err)
	}
	step(appinstance.JobClosureFanOut)
	for _, id := range []string{a, b, c, moved} {
		deleteOf(id) // fails on a second Delete
	}

	// delivery_completed waits for every delivery to end.
	if err := deletes.Complete(ctx); err != nil || closure.completed != 0 {
		t.Fatalf("Complete before the deliveries ended: %v, completed %d", err, closure.completed)
	}
	outcomes := []federation.DeliveryOutcome{federation.DeliverySucceeded, federation.DeliveryPermanentFailure, federation.DeliverySkipped}
	n := 0
	for _, id := range []string{a, b, c, moved} {
		del, _ := deleteOf(id)
		ds, err := e.h.Federation.ListDeliveries(ctx, del.ID, "", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range ds {
			attempts := []federation.DeliveryOutcome{outcomes[n%len(outcomes)]}
			if n == 0 {
				// The first one uses up its attempts: a dead job.
				attempts = slices.Repeat([]federation.DeliveryOutcome{federation.DeliveryRetryableFailure}, appfederation.DeliveryRetryPolicy.MaxAttempts)
			}
			for _, o := range attempts {
				e.clock.Advance(time.Millisecond)
				err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
					return e.h.Federation.InsertDeliveryAttempt(ctx, tx, federation.DeliveryAttempt{
						ID: e.ids.NewID(), DeliveryID: d.ID, Outcome: o, AttemptedAt: e.clock.Now(),
					})
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			n++
		}
	}
	progress, err = deletes.Progress(ctx)
	if err != nil || progress.Accounts != 4 || progress.FanOuts != 0 || progress.Pending() != 0 || progress.Deliveries[federation.ClosureDeliveryDead] != 1 {
		t.Fatalf("progress after the deliveries = %+v, %v; want 4 accounts, nothing pending, one dead", progress, err)
	}
	total := 0
	for _, v := range progress.Deliveries {
		total += v
	}
	if total != n {
		t.Errorf("progress counts %d deliveries, want %d", total, n)
	}
	if err := deletes.Complete(ctx); err != nil || closure.completed != 1 {
		t.Fatalf("Complete after the deliveries ended: %v, completed %d", err, closure.completed)
	}
	if err := deletes.Complete(ctx); err != nil || closure.completed != 1 {
		t.Fatalf("Complete once completed: %v, completed %d", err, closure.completed)
	}
}

// closureFirstPage returns the first page of the closure's fan-out, as the closure enqueues it.
func closureFirstPage(t *testing.T) queue.NewJob {
	t.Helper()
	return queue.NewJob{Type: appinstance.JobClosureFanOut, Payload: json.RawMessage(`{}`), IdempotencyKey: "closure:first"}
}

// closurePageAgain returns the first page once more, with a key of its own.
func closurePageAgain(t *testing.T) queue.NewJob {
	t.Helper()
	return queue.NewJob{Type: appinstance.JobClosureFanOut, Payload: json.RawMessage(`{}`), IdempotencyKey: "closure:again"}
}

// fedCountsPendingMoves counts the Moves still being sent: recorded since the given time, with a
// fan-out that has not ended or a delivery still to be attempted.
func fedCountsPendingMoves(t *testing.T, e *fedEnv) {
	ctx := t.Context()
	author := e.local()
	since := e.clock.Now()
	move := func() federation.OutboundActivity {
		t.Helper()
		e.clock.Advance(time.Second)
		act := federation.OutboundActivity{
			ID: e.ids.NewID(), Type: federation.ActivityMove, ActorAccountID: author,
			ObjectKind: federation.OutboundMove, ObjectID: e.ids.NewID(), CreatedAt: e.clock.Now(),
		}
		act.ActivityURI = federation.IRI("https://" + contractHost + "/ap/activities/" + act.ID)
		if err := e.h.UnitOfWork.Tx(ctx, func(tx uow.Tx) error { return e.h.Federation.InsertOutboundActivity(ctx, tx, act) }); err != nil {
			t.Fatal(err)
		}
		return act
	}
	count := func(since time.Time) int {
		t.Helper()
		n, err := e.h.Federation.CountPendingMoves(ctx, since, 3)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	tx := func(f func(tx uow.Tx) error) {
		t.Helper()
		if err := e.h.UnitOfWork.Tx(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	m := move()
	if got := count(since); got != 1 {
		t.Fatalf("a Move before its fan-out ended: %d pending, want 1", got)
	}
	d := federation.Delivery{ID: e.ids.NewID(), OutboundActivityID: m.ID, InboxURL: "https://a.example/inbox", Domain: "a.example", CreatedAt: e.clock.Now()}
	tx(func(tx uow.Tx) error {
		if _, err := e.h.Federation.InsertDelivery(ctx, tx, d); err != nil {
			return err
		}
		return e.h.Federation.InsertFanOutCompletion(ctx, tx, m.ID, e.clock.Now())
	})
	if got := count(since); got != 1 {
		t.Fatalf("a Move with an unattempted delivery: %d pending, want 1", got)
	}
	for i := range 3 {
		if got := count(since); got != 1 {
			t.Fatalf("a Move after %d failed attempts: %d pending, want 1", i, got)
		}
		e.clock.Advance(time.Millisecond)
		tx(func(tx uow.Tx) error {
			return e.h.Federation.InsertDeliveryAttempt(ctx, tx, federation.DeliveryAttempt{
				ID: e.ids.NewID(), DeliveryID: d.ID, Outcome: federation.DeliveryRetryableFailure, AttemptedAt: e.clock.Now(),
			})
		})
	}
	if got := count(since); got != 0 {
		t.Fatalf("a Move whose delivery used up its attempts: %d pending, want 0", got)
	}
	// A Move recorded before the time is not counted.
	move()
	e.clock.Advance(time.Second)
	if got := count(e.clock.Now()); got != 0 {
		t.Fatalf("Moves since now: %d, want 0", got)
	}
	if got := count(since); got != 1 {
		t.Fatalf("Moves since the start: %d, want 1", got)
	}
	// Recording the end of a fan-out again keeps the first.
	tx(func(tx uow.Tx) error { return e.h.Federation.InsertFanOutCompletion(ctx, tx, m.ID, e.clock.Now()) })
}
