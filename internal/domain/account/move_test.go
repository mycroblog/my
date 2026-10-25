// SPDX-License-Identifier: AGPL-3.0-only

package account

import (
	"errors"
	"testing"
	"time"
)

func TestMoveValidate(t *testing.T) {
	ok := Move{ID: "m", AccountID: "a", TargetAccountID: "b", InitiatedBy: MoveInitiatedByRemote}
	if err := ok.Validate(); err != nil {
		t.Errorf("Validate = %v", err)
	}
	self := ok
	self.TargetAccountID = "a"
	if err := self.Validate(); !errors.Is(err, ErrSelfMove) {
		t.Errorf("Validate of a move to itself = %v, want ErrSelfMove", err)
	}
	unknown := ok
	unknown.InitiatedBy = "moderator"
	if err := unknown.Validate(); err == nil {
		t.Error("Validate of an unknown initiator = nil")
	}
}

func TestFollowsRemote(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	latest := &Move{ID: "m1", AccountID: "a", TargetAccountID: "b", OccurredAt: at}
	for _, tc := range []struct {
		name   string
		latest *Move
		next   Move
		want   bool
		err    error
	}{
		{"first", nil, Move{TargetAccountID: "b", OccurredAt: at}, true, nil},
		{"same target", latest, Move{TargetAccountID: "b", OccurredAt: at.Add(30 * 24 * time.Hour)}, false, nil},
		{"another target too soon", latest, Move{TargetAccountID: "c", OccurredAt: at.Add(RemoteMoveInterval - time.Second)}, false, ErrMoveTooSoon},
		{"another target after the interval", latest, Move{TargetAccountID: "c", OccurredAt: at.Add(RemoteMoveInterval)}, true, nil},
	} {
		got, err := tc.next.FollowsRemote(tc.latest)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("%s: FollowsRemote = %t, %v, want %t, %v", tc.name, got, err, tc.want, tc.err)
		}
	}
}

func TestMoveStatesTakeTheNewestMoveUnlessCancelled(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	records := []MoveRecord{
		{Move: Move{ID: "m2", AccountID: "b", TargetAccountID: "x", OccurredAt: at}},
		{Move: Move{ID: "m1", AccountID: "a", TargetAccountID: "x", OccurredAt: at}},
		{Move: Move{ID: "m3", AccountID: "a", TargetAccountID: "y", OccurredAt: at.Add(time.Hour)}},
		{Move: Move{ID: "m4", AccountID: "c", TargetAccountID: "x", OccurredAt: at}, Cancelled: true},
	}
	got := MoveStates(records)
	want := []MoveState{
		{AccountID: "a", MoveID: "m3", TargetAccountID: "y", Since: at.Add(time.Hour)},
		{AccountID: "b", MoveID: "m2", TargetAccountID: "x", Since: at},
	}
	if len(got) != len(want) {
		t.Fatalf("MoveStates = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("MoveStates[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestFollowsLocal(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	latest := &Move{ID: "m1", AccountID: "a", TargetAccountID: "b", OccurredAt: at}
	for _, tc := range []struct {
		name   string
		latest *Move
		next   Move
		err    error
	}{
		{"first", nil, Move{TargetAccountID: "b", OccurredAt: at}, nil},
		{"too soon", latest, Move{TargetAccountID: "c", OccurredAt: at.Add(LocalMoveInterval - time.Second)}, ErrMoveTooSoon},
		{"same target too soon", latest, Move{TargetAccountID: "b", OccurredAt: at.Add(time.Hour)}, ErrMoveTooSoon},
		{"same target after the interval", latest, Move{TargetAccountID: "b", OccurredAt: at.Add(LocalMoveInterval)}, ErrMovedBefore},
		{"another target after the interval", latest, Move{TargetAccountID: "c", OccurredAt: at.Add(LocalMoveInterval)}, nil},
	} {
		if err := tc.next.FollowsLocal(tc.latest); !errors.Is(err, tc.err) {
			t.Errorf("%s: FollowsLocal = %v, want %v", tc.name, err, tc.err)
		}
	}
}
