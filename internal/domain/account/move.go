// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"cmp"
	"errors"
	"slices"
	"time"
)

// MoveInitiator says who moved an account (account_moves.initiated_by).
type MoveInitiator string

// Move initiators.
const (
	// MoveInitiatedBySelf is a local account's own move.
	MoveInitiatedBySelf MoveInitiator = "self"
	// MoveInitiatedByRemote is a Move that a remote account's server sent.
	MoveInitiatedByRemote MoveInitiator = "remote"
)

// RemoteMoveInterval is how long after the move of a remote account a Move of it to another
// target is dropped (ADR-0089; Mastodon waits 7 days per account, GoToSocial a week).
const RemoteMoveInterval = 7 * 24 * time.Hour

var (
	// ErrSelfMove reports a move whose account and target are the same.
	ErrSelfMove = errors.New("account: an account cannot move to itself")
	// ErrMoveTooSoon reports a move of an account to another target within the interval after
	// its newest move.
	ErrMoveTooSoon = errors.New("account: the account moved too recently")
)

// Move is the event that an account moved to another (account_moves, ADR-0089). An account
// moves to one target once.
type Move struct {
	ID              string
	AccountID       string
	TargetAccountID string
	InitiatedBy     MoveInitiator
	// ActivityURI is the Move received, for a remote move; "" for a local one.
	ActivityURI string
	OccurredAt  time.Time
}

// Validate checks the invariants of a new move.
func (m Move) Validate() error {
	if m.ID == "" || m.AccountID == "" || m.TargetAccountID == "" {
		return errors.New("account: a move needs an ID, an account, and a target")
	}
	if m.AccountID == m.TargetAccountID {
		return ErrSelfMove
	}
	if m.InitiatedBy != MoveInitiatedBySelf && m.InitiatedBy != MoveInitiatedByRemote {
		return errors.New("account: unknown move initiator")
	}
	return nil
}

// FollowsRemote reports whether a remote move m may follow the account's newest move latest
// (nil for none): to the same target it changes nothing (false, nil), and to another target it
// must come RemoteMoveInterval after latest (ErrMoveTooSoon).
func (m Move) FollowsRemote(latest *Move) (bool, error) {
	switch {
	case latest == nil:
		return true, nil
	case latest.TargetAccountID == m.TargetAccountID:
		return false, nil
	case m.OccurredAt.Sub(latest.OccurredAt) < RemoteMoveInterval:
		return false, ErrMoveTooSoon
	}
	return true, nil
}

// MoveCancellation is the event that a local account cancelled its move
// (account_move_cancellations).
type MoveCancellation struct {
	MoveID      string
	CancelledAt time.Time
}

// MoveState is the move in force of an account (the account_move_states projection): its
// newest move, unless that is cancelled. An account without one has not moved.
type MoveState struct {
	AccountID       string
	MoveID          string
	TargetAccountID string
	Since           time.Time
}

// StateOf returns the state that move m puts its account in.
func StateOf(m Move) MoveState {
	return MoveState{AccountID: m.AccountID, MoveID: m.ID, TargetAccountID: m.TargetAccountID, Since: m.OccurredAt}
}

// MoveRecord is a move with whether it is cancelled: the source of account_move_states.
type MoveRecord struct {
	Move      Move
	Cancelled bool
}

// MoveStates derives the move in force of every account from its moves: the newest one by
// OccurredAt and then ID, when it is not cancelled. Updating account_move_states move by move
// and rebuilding it with this function agree. The states are ordered by account ID.
func MoveStates(records []MoveRecord) []MoveState {
	newest := map[string]MoveRecord{}
	var order []string
	for _, r := range records {
		cur, seen := newest[r.Move.AccountID]
		if !seen {
			order = append(order, r.Move.AccountID)
		}
		if !seen || cmp.Or(r.Move.OccurredAt.Compare(cur.Move.OccurredAt), cmp.Compare(r.Move.ID, cur.Move.ID)) > 0 {
			newest[r.Move.AccountID] = r
		}
	}
	out := make([]MoveState, 0, len(order))
	for _, id := range order {
		if r := newest[id]; !r.Cancelled {
			out = append(out, StateOf(r.Move))
		}
	}
	slices.SortFunc(out, func(a, b MoveState) int { return cmp.Compare(a.AccountID, b.AccountID) })
	return out
}
