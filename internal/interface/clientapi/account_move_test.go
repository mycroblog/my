// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	appidentity "github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

func (f *fakeOwnAccount) ConfirmMove(_ context.Context, _ string, c appidentity.Confirmation) error {
	return f.confirm(c)
}

// fakeLocalMoves records the moves and refuses the targets in refuse with their errors; a move
// in force is in states, which the client API reads through Options.Moves.
type fakeLocalMoves struct {
	refuse map[string]error
	moved  []appfederation.LocalMoveRequest
	states map[string]account.MoveState
}

func (f *fakeLocalMoves) Move(_ context.Context, in appfederation.LocalMoveRequest) (appaccount.Move, error) {
	if err := f.refuse[in.TargetAccountID]; err != nil {
		return appaccount.Move{}, err
	}
	f.moved = append(f.moved, in)
	f.states[in.AccountID] = account.MoveState{AccountID: in.AccountID, MoveID: testkit.NamedID("move"), TargetAccountID: in.TargetAccountID, Since: time.Now()}
	return appaccount.Move{ID: testkit.NamedID("move")}, nil
}

func (f *fakeLocalMoves) Cancel(_ context.Context, accountID string) error {
	if _, ok := f.states[accountID]; !ok {
		return appaccount.ErrNotMoved
	}
	delete(f.states, accountID)
	return nil
}

// TestMoveOwnAccount checks POST and DELETE /accounts/move (ADR-0089): the
// confirmation comes first, so a wrong password moves nothing; the refused targets have their
// problems; a moved account cannot post, follow, boost, favourite, react, or vote (403
// account-moved) until it cancels the move, which an account that has not moved cannot.
func TestMoveOwnAccount(t *testing.T) {
	own := &fakeOwnAccount{}
	const (
		noAlias = "01890a5d-ac96-774b-bcce-b302099a9001"
		tooSoon = "01890a5d-ac96-774b-bcce-b302099a9002"
		moved   = "01890a5d-ac96-774b-bcce-b302099a9003"
		gone    = "01890a5d-ac96-774b-bcce-b302099a9004"
	)
	moves := &fakeLocalMoves{states: map[string]account.MoveState{}, refuse: map[string]error{
		noAlias: appfederation.ErrMoveAliasMissing, tooSoon: appaccount.ErrMoveTooSoon,
		moved: appfederation.ErrMoveTargetMoved, gone: appfederation.ErrMoveTargetUnavailable,
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.OwnAccount = own
		o.MoveConfirmations = own
		o.LocalMoves = moves
		o.Moves = fakeMoves{states: moves.states}
		o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
	})
	body := func(target, password string) string {
		return `{"target_account_id":"` + target + `","password":"` + password + `"}`
	}

	res := e.sendJSON(t, http.MethodPost, "/accounts/move", "write", body(bobAccountID, "wrong"))
	if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeWrongPassword {
		t.Errorf("wrong password = %d %+v, want 422 wrong-password", res.StatusCode, p)
	}
	res = e.sendJSON(t, http.MethodPost, "/accounts/move", "write", `{"target_account_id":"`+bobAccountID+`"}`)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("no confirmation = %d, want 422", res.StatusCode)
	}
	for _, tc := range []struct{ target, problem string }{
		{noAlias, clientapi.ProblemTypeMoveAliasMissing},
		{tooSoon, clientapi.ProblemTypeMoveTooSoon},
		{moved, clientapi.ProblemTypeMoveTargetMoved},
		{gone, clientapi.ProblemTypeMoveTargetUnavailable},
	} {
		res := e.sendJSON(t, http.MethodPost, "/accounts/move", "write", body(tc.target, "secret"))
		if p := decodeProblem(t, res); res.StatusCode != http.StatusUnprocessableEntity || p.Type != tc.problem {
			t.Errorf("move to %s = %d %+v, want 422 %s", tc.target, res.StatusCode, p, tc.problem)
		}
	}
	if len(moves.moved) != 0 {
		t.Fatalf("moved = %+v, want none yet", moves.moved)
	}
	res = e.sendJSON(t, http.MethodDelete, "/accounts/move", "write", "")
	if p := decodeProblem(t, res); res.StatusCode != http.StatusConflict || p.Type != clientapi.ProblemTypeAccountNotMoved {
		t.Errorf("cancel before a move = %d %+v, want 409 account-not-moved", res.StatusCode, p)
	}

	res = e.sendJSON(t, http.MethodPost, "/accounts/move", "write", body(bobAccountID, "secret"))
	if res.StatusCode != http.StatusNoContent || len(moves.moved) != 1 || moves.moved[0].AccountID != aliceAccountID ||
		moves.moved[0].TargetAccountID != bobAccountID || moves.moved[0].UserID != userOf(aliceAccountID) {
		t.Fatalf("move = %d %+v, want 204 and alice moving to bob", res.StatusCode, moves.moved)
	}
	refused := func(method, path, body string) {
		t.Helper()
		res := e.sendJSON(t, method, path, "write", body)
		if p := decodeProblem(t, res); res.StatusCode != http.StatusForbidden || p.Type != clientapi.ProblemTypeAccountMoved {
			t.Errorf("%s %s of a moved account = %d %+v, want 403 account-moved", method, path, res.StatusCode, p)
		}
	}
	status := testkit.NamedID("status")
	refused(http.MethodPost, "/statuses", `{"text":"hello","visibility":"public"}`)
	refused(http.MethodPost, "/accounts/"+bobAccountID+"/follow", "")
	refused(http.MethodPost, "/statuses/"+status+"/boost", "")
	refused(http.MethodPost, "/statuses/"+status+"/favourite", "")
	refused(http.MethodPut, "/statuses/"+status+"/reaction", `{"emoji":"👍"}`)
	refused(http.MethodPost, "/statuses/"+status+"/poll/votes", `{"poll_id":"`+testkit.NamedID("poll")+`","choices":[0]}`)

	res = e.sendJSON(t, http.MethodDelete, "/accounts/move", "write", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("cancel = %d, want 204", res.StatusCode)
	}
	res = e.sendJSON(t, http.MethodPost, "/statuses", "write", `{"text":"hello","visibility":"public"}`)
	if res.StatusCode == http.StatusForbidden {
		t.Errorf("post after the cancellation = %d, want it accepted", res.StatusCode)
	}
}

