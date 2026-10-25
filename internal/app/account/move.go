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
	// FindMoveStateTx is FindMoveState inside tx.
	FindMoveStateTx(ctx context.Context, tx uow.Tx, accountID string) (account.MoveState, error)
	// InsertMoveCancellation inserts the cancellation of a move and reports false, writing
	// nothing, when the move is cancelled already.
	InsertMoveCancellation(ctx context.Context, tx uow.Tx, c account.MoveCancellation) (bool, error)
	// DeleteMoveState deletes the row of account_move_states of an account, for this module's
	// projector only.
	DeleteMoveState(ctx context.Context, tx uow.Tx, accountID string) error
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
	// AuditID is the ID of the audit log entry account_moved of a local move of a limited account
	// to a local one (ADR-0089), "" for the other moves; ByUserID is the user who moved and
	// SourceLabel the handle of the account that moved, for that entry.
	AuditID     string
	ByUserID    string
	SourceLabel string
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

// EventMoveCancelled is emitted when a local account cancels its move.
const EventMoveCancelled = "account.move_cancelled"

// MoveCancelled is emitted when a local account cancels its move in force, in the transaction
// that records the cancellation.
type MoveCancelled struct {
	Move         account.Move
	Cancellation account.MoveCancellation
}

// EventType implements uow.Event.
func (MoveCancelled) EventType() string { return EventMoveCancelled }

// ErrNotMoved reports a cancellation of the move of an account that has no move in force.
var ErrNotMoved = errors.New("account: the account has not moved")

// ErrMovedBefore reports a local move to a target that the account moved to before; see
// account.ErrMovedBefore.
var ErrMovedBefore = account.ErrMovedBefore

// LocalMoveInput is the move of a local account that the federation module checked: its target
// names the account among its aliases, has not moved, and is not suspended (ADR-0089).
type LocalMoveInput struct {
	AccountID       string
	TargetAccountID string
	// ByUserID is the user who moves the account.
	ByUserID string
	// SourceLimited reports that moderation limits the account and the target is local, which
	// tells the moderators and writes the audit log entry account_moved (ADR-0089).
	SourceLimited bool
}

// RecordLocalMove records the move of local account in.AccountID to in.TargetAccountID
// (initiated_by self) and emits Moved, which points the account's move state at the target, so
// that its actor carries movedTo from the commit on; the federation module sends the Move and
// the Update of the actor, and the graph module moves the local followers. It fails with
// account.ErrMoveTooSoon within account.LocalMoveInterval after the account's newest move,
// cancelled or not, with ErrMovedBefore for a target that the account moved to before, and
// with ErrNotLocal for an account that is not a local user's.
func (s *Service) RecordLocalMove(ctx context.Context, in LocalMoveInput) (account.Move, error) {
	m := account.Move{
		ID: s.d.IDs.NewID(), AccountID: in.AccountID, TargetAccountID: in.TargetAccountID,
		InitiatedBy: account.MoveInitiatedBySelf, OccurredAt: kernel.Normalize(s.d.Clock.Now()),
	}
	if err := m.Validate(); err != nil {
		return account.Move{}, err
	}
	auditID := ""
	if in.SourceLimited {
		auditID = s.d.IDs.NewID()
	}
	repo := s.d.Repository
	err := s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		a, err := repo.Account(ctx, tx, m.AccountID)
		if err != nil {
			return err
		}
		if !a.IsLocal() || a.ActorType == account.ActorApplication {
			return fmt.Errorf("account %s: %w", m.AccountID, ErrNotLocal)
		}
		local, err := repo.FindLocalAccountTx(ctx, tx, m.AccountID)
		if err != nil {
			return err
		}
		var latest *account.Move
		switch l, err := repo.LatestMove(ctx, tx, m.AccountID); {
		case err == nil:
			latest = &l
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if err := m.FollowsLocal(latest); err != nil {
			return err
		}
		switch inserted, err := repo.InsertMove(ctx, tx, m); {
		case err != nil:
			return err
		case !inserted:
			return ErrMovedBefore
		}
		return tx.Emit(ctx, Moved{
			Move: m, SourceLimited: in.SourceLimited, AuditID: auditID, ByUserID: in.ByUserID, SourceLabel: "@" + local.Username,
		})
	})
	if err != nil {
		return account.Move{}, err
	}
	return m, nil
}

// CancelMove cancels the move in force of local account accountID (ADR-0089): it records the
// cancellation and emits MoveCancelled, which deletes the account's move state, so that the
// account can post again and its actor no longer carries movedTo; the federation module sends
// the Update of the actor. The followers that moved stay with the target, and the wait before
// the next move still counts from the cancelled move. It fails with ErrNotMoved when the
// account has no move in force.
func (s *Service) CancelMove(ctx context.Context, accountID string) error {
	now := kernel.Normalize(s.d.Clock.Now())
	repo := s.d.Repository
	return s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		st, err := repo.FindMoveStateTx(ctx, tx, accountID)
		switch {
		case errors.Is(err, ErrNotFound):
			return ErrNotMoved
		case err != nil:
			return err
		}
		a, err := repo.Account(ctx, tx, accountID)
		if err != nil {
			return err
		}
		if !a.IsLocal() {
			return fmt.Errorf("account %s: %w", accountID, ErrNotLocal)
		}
		m, err := repo.FindMove(ctx, st.MoveID)
		if err != nil {
			return err
		}
		c := account.MoveCancellation{MoveID: st.MoveID, CancelledAt: now}
		switch ok, err := repo.InsertMoveCancellation(ctx, tx, c); {
		case err != nil:
			return err
		case !ok:
			return ErrNotMoved
		}
		return tx.Emit(ctx, MoveCancelled{Move: m, Cancellation: c})
	})
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
// move in force, and a cancelled one leaves the account without one.
func registerMoveProjector(r *uow.Projectors, repo Repository) {
	r.Register(EventAccountMoved, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.UpsertMoveState(ctx, tx, account.StateOf(e.(Moved).Move))
	})
	r.Register(EventMoveCancelled, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.DeleteMoveState(ctx, tx, e.(MoveCancelled).Move.AccountID)
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
