// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

const (
	// JobExecuteDeletions deletes the accounts whose scheduled deletion is due (data-model
	// §4.2, §8): a ban's, or that of an account whose owner deactivated it DeactivationPeriod ago
	// (ADR-0145 §2).
	JobExecuteDeletions = "account.execute_deletions"
	// DeletionInterval is the period of the JobExecuteDeletions schedule.
	DeletionInterval = time.Hour
	// deletionBatch bounds the accounts that one run deletes; the next run takes the rest.
	deletionBatch = 100
)

// ScheduleDeletionByModeration records inside tx that a moderation action (a ban) schedules
// the deletion of a local account at due: a deletion_scheduled lifecycle event initiated by a
// moderator that names the action (data-model §4.2). An account that is deleted already, or
// whose deletion is due no later than due, is left as it is. A remote account fails with
// ErrNotLocal.
func (s *Service) ScheduleDeletionByModeration(ctx context.Context, tx uow.Tx, accountID, actionID string, due time.Time) error {
	a, err := s.d.Repository.Account(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if !a.IsLocal() {
		return fmt.Errorf("account %s: %w", accountID, ErrNotLocal)
	}
	due = kernel.Normalize(due)
	st, err := s.d.Repository.LifecycleState(ctx, tx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return err
	case st.State == account.StateDeleted:
		return nil
	case st.State == account.StateDeletionScheduled && st.DeletionDueAt != nil && !st.DeletionDueAt.After(due):
		return nil
	}
	e := account.LifecycleEvent{
		ID: s.d.IDs.NewID(), AccountID: accountID, Kind: account.LifecycleDeletionScheduled,
		InitiatedBy: account.InitiatedByModerator, ModerationActionID: actionID, DeletionDueAt: &due,
		OccurredAt: s.d.Clock.Now(),
	}
	if err := s.d.Repository.InsertLifecycleEvent(ctx, tx, e); err != nil {
		return err
	}
	return tx.Emit(ctx, LifecycleEventRecorded{Event: e})
}

// ErrAlreadyDeleted reports a change to the lifecycle of an account that is deleted already.
var ErrAlreadyDeleted = errors.New("account: the account is deleted")

// CancelDeletionByModeration records inside tx that a moderation decision (an overturned appeal
// of a ban) cancels the scheduled deletion of an account: a deletion_cancelled lifecycle event
// initiated by a moderator that names the action, with ID eventID (data-model §4.2). It reports
// whether it recorded one: an account whose deletion is not scheduled is left as it is. A deleted
// account fails with ErrAlreadyDeleted. The state is read with a lock where the database takes
// one, so the deletion job and the cancellation never both win.
func (s *Service) CancelDeletionByModeration(ctx context.Context, tx uow.Tx, accountID, actionID, eventID string) (bool, error) {
	st, err := s.d.Repository.LifecycleState(ctx, tx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	case st.State == account.StateDeleted:
		return false, fmt.Errorf("account %s: %w", accountID, ErrAlreadyDeleted)
	case st.State != account.StateDeletionScheduled:
		return false, nil
	}
	e := account.LifecycleEvent{
		ID: eventID, AccountID: accountID, Kind: account.LifecycleDeletionCancelled,
		InitiatedBy: account.InitiatedByModerator, ModerationActionID: actionID, OccurredAt: kernel.Normalize(s.d.Clock.Now()),
	}
	if err := s.d.Repository.InsertLifecycleEvent(ctx, tx, e); err != nil {
		return false, err
	}
	return true, tx.Emit(ctx, LifecycleEventRecorded{Event: e})
}

// DeleteBySelf deletes a local account at its owner's request (FR-008): a deleted lifecycle event
// initiated by the account itself, recorded at once without a waiting period, which hides the
// account and its statuses, ends its user's sessions and tokens, and sends the Delete of the
// actor (federation.md §7.1) in the same transaction. Its data is erased ErasureDelay later
// (data-model §8). An account that is deleted already fails with ErrAlreadyDeleted, and a
// remote account with ErrNotLocal. The state is read with a lock where the database takes one,
// so that a concurrent deletion records one event only.
func (s *Service) DeleteBySelf(ctx context.Context, accountID string) error {
	return s.DeleteBySelfGuarded(ctx, accountID, nil)
}

// DeleteBySelfGuarded is DeleteBySelf with guard, which runs first in its transaction and refuses
// the deletion by failing; a nil guard refuses nothing.
func (s *Service) DeleteBySelfGuarded(ctx context.Context, accountID string, guard func(tx uow.Tx) error) error {
	a, err := s.d.Repository.FindAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if !a.IsLocal() || a.ActorType == account.ActorApplication {
		return fmt.Errorf("account %s: %w", accountID, ErrNotLocal)
	}
	id := s.d.IDs.NewID()
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		if guard != nil {
			if err := guard(tx); err != nil {
				return err
			}
		}
		st, err := s.d.Repository.LifecycleState(ctx, tx, accountID)
		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return err
		case st.State == account.StateDeleted:
			return fmt.Errorf("account %s: %w", accountID, ErrAlreadyDeleted)
		}
		e := account.LifecycleEvent{
			ID: id, AccountID: accountID, Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySelf,
			OccurredAt: kernel.Normalize(s.d.Clock.Now()),
		}
		if err := s.d.Repository.InsertLifecycleEvent(ctx, tx, e); err != nil {
			return err
		}
		return tx.Emit(ctx, LifecycleEventRecorded{Event: e})
	})
}

