// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"context"
	"errors"
	"fmt"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The move of a local account to another (FR-009, FR-414, ADR-0089 送る側). Before the account
// module records it, the target is checked as a received Move checks its target: it is fetched
// again without a cache and must name the account among its aliases, it must not be the account
// or have moved itself, and neither it nor its domain may be suspended here. The target may be a
// local account, whose aliases are read here instead. Recording the move makes the actor carry
// movedTo, queues the Update of the actor and the Move to the remote followers
// (registerMoveProjectors), and makes the graph module move the local followers.

// The reasons that a local move is refused.
var (
	// ErrMoveSourceUnavailable reports a move of an account that is not an active local user's,
	// or that moderation suspends.
	ErrMoveSourceUnavailable = errors.New("federation: the account cannot move")
	// ErrMoveTargetUnavailable reports a target that is not found, could not be fetched, is the
	// account itself, is not active, or is suspended here, it or its domain.
	ErrMoveTargetUnavailable = errors.New("federation: the target of the move is unavailable")
	// ErrMoveTargetMoved reports a target that moved itself.
	ErrMoveTargetMoved = errors.New("federation: the target of the move moved itself")
	// ErrMoveAliasMissing reports a target whose aliases (alsoKnownAs) do not name the account.
	ErrMoveAliasMissing = errors.New("federation: the target of the move does not name the account as an alias")
)

// LocalMoveAccounts are the reads and writes of the account module that a local move needs;
// *appaccount.Service implements it.
type LocalMoveAccounts interface {
	FindByID(ctx context.Context, id string) (appaccount.Actor, error)
	MoveState(ctx context.Context, accountID string) (appaccount.MoveState, error)
	RecordLocalMove(ctx context.Context, in appaccount.LocalMoveInput) (appaccount.Move, error)
	CancelMove(ctx context.Context, accountID string) error
}

// LocalMovesDeps are the ports of LocalMoves.
type LocalMovesDeps struct {
	Accounts LocalMoveAccounts
	// Targets fetches a remote target again from its origin.
	Targets MoveTargets
	// Aliases are the aliases of the local accounts; nil means that none has one, so no account
	// moves to a local one.
	Aliases LocalAliases
	// Domains and Suspensions are the suspensions of domains and accounts; nil suspends none.
	Domains     DomainPolicy
	Suspensions AccountPolicy
	// Limits tells the accounts that moderation limits; nil limits none.
	Limits MoveLimits
}

// LocalMoves moves local accounts to other accounts and cancels their moves.
type LocalMoves struct{ d LocalMovesDeps }

// NewLocalMoves returns the local moves on d.
func NewLocalMoves(d LocalMovesDeps) *LocalMoves {
	if d.Domains == nil {
		d.Domains = NoDomainRestrictions{}
	}
	if d.Suspensions == nil {
		d.Suspensions = NoAccountRestrictions{}
	}
	return &LocalMoves{d: d}
}

// LocalMoveRequest is the move of the local account AccountID, which the user UserID logs in
// as and confirmed, to the account TargetAccountID.
type LocalMoveRequest struct {
	AccountID       string
	UserID          string
	TargetAccountID string
}

// Move checks the target of in and records the move. It fails with ErrMoveSourceUnavailable,
// ErrMoveTargetUnavailable, ErrMoveTargetMoved, or ErrMoveAliasMissing for a move that the
// checks refuse, and with the errors of appaccount.Service.RecordLocalMove
// (appaccount.ErrMoveTooSoon within 30 days of the account's last move, appaccount.ErrMovedBefore
// for a target it moved to before). A moderation limit of the account is not carried to a local
// target; the moderators are told instead (ADR-0089).
func (m *LocalMoves) Move(ctx context.Context, in LocalMoveRequest) (appaccount.Move, error) {
	source, err := m.d.Accounts.FindByID(ctx, in.AccountID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Move{}, ErrMoveSourceUnavailable
	case err != nil:
		return appaccount.Move{}, err
	case !source.Account.IsLocal() || source.Local == nil || !source.Lifecycle.Active():
		return appaccount.Move{}, ErrMoveSourceUnavailable
	}
	switch suspended, err := m.d.Suspensions.AccountFederationSuspended(ctx, source.Account.ID); {
	case err != nil:
		return appaccount.Move{}, err
	case suspended:
		return appaccount.Move{}, ErrMoveSourceUnavailable
	}
	if in.TargetAccountID == source.Account.ID {
		return appaccount.Move{}, fmt.Errorf("%w: the account itself", ErrMoveTargetUnavailable)
	}
	target, err := m.target(ctx, source, in.TargetAccountID)
	if err != nil {
		return appaccount.Move{}, err
	}
	limited := false
	if m.d.Limits != nil && target.Account.IsLocal() {
		if limited, err = m.d.Limits.AccountLimited(ctx, source.Account.ID); err != nil {
			return appaccount.Move{}, err
		}
	}
	return m.d.Accounts.RecordLocalMove(ctx, appaccount.LocalMoveInput{
		AccountID: source.Account.ID, TargetAccountID: target.Account.ID, ByUserID: in.UserID, SourceLimited: limited,
	})
}

