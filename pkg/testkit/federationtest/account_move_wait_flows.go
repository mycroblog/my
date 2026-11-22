// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"errors"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/federation"
)

// movesSayWhenTheyCanMoveAgain checks the wait after a local move (ADR-0089 §8), part of
// AccountMoveFlows, on the clock of the environment: MoveWait has neither time for an account that
// never moved, and the time of the newest move with LocalMoveInterval after it while that is ahead,
// also after the move is canceled, then the time of the move alone; a move within the wait, to
// another target or to the same one, fails with a *MoveTooSoonError carrying that CanMoveAt (too
// soon is told before moved before), and a move to the same target after it fails with
// ErrMovedBefore alone.
func movesSayWhenTheyCanMoveAgain(t *testing.T, e *moveEnv) {
	ctx := t.Context()
	reg := e.Register(t, "alice")
	alice := reg.AccountID
	aliceURI := federation.IRI(e.actorURI(alice))
	const second = "https://second.example/users/alice"
	e.serve(t, moveTarget, "", aliceURI)
	e.serve(t, second, "", aliceURI)
	target, secondID := e.Remote(t, moveTarget), e.Remote(t, second)
	moves := e.localMoves(nil)
	move := func(targetID string) error {
		_, err := moves.Move(ctx, appfederation.LocalMoveRequest{AccountID: alice, UserID: reg.User.ID, TargetAccountID: targetID})
		return err
	}
	wait := func(what string, last, next time.Time) {
		t.Helper()
		got, err := e.Accounts.MoveWait(ctx, alice)
		if err != nil {
			t.Fatalf("%s: MoveWait: %v", what, err)
		}
		if !got.LastMovedAt.Equal(last) || !got.CanMoveAt.Equal(next) {
			t.Errorf("%s: MoveWait = %+v, want last moved at %v and can move at %v", what, got, last, next)
		}
	}
	tooSoon := func(what string, err error, canMoveAt time.Time) {
		t.Helper()
		var soon *appaccount.MoveTooSoonError
		if !errors.As(err, &soon) || !errors.Is(err, appaccount.ErrMoveTooSoon) || errors.Is(err, appaccount.ErrMovedBefore) {
			t.Errorf("%s = %v, want a *MoveTooSoonError and not ErrMovedBefore", what, err)
			return
		}
		if !soon.CanMoveAt.Equal(canMoveAt) {
			t.Errorf("%s: CanMoveAt = %v, want %v", what, soon.CanMoveAt, canMoveAt)
		}
	}

	wait("never moved", time.Time{}, time.Time{})

	movedAt := e.Clock.Now()
	if err := move(target); err != nil {
		t.Fatalf("Move: %v", err)
	}
	canMoveAt := movedAt.Add(account.LocalMoveInterval)
	wait("after the move", movedAt, canMoveAt)
	tooSoon("Move to another target at once", move(secondID), canMoveAt)
	wait("after a refused move", movedAt, canMoveAt)

	if err := moves.Cancel(ctx, alice); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	wait("after the cancellation", movedAt, canMoveAt)
	tooSoon("Move to the same target after the cancellation", move(target), canMoveAt)
	tooSoon("Move to another target after the cancellation", move(secondID), canMoveAt)

	e.Clock.Advance(account.LocalMoveInterval - time.Second)
	wait("a second before the wait ends", movedAt, canMoveAt)
	tooSoon("Move a second before the wait ends", move(secondID), canMoveAt)

	e.Clock.Advance(time.Second)
	wait("when the wait ends", movedAt, time.Time{})
	if err := move(target); !errors.Is(err, appaccount.ErrMovedBefore) || errors.Is(err, appaccount.ErrMoveTooSoon) {
		t.Errorf("Move to the first target after the wait = %v, want ErrMovedBefore alone", err)
	}
	wait("after a repeated move", movedAt, time.Time{})

	e.Clock.Advance(time.Hour)
	secondAt := e.Clock.Now()
	if err := move(secondID); err != nil {
		t.Fatalf("Move after the wait: %v", err)
	}
	wait("after the second move", secondAt, secondAt.Add(account.LocalMoveInterval))
}