// TestVerifyAccountCredentialsSaysWhereItMoved checks that the credential account carries the
// move as the public account does: after a move its moved names the target, the same account
// as the public read of the viewer; after the cancellation, and for an account that never
// moved, it has none.
func TestVerifyAccountCredentialsSaysWhereItMoved(t *testing.T) {
	own := &fakeOwnAccount{}
	moves := &fakeLocalMoves{states: map[string]account.MoveState{}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.OwnAccount = own
		o.MoveConfirmations = own
		o.LocalMoves = moves
		o.Moves = fakeMoves{states: moves.states}
		o.Accounts.(fakeAccounts)[bobAccountID] = appaccount.Actor{
			Account:   account.Account{ID: bobAccountID, URI: "https://social.example/ap/actors/" + bobAccountID, CreatedAt: createdAt},
			Local:     &account.LocalAccount{AccountID: bobAccountID, UserID: userOf(bobAccountID), Username: "bob"},
			Profile:   &account.ProfileRevision{DisplayName: "Bob"},
			Lifecycle: account.InitialLifecycleState(bobAccountID),
		}
	})
	decode := func(res *http.Response) clientapi.Account {
		t.Helper()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		var got clientapi.Account
		if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	credentials := func() clientapi.Account {
		t.Helper()
		return decode(e.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"}))
	}

	if got := credentials(); got.Moved != nil {
		t.Errorf("moved of an account that never moved = %+v, want none", got.Moved)
	}

	res := e.sendJSON(t, http.MethodPost, "/accounts/move", "write", `{"target_account_id":"`+bobAccountID+`","password":"secret"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("move = %d, want 204", res.StatusCode)
	}
	got := credentials()
	if got.Moved == nil || got.Moved.Id != bobAccountID {
		t.Fatalf("moved after the move = %+v, want the account %s", got.Moved, bobAccountID)
	}
	if want := decode(e.get(t, "/accounts/"+aliceAccountID, nil)).Moved; !reflect.DeepEqual(got.Moved, want) {
		t.Errorf("credential moved = %+v, public moved = %+v, want the same", got.Moved, want)
	}

	res = e.sendJSON(t, http.MethodDelete, "/accounts/move", "write", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("cancel = %d, want 204", res.StatusCode)
	}
	if got := credentials(); got.Moved != nil {
		t.Errorf("moved after the cancellation = %+v, want none", got.Moved)
	}
}
