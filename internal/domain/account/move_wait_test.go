// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"errors"
	"testing"
	"time"
)

func TestNextLocalMoveAt(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC)
	m := Move{ID: "m1", AccountID: "a", TargetAccountID: "b", InitiatedBy: MoveInitiatedBySelf, OccurredAt: at}
	if got, want := m.NextLocalMoveAt(), at.Add(LocalMoveInterval); !got.Equal(want) {
		t.Errorf("NextLocalMoveAt = %v, want %v", got, want)
	}
	if want := 30 * 24 * time.Hour; LocalMoveInterval != want {
		t.Errorf("LocalMoveInterval = %v, want %v", LocalMoveInterval, want)
	}
}

func TestMoveTooSoonErrorIsErrMoveTooSoon(t *testing.T) {
	at := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	var err error = &MoveTooSoonError{CanMoveAt: at}
	if !errors.Is(err, ErrMoveTooSoon) {
		t.Errorf("errors.Is(%v, ErrMoveTooSoon) = false", err)
	}
	if errors.Is(err, ErrMovedBefore) {
		t.Errorf("errors.Is(%v, ErrMovedBefore) = true", err)
	}
	if err.Error() == "" {
		t.Error("Error is empty")
	}
}

// TestFollowsLocalSaysWhenItCanMoveAgain checks that a local move within LocalMoveInterval of
// the newest move fails with a *MoveTooSoonError whose CanMoveAt is the NextLocalMoveAt of that
// move, also when it goes to the same target, so that too soon is told before moved before; and
// that from the end of the interval on, a move to the same target is ErrMovedBefore alone.
func TestFollowsLocalSaysWhenItCanMoveAgain(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	latest := &Move{ID: "m1", AccountID: "a", TargetAccountID: "b", InitiatedBy: MoveInitiatedBySelf, OccurredAt: at}
	for _, tc := range []struct {
		name string
		next Move
	}{
		{"another target at once", Move{TargetAccountID: "c", OccurredAt: at}},
		{"another target a second before the interval ends", Move{TargetAccountID: "c", OccurredAt: at.Add(LocalMoveInterval - time.Second)}},
		{"the same target too soon", Move{TargetAccountID: "b", OccurredAt: at.Add(time.Hour)}},
	} {
		err := tc.next.FollowsLocal(latest)
		var soon *MoveTooSoonError
		if !errors.As(err, &soon) {
			t.Errorf("%s: FollowsLocal = %v, want a *MoveTooSoonError", tc.name, err)
			continue
		}
		if !soon.CanMoveAt.Equal(latest.NextLocalMoveAt()) {
			t.Errorf("%s: CanMoveAt = %v, want %v", tc.name, soon.CanMoveAt, latest.NextLocalMoveAt())
		}
		if !errors.Is(err, ErrMoveTooSoon) || errors.Is(err, ErrMovedBefore) {
			t.Errorf("%s: FollowsLocal = %v, want ErrMoveTooSoon and not ErrMovedBefore", tc.name, err)
		}
	}

	err := Move{TargetAccountID: "b", OccurredAt: at.Add(LocalMoveInterval)}.FollowsLocal(latest)
	if !errors.Is(err, ErrMovedBefore) || errors.Is(err, ErrMoveTooSoon) {
		t.Errorf("the same target after the interval: FollowsLocal = %v, want ErrMovedBefore alone", err)
	}
}
