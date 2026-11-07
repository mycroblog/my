// SPDX-License-Identifier: AGPL-3.0-only

package identitytest

import (
	"errors"
	"sync"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/pkg/testkit"
)

// accountExecutesDeletions checks the deletions that moderation schedules and their execution
// (data-model §4.2, §8): a ban's deletion_scheduled names the action and keeps an earlier due
// time, a remote account takes none, and ExecuteDueDeletions deletes the accounts whose
// deletion is due, once, leaving the others and the canceled ones; CancelDeletionByModeration
// cancels a scheduled deletion once, refuses a deleted account, and races the job with exactly
// one winner.
func accountExecutesDeletions(t *testing.T, e *AccountEnv) {
	ctx := t.Context()
	banned, later, canceled := e.Register(t, "banned"), e.Register(t, "later"), e.Register(t, "canceled")
	schedule := func(id, action string, due time.Time) {
		t.Helper()
		err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			return e.Accounts.ScheduleDeletionByModeration(ctx, tx, id, action, due)
		})
		if err != nil {
			t.Fatalf("ScheduleDeletionByModeration(%s) = %v", id, err)
		}
	}
	state := func(id string) account.LifecycleState {
		t.Helper()
		a, err := e.Accounts.FindByID(ctx, id)
		if err != nil {
			t.Fatalf("FindByID = %v", err)
		}
		return a.Lifecycle
	}
	day := 24 * time.Hour
	due := e.Clock.Now().Add(30 * day)
	schedule(banned.AccountID, testkit.NamedID("action-1"), due)
	// A later due time keeps the earlier one; an earlier one replaces it.
	schedule(banned.AccountID, testkit.NamedID("action-2"), due.Add(day))
	if s := state(banned.AccountID); s.State != account.StateDeletionScheduled || s.DeletionDueAt == nil || !s.DeletionDueAt.Equal(due) {
		t.Errorf("state after a later schedule = %+v, want due %v", s, due)
	}
	schedule(later.AccountID, testkit.NamedID("action-3"), due.Add(day))
	schedule(canceled.AccountID, testkit.NamedID("action-4"), due)
	cancel := func(id string) (bool, error) {
		t.Helper()
		var ok bool
		err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			ok, err = e.Accounts.CancelDeletionByModeration(ctx, tx, id, testkit.NamedID("action-4"), e.IDs.NewID())
			return err
		})
		return ok, err
	}
	// An overturned appeal of a ban cancels the deletion once; an account without a scheduled
	// deletion is left as it is.
	if ok, err := cancel(canceled.AccountID); err != nil || !ok {
		t.Errorf("CancelDeletionByModeration = %v, %v; want true", ok, err)
	}
	if ok, err := cancel(canceled.AccountID); err != nil || ok {
		t.Errorf("CancelDeletionByModeration again = %v, %v; want false", ok, err)
	}
	err := e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		events, err := e.H.Accounts.ListLifecycleEvents(ctx, tx)
		for _, ev := range events {
			if ev.AccountID == banned.AccountID && (ev.ModerationActionID != testkit.NamedID("action-1") || ev.InitiatedBy != account.InitiatedByModerator) {
				t.Errorf("event of the ban = %+v, want one by a moderator naming action-1", ev)
			}
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	run := func(want int) {
		t.Helper()
		if n, err := e.Accounts.ExecuteDueDeletions(ctx); err != nil || n != want {
			t.Errorf("ExecuteDueDeletions = %d, %v; want %d", n, err, want)
		}
	}
	run(0)
	e.Clock.Advance(30 * day)
	run(1)
	if s := state(banned.AccountID); s.State != account.StateDeleted {
		t.Errorf("banned account after its due time = %+v, want deleted", s)
	}
	if s := state(later.AccountID); s.State != account.StateDeletionScheduled {
		t.Errorf("account due later = %+v, want deletion_scheduled", s)
	}
	run(0)
	e.Clock.Advance(day)
	run(1)
	if s := state(canceled.AccountID); s.State != account.StateActive {
		t.Errorf("canceled account = %+v, want active", s)
	}
	// A deleted account stays deleted, and its deletion is not canceled.
	if _, err := cancel(banned.AccountID); !errors.Is(err, appaccount.ErrAlreadyDeleted) {
		t.Errorf("CancelDeletionByModeration(deleted) = %v, want ErrAlreadyDeleted", err)
	}
	schedule(banned.AccountID, testkit.NamedID("action-5"), e.Clock.Now().Add(day))
	if s := state(banned.AccountID); s.State != account.StateDeleted {
		t.Errorf("deleted account after a schedule = %+v, want deleted", s)
	}

	// A cancellation that races the deletion job: exactly one of them wins.
	racing := e.Register(t, "racing")
	schedule(racing.AccountID, testkit.NamedID("action-7"), e.Clock.Now())
	var (
		wg       sync.WaitGroup
		n        int
		runErr   error
		ok       bool
		cancelEr error
	)
	wg.Add(2)
	go func() { defer wg.Done(); n, runErr = e.Accounts.ExecuteDueDeletions(ctx) }()
	go func() {
		defer wg.Done()
		cancelEr = e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			var err error
			ok, err = e.Accounts.CancelDeletionByModeration(ctx, tx, racing.AccountID, testkit.NamedID("action-7"), e.IDs.NewID())
			return err
		})
	}()
	wg.Wait()
	switch s := state(racing.AccountID); {
	case runErr != nil:
		t.Errorf("ExecuteDueDeletions while cancelling = %v", runErr)
	case s.State == account.StateDeleted && (n != 1 || !errors.Is(cancelEr, appaccount.ErrAlreadyDeleted)):
		t.Errorf("deleted while cancelling: deleted %d, cancel %v, %v", n, ok, cancelEr)
	case s.State == account.StateActive && (n != 0 || cancelEr != nil || !ok):
		t.Errorf("canceled while deleting: deleted %d, cancel %v, %v", n, ok, cancelEr)
	case s.State != account.StateDeleted && s.State != account.StateActive:
		t.Errorf("state after the race = %+v", s)
	}

	remote := account.Account{
		ID: e.IDs.NewID(), URI: "https://remote.example/users/r", Domain: "remote.example",
		ActorType: account.ActorPerson, CreatedAt: e.Clock.Now(),
	}
	err = e.H.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if err := e.H.Accounts.InsertAccount(ctx, tx, remote); err != nil {
			return err
		}
		return e.Accounts.ScheduleDeletionByModeration(ctx, tx, remote.ID, testkit.NamedID("action-6"), e.Clock.Now().Add(day))
	})
	if !errors.Is(err, appaccount.ErrNotLocal) {
		t.Errorf("ScheduleDeletionByModeration(remote) = %v, want ErrNotLocal", err)
	}
}
