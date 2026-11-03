// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// DeactivationPeriod and DeactivationGap are the domain's account.DeactivationPeriod and
// account.DeactivationGap.
const (
	DeactivationPeriod = account.DeactivationPeriod
	DeactivationGap    = account.DeactivationGap
)

// DeactivationRefusal says why an account cannot be deactivated (ADR-0145 §1, §8).
type DeactivationRefusal string

// Refusals of a deactivation.
const (
	// DeactivationRefusedRestricted is an account that moderation keeps from logging in.
	DeactivationRefusedRestricted DeactivationRefusal = "login_restricted"
	// DeactivationRefusedDeletionScheduled is an account waiting for its deletion, or deleted.
	DeactivationRefusedDeletionScheduled DeactivationRefusal = "deletion_scheduled"
	// DeactivationRefusedAlready is an account that is deactivated.
	DeactivationRefusedAlready DeactivationRefusal = "already_deactivated"
	// DeactivationRefusedMoved is an account that moved to another one.
	DeactivationRefusedMoved DeactivationRefusal = "moved"
	// DeactivationRefusedPending is an account whose registration waits for review.
	DeactivationRefusedPending DeactivationRefusal = "registration_pending"
	// DeactivationRefusedSubscription is an account with a paid subscription that goes on.
	DeactivationRefusedSubscription DeactivationRefusal = "paid_subscription"
	// DeactivationRefusedRecentlyReactivated is an account reactivated less than
	// account.DeactivationGap ago.
	DeactivationRefusedRecentlyReactivated DeactivationRefusal = "recently_reactivated"
)

// DeactivationRefusedError reports a deactivation that was refused.
type DeactivationRefusedError struct {
	Reason DeactivationRefusal
	// AvailableAt is when the account can be deactivated again; nil unless Reason is
	// DeactivationRefusedRecentlyReactivated.
	AvailableAt *time.Time
}

func (e *DeactivationRefusedError) Error() string {
	return "account: the deactivation is refused: " + string(e.Reason)
}

var (
	// ErrNotDeactivated reports a reactivation of an account that is not deactivated.
	ErrNotDeactivated = errors.New("account: the account is not deactivated")
	// ErrDeactivationExpired reports a reactivation of an account whose deadline has passed.
	ErrDeactivationExpired = errors.New("account: the deadline of the deactivation has passed")
)

// deactivationReads are the reads that decide a deactivation: outside transactions for the
// settings screen, in the transaction for the deactivation.
type deactivationReads struct {
	state        func(ctx context.Context, id string) (account.LifecycleState, error)
	move         func(ctx context.Context, id string) (account.MoveState, error)
	reactivation func(ctx context.Context, id string) (time.Time, error)
}