// RegisterDeletionJobs registers the JobExecuteDeletions handler and its hourly schedule.
func (s *Service) RegisterDeletionJobs(reg *queue.JobHandlerRegistry, sched *queue.Scheduler) {
	reg.Register(JobExecuteDeletions, func(ctx context.Context, _ queue.Job) queue.Result {
		if _, err := s.ExecuteDueDeletions(ctx); err != nil {
			return queue.Retry(0, err)
		}
		return queue.Done()
	})
	sched.Add(queue.Schedule{Name: JobExecuteDeletions, Every: DeletionInterval, Job: queue.NewJob{Type: JobExecuteDeletions}})
}

// DeletionHold reports, in tx, whether the deletion of a deactivated account must wait: it would
// leave the instance without an administrator who can sign in. It takes the lock of the admin role.
type DeletionHold func(ctx context.Context, tx uow.Tx, accountID string) (bool, error)

// SetDeletionHold sets the hold of ExecuteDueDeletions. The identity module, which knows the
// administrators, is built after this one, so the composition root sets it then; without a hold,
// nothing waits. It is not safe to call once the jobs run.
func (s *Service) SetDeletionHold(hold DeletionHold) { s.deletionHold = hold }

// ExecuteDueDeletions deletes up to deletionBatch accounts of each kind that are due: those whose
// scheduled deletion is due, and those deactivated for account.DeactivationPeriod (ADR-0145 §2).
// Each gets a deleted lifecycle event initiated by the system, in a short transaction of its own,
// which hides it and its statuses at once and sends the Delete of the actor (federation.md §7.1).
// The state is read again under its lock, and an account that was reactivated, or whose deletion
// was cancelled or rescheduled meanwhile, is left as it is, so running it again is safe. A
// deactivated account that DeletionHold holds back stays as it is and is logged, and the next run
// tries again. It returns how many accounts it deleted.
func (s *Service) ExecuteDueDeletions(ctx context.Context) (int, error) {
	now := kernel.Normalize(s.d.Clock.Now())
	due, err := s.d.Repository.ListDueDeletions(ctx, now, deletionBatch)
	if err != nil {
		return 0, err
	}
	deactivated, err := s.d.Repository.ListDueDeactivations(ctx, now.Add(-account.DeactivationPeriod), deletionBatch)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, d := range append(due, deactivated...) {
		id := s.d.IDs.NewID()
		var done bool
		err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
			done = false
			// The lock of the admin role comes before that of the state, as in the other paths
			// that remove an administrator.
			if d.Deactivated() && s.deletionHold != nil {
				switch held, err := s.deletionHold(ctx, tx, d.AccountID); {
				case err != nil:
					return err
				case held:
					slog.WarnContext(ctx, "account: the deletion of a deactivated account waits: it is the last administrator", slog.String("account", d.AccountID))
					return nil
				}
			}
			st, err := s.d.Repository.LifecycleState(ctx, tx, d.AccountID)
			if err != nil {
				return err
			}
			if !deletionDue(st, now) {
				return nil
			}
			e := account.LifecycleEvent{
				ID: id, AccountID: d.AccountID, Kind: account.LifecycleDeleted, InitiatedBy: account.InitiatedBySystem, OccurredAt: now,
			}
			if err := s.d.Repository.InsertLifecycleEvent(ctx, tx, e); err != nil {
				return err
			}
			done = true
			return tx.Emit(ctx, LifecycleEventRecorded{Event: e})
		})
		if err != nil {
			return deleted, fmt.Errorf("account: delete account %s: %w", d.AccountID, err)
		}
		if done {
			deleted++
		}
	}
	return deleted, nil
}

// deletionDue reports whether state st deletes its account at now.
func deletionDue(st account.LifecycleState, now time.Time) bool {
	if st.State == account.StateDeletionScheduled {
		return st.DeletionDueAt != nil && !st.DeletionDueAt.After(now)
	}
	return st.DeactivationExpired(now)
}