// target returns the checked target of the move of source.
func (m *LocalMoves) target(ctx context.Context, source appaccount.Actor, targetID string) (appaccount.Actor, error) {
	t, err := m.d.Accounts.FindByID(ctx, targetID)
	switch {
	case errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Actor{}, fmt.Errorf("%w: account %s", ErrMoveTargetUnavailable, targetID)
	case err != nil:
		return appaccount.Actor{}, err
	case t.IsInstanceActor():
		return appaccount.Actor{}, fmt.Errorf("%w: the instance actor", ErrMoveTargetUnavailable)
	}
	sourceURI := federation.IRI(source.Account.URI)
	if t.Account.IsLocal() {
		if t.Local == nil || !t.Lifecycle.Active() {
			return appaccount.Actor{}, fmt.Errorf("%w: %s is not active", ErrMoveTargetUnavailable, t.Account.URI)
		}
		aliased := false
		if m.d.Aliases != nil {
			if aliased, err = m.d.Aliases.HasAlias(ctx, t.Account.ID, string(sourceURI)); err != nil {
				return appaccount.Actor{}, err
			}
		}
		if !aliased {
			return appaccount.Actor{}, ErrMoveAliasMissing
		}
	} else {
		if m.d.Targets == nil {
			return appaccount.Actor{}, fmt.Errorf("%w: remote targets are not fetched", ErrMoveTargetUnavailable)
		}
		stored, doc, err := m.d.Targets.RefreshActor(ctx, t.Account.URI)
		switch {
		case err != nil:
			return appaccount.Actor{}, fmt.Errorf("%w: fetch %s: %w", ErrMoveTargetUnavailable, t.Account.URI, err)
		case doc == nil || doc.ID != federation.IRI(t.Account.URI) || !stored.Lifecycle.Active():
			return appaccount.Actor{}, fmt.Errorf("%w: %s is not active", ErrMoveTargetUnavailable, t.Account.URI)
		case !slices.Contains(doc.AlsoKnownAs, sourceURI):
			return appaccount.Actor{}, ErrMoveAliasMissing
		case doc.MovedTo != "":
			return appaccount.Actor{}, ErrMoveTargetMoved
		}
		switch suspended, err := m.d.Domains.FederationSuspended(ctx, t.Account.Domain); {
		case err != nil:
			return appaccount.Actor{}, err
		case suspended:
			return appaccount.Actor{}, fmt.Errorf("%w: the domain %s is suspended", ErrMoveTargetUnavailable, t.Account.Domain)
		}
		t = stored
	}
	switch suspended, err := m.d.Suspensions.AccountFederationSuspended(ctx, t.Account.ID); {
	case err != nil:
		return appaccount.Actor{}, err
	case suspended:
		return appaccount.Actor{}, fmt.Errorf("%w: %s is suspended", ErrMoveTargetUnavailable, t.Account.URI)
	}
	switch _, err := m.d.Accounts.MoveState(ctx, t.Account.ID); {
	case err == nil:
		return appaccount.Actor{}, ErrMoveTargetMoved
	case !errors.Is(err, appaccount.ErrNotFound):
		return appaccount.Actor{}, err
	}
	return t, nil
}

// Cancel cancels the move in force of local account accountID (appaccount.Service.CancelMove),
// which sends the Update of the actor without movedTo. It fails with appaccount.ErrNotMoved when
// the account has not moved.
func (m *LocalMoves) Cancel(ctx context.Context, accountID string) error {
	return m.d.Accounts.CancelMove(ctx, accountID)
}