// refusal returns the error of an account that cannot be deactivated at now, nil for one that can.
func (r deactivationReads) refusal(ctx context.Context, accountID string, now time.Time) error {
	st, err := r.state(ctx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return err
	case st.Deactivated():
		return &DeactivationRefusedError{Reason: DeactivationRefusedAlready}
	case !st.Active():
		return &DeactivationRefusedError{Reason: DeactivationRefusedDeletionScheduled}
	}
	switch _, err := r.move(ctx, accountID); {
	case err == nil:
		return &DeactivationRefusedError{Reason: DeactivationRefusedMoved}
	case !errors.Is(err, ErrNotFound):
		return err
	}
	last, err := r.reactivation(ctx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	if available := last.Add(account.DeactivationGap); now.Before(available) {
		return &DeactivationRefusedError{Reason: DeactivationRefusedRecentlyReactivated, AvailableAt: &available}
	}
	return nil
}

// DeactivationRefused returns the *DeactivationRefusedError of a local account whose lifecycle,
// move, or last reactivation refuses its deactivation now; nil when none does. It only reads:
// DeactivateTx decides again.
func (s *Service) DeactivationRefused(ctx context.Context, accountID string) error {
	repo := s.d.Repository
	return deactivationReads{repo.FindLifecycleState, repo.FindMoveState, repo.FindLatestReactivation}.
		refusal(ctx, accountID, s.d.Clock.Now())
}

// DeactivateTx records inside tx that the owner of a local account deactivates it: a deactivated
// lifecycle event initiated by the account itself, which hides it and its statuses and starts the
// DeactivationPeriod that ends in its deletion (ADR-0145). The state is read with a lock where the
// database takes one. A remote account fails with ErrNotLocal, and a refused one with a
// *DeactivationRefusedError. It returns the event.
func (s *Service) DeactivateTx(ctx context.Context, tx uow.Tx, accountID, eventID string) (account.LifecycleEvent, error) {
	repo := s.d.Repository
	a, err := repo.Account(ctx, tx, accountID)
	if err != nil {
		return account.LifecycleEvent{}, err
	}
	if !a.IsLocal() || a.ActorType == account.ActorApplication {
		return account.LifecycleEvent{}, fmt.Errorf("account %s: %w", accountID, ErrNotLocal)
	}
	now := kernel.Normalize(s.d.Clock.Now())
	reads := deactivationReads{
		state: func(ctx context.Context, id string) (account.LifecycleState, error) {
			return repo.LifecycleState(ctx, tx, id)
		},
		move: func(ctx context.Context, id string) (account.MoveState, error) {
			return repo.FindMoveStateTx(ctx, tx, id)
		},
		reactivation: func(ctx context.Context, id string) (time.Time, error) {
			return repo.FindLatestReactivationTx(ctx, tx, id)
		},
	}
	if err := reads.refusal(ctx, accountID, now); err != nil {
		return account.LifecycleEvent{}, err
	}
	e := account.LifecycleEvent{
		ID: eventID, AccountID: accountID, Kind: account.LifecycleDeactivated, InitiatedBy: account.InitiatedBySelf, OccurredAt: now,
	}
	if err := repo.InsertLifecycleEvent(ctx, tx, e); err != nil {
		return account.LifecycleEvent{}, err
	}
	return e, tx.Emit(ctx, LifecycleEventRecorded{Event: e})
}

// ReactivateTx brings back inside tx a local account that its owner deactivated: a reactivated lifecycle event
// initiated by the account itself. Its statuses, follows, and followers are as they were; its
// sessions and tokens stay ended, and any moderation action stays in effect. An account that is
// not deactivated, which includes one a ban schedules for deletion, fails with ErrNotDeactivated,
// and one whose deadline has passed, also before the deletion job records the deletion, with
// ErrDeactivationExpired. The state is read with a lock where the database takes one, so that the
// deletion job and the reactivation never both win.
func (s *Service) ReactivateTx(ctx context.Context, tx uow.Tx, accountID string) error {
	st, err := s.d.Repository.LifecycleState(ctx, tx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return fmt.Errorf("account %s: %w", accountID, ErrNotDeactivated)
	case err != nil:
		return err
	case !st.Deactivated():
		return fmt.Errorf("account %s: %w", accountID, ErrNotDeactivated)
	}
	now := kernel.Normalize(s.d.Clock.Now())
	if st.DeactivationExpired(now) {
		return fmt.Errorf("account %s: %w", accountID, ErrDeactivationExpired)
	}
	e := account.LifecycleEvent{
		ID: s.d.IDs.NewID(), AccountID: accountID, Kind: account.LifecycleReactivated, InitiatedBy: account.InitiatedBySelf, OccurredAt: now,
	}
	if err := s.d.Repository.InsertLifecycleEvent(ctx, tx, e); err != nil {
		return err
	}
	return tx.Emit(ctx, LifecycleEventRecorded{Event: e})
}

// DeactivatedTx reports, inside tx, whether a local account's owner deactivated it and has not
// come back; an account without lifecycle events is not deactivated.
func (s *Service) DeactivatedTx(ctx context.Context, tx uow.Tx, id string) (bool, error) {
	st, err := s.d.Repository.FindLifecycleStateTx(ctx, tx, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return st.Deactivated(), nil
}
