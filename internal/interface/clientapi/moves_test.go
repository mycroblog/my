// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/domain/notification"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// fakeMoves holds moves by ID and the moves in force by account.
type fakeMoves struct {
	moves  map[string]account.Move
	states map[string]account.MoveState
}

func (f fakeMoves) MoveState(_ context.Context, accountID string) (appaccount.MoveState, error) {
	st, ok := f.states[accountID]
	if !ok {
		return appaccount.MoveState{}, appaccount.ErrNotFound
	}
	return st, nil
}

func (f fakeMoves) FindMove(_ context.Context, id string) (appaccount.Move, error) {
	m, ok := f.moves[id]
	if !ok {
		return appaccount.Move{}, appaccount.ErrNotFound
	}
	return m, nil
}

// TestGetAccountShowsItsMove checks that an account that moved shows its target as moved.
func TestGetAccountShowsItsMove(t *testing.T) {
	moveID := testkit.NamedID("move")
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Moves = fakeMoves{states: map[string]account.MoveState{
			aliceAccountID: {AccountID: aliceAccountID, MoveID: moveID, TargetAccountID: bobAccountID, Since: time.Now()},
		}}
		o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
	})
	res := e.get(t, "/accounts/"+aliceAccountID, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var got clientapi.Account
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Moved == nil || got.Moved.Moved != nil || got.Moved.FollowersCount != nil {
		t.Errorf("moved = %+v, want the target without its own move or counts", got.Moved)
	}
}

// TestListMoveNotifications checks the account_moved and admin_account_moved notifications:
// the moved account as their account (the source of an account_moved, the account of the move
// of an admin_account_moved), the target and the outcome in account_move, and neither when the
// move is gone.
func TestListMoveNotifications(t *testing.T) {
	at := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	moveID := testkit.NamedID("move")
	ns := []notification.Notification{
		{
			ID: notificationID(1), RecipientAccountID: aliceAccountID, Type: notification.TypeAccountMoved, SourceAccountID: bobAccountID,
			AccountMoveID: moveID, AccountMoveOutcome: notification.MoveBlocking, CreatedAt: at,
		},
		{ID: notificationID(2), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminAccountMoved, AccountMoveID: moveID, CreatedAt: at},
		{ID: notificationID(3), RecipientAccountID: aliceAccountID, Type: notification.TypeAdminAccountMoved, AccountMoveID: testkit.NamedID("gone"), CreatedAt: at},
	}
	f := &fakeNotifications{dismissed: map[string]bool{}, markers: map[string]string{}, ns: ns}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.Notifications = f
		o.Moves = fakeMoves{moves: map[string]account.Move{
			moveID: {ID: moveID, AccountID: bobAccountID, TargetAccountID: aliceAccountID, InitiatedBy: account.MoveInitiatedByRemote, OccurredAt: at},
		}}
		o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
	})
	res := e.send(t, http.MethodGet, "/notifications", "read", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	page := decode[clientapi.NotificationPage](t, res)
	if len(page.Items) != 3 {
		t.Fatalf("items = %+v, want 3", page.Items)
	}
	gone, admin, moved := page.Items[0], page.Items[1], page.Items[2]
	if m := moved.AccountMove; moved.Account == nil || m == nil || m.Target == nil || m.Outcome == nil || *m.Outcome != clientapi.AccountMoveOutcomeBlocking {
		t.Errorf("account_moved = %+v (%+v), want the moved account, the target, and blocking", moved, moved.AccountMove)
	}
	if m := admin.AccountMove; admin.Account == nil || m == nil || m.Target == nil || m.Outcome != nil {
		t.Errorf("admin_account_moved = %+v (%+v), want the moved account and the target, without an outcome", admin, admin.AccountMove)
	}
	if gone.AccountMove != nil || gone.Account != nil {
		t.Errorf("a notification of a move that is gone = %+v, want neither the move nor an account", gone)
	}
}
