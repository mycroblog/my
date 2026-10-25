// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"context"
	"errors"
	"fmt"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
)

// The moves of accounts (ADR-0089, FR-414): the event that an account moved to another, and the
// projection of the move in force, an axis of its own beside the lifecycle (ADR-0020 rule 2).
// The federation module records the Moves of remote accounts that pass its checks; the graph
// module moves the local followers on account.moved.

// MoveRepository is the part of Repository about moves (data-model §4.2). Lookups that find
// nothing return an error wrapping ErrNotFound.
type MoveRepository interface {
	// InsertMove inserts a move and reports false, writing nothing, when the account already
	// moved to the target.
	InsertMove(ctx context.Context, tx uow.Tx, m account.Move) (bool, error)
	// LatestMove returns the newest move of an account inside tx.
	LatestMove(ctx context.Context, tx uow.Tx, accountID string) (account.Move, error)
	// FindMove returns a move by ID.
	FindMove(ctx context.Context, id string) (account.Move, error)
	// UpsertMoveState and DeleteAllMoveStates write account_move_states, for this module's
	// projector and rebuild only.
	UpsertMoveState(ctx context.Context, tx uow.Tx, s account.MoveState) error
	DeleteAllMoveStates(ctx context.Context, tx uow.Tx) error
	// FindMoveState returns the move in force of an account; ErrNotFound means it has not moved.
	FindMoveState(ctx context.Context, accountID string) (account.MoveState, error)
	// ListMoveRecords returns every move with whether it is cancelled, ordered by account,
	// time, and ID, for rebuilding account_move_states.
	ListMoveRecords(ctx context.Context, tx uow.Tx) ([]account.MoveRecord, error)
	// ListMoveStates returns every row of account_move_states, ordered by account ID.
	ListMoveStates(ctx context.Context) ([]account.MoveState, error)
}

// EventAccountMoved is emitted when a move is recorded.
const EventAccountMoved = "account.moved"

// Moved is emitted when an account moves to another, in the transaction that records the
// move.
type Moved struct {
	Move account.Move
	// SourceLimited reports that moderation limits the account that moved. The target does not
	// inherit the limit; the moderators are told (ADR-0089).
	SourceLimited bool
}

// EventType implements uow.Event.
func (Moved) EventType() string { return EventAccountMoved }

// RemoteMoveInput is a Move of a remote account that passed the federation module's checks.
type RemoteMoveInput struct {
	AccountID       string
	TargetAccountID string
	// ActivityURI is the IRI of the Move activity.
	ActivityURI string
	// SourceLimited is Moved.SourceLimited.
	SourceLimited bool
}

// RecordRemoteMove records the move of remote account in.AccountID to in.TargetAccountID
// (initiated_by remote) and emits Moved, which points the account's move state at the
// target. A move to the target that the account already moved to records nothing and reports
// false, so receiving the same Move again changes nothing. A move to another target within
// account.RemoteMoveInterval after the account's newest move fails with account.ErrMoveTooSoon.
// The account must be remote (ErrNotRemote).
func (s *Service) RecordRemoteMove(ctx context.Context, in RemoteMoveInput) (account.Move, bool, error) {
	m := account.Move{
		ID: s.d.IDs.NewID(), AccountID: in.AccountID, TargetAccountID: in.TargetAccountID,
		InitiatedBy: account.MoveInitiatedByRemote, ActivityURI: in.ActivityURI, OccurredAt: kernel.Normalize(s.d.Clock.Now()),
	}
	if err := m.Validate(); err != nil {
		return account.Move{}, false, err
	}
	repo := s.d.Repository
	var recorded bool
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		recorded = false
		a, err := repo.Account(ctx, tx, m.AccountID)
		if err != nil {
			return err
		}
		if a.IsLocal() {
			return fmt.Errorf("account %s: %w", m.AccountID, ErrNotRemote)
		}
		var latest *account.Move
		switch l, err := repo.LatestMove(ctx, tx, m.AccountID); {
		case err == nil:
			latest = &l
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if ok, err := m.FollowsRemote(latest); err != nil || !ok {
			return err
		}
		if recorded, err = repo.InsertMove(ctx, tx, m); err != nil || !recorded {
			return err
		}
		return tx.Emit(ctx, Moved{Move: m, SourceLimited: in.SourceLimited})
	})
	if err != nil {
		return account.Move{}, false, err
	}
	return m, recorded, nil
}

// MoveState returns the move in force of an account: whom it moved to, for showing the account
// with its target (ADR-0089). It fails with ErrNotFound when the account has not moved.
func (s *Service) MoveState(ctx context.Context, accountID string) (account.MoveState, error) {
	return s.d.Repository.FindMoveState(ctx, accountID)
}

// FindMove returns a move by ID; ErrNotFound means there is none.
func (s *Service) FindMove(ctx context.Context, id string) (account.Move, error) {
	return s.d.Repository.FindMove(ctx, id)
}

// registerMoveProjector makes account_move_states follow the moves: a new move is the account's
// move in force.
func registerMoveProjector(r *uow.Projectors, repo Repository) {
	r.Register(EventAccountMoved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertMoveState(ctx, tx, account.StateOf(e.(Moved).Move))
	})
}

func rebuildMoveStates(ctx context.Context, tx uow.Tx, repo Repository) error {
	if err := repo.DeleteAllMoveStates(ctx, tx); err != nil {
		return err
	}
	records, err := repo.ListMoveRecords(ctx, tx)
	if err != nil {
		return err
	}
	for _, st := range account.MoveStates(records) {
		if err := repo.UpsertMoveState(ctx, tx, st); err != nil {
			return err
		}
	}
	return nil
}
