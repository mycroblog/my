// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/i18n"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/pkg/testkit"
)

// readJSON decodes the body of res into v and returns its members as they were sent.
func readJSON(t *testing.T, res *http.Response, v any) map[string]any {
	t.Helper()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("%v in %s", err, raw)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestMoveRefusalsSayWhy checks the 422 MoveProblem of each refused move (ADR-0089 §8): a move
// within 30 days of the last move is move-too-soon with the can_move_at of the refusal, in UTC; a
// move to an account that it moved to before is move-repeated, without can_move_at; and the other
// refusals have no can_move_at either. Each title is the message of its problem, and a move that
// is not refused still answers 204.
func TestMoveRefusalsSayWhy(t *testing.T) {
	const (
		noAlias  = "01890a5d-ac96-774b-bcce-b302099a9001"
		tooSoon  = "01890a5d-ac96-774b-bcce-b302099a9002"
		moved    = "01890a5d-ac96-774b-bcce-b302099a9003"
		gone     = "01890a5d-ac96-774b-bcce-b302099a9004"
		repeated = "01890a5d-ac96-774b-bcce-b302099a9005"
	)
	canMoveAt := time.Date(2026, 11, 14, 9, 30, 0, 0, time.FixedZone("JST", 9*60*60))
	own := &fakeOwnAccount{}
	moves := &fakeLocalMoves{states: map[string]account.MoveState{}, refuse: map[string]error{
		tooSoon: &appaccount.MoveTooSoonError{CanMoveAt: canMoveAt}, repeated: appaccount.ErrMovedBefore,
		noAlias: appfederation.ErrMoveAliasMissing, moved: appfederation.ErrMoveTargetMoved, gone: appfederation.ErrMoveTargetUnavailable,
	}}
	e := newAuthEnv(t, func(o *clientapi.Options) {
		o.OwnAccount = own
		o.MoveConfirmations = own
		o.LocalMoves = moves
		o.Moves = fakeMoves{states: moves.states}
		o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
	})
	messages, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	move := func(target string) (*http.Response, clientapi.MoveProblem, map[string]any) {
		t.Helper()
		res := e.sendJSON(t, http.MethodPost, "/accounts/move", "write", `{"target_account_id":"`+target+`","password":"secret"}`)
		var p clientapi.MoveProblem
		doc := readJSON(t, res, &p)
		return res, p, doc
	}

	res, p, doc := move(tooSoon)
	l := messages.Localizer(res.Header.Get("Content-Language"))
	if res.StatusCode != http.StatusUnprocessableEntity || p.Status != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeMoveTooSoon ||
		p.Title != l.ServerErrorsMoveTooSoon() {
		t.Errorf("move too soon = %d %+v, want 422 %s titled %q", res.StatusCode, p, clientapi.ProblemTypeMoveTooSoon, l.ServerErrorsMoveTooSoon())
	}
	if want := canMoveAt.UTC().Format(time.RFC3339); doc["can_move_at"] != want {
		t.Errorf("can_move_at of move-too-soon = %v, want %s, in UTC", doc["can_move_at"], want)
	}

	res, p, doc = move(repeated)
	l = messages.Localizer(res.Header.Get("Content-Language"))
	if res.StatusCode != http.StatusUnprocessableEntity || p.Status != http.StatusUnprocessableEntity || p.Type != clientapi.ProblemTypeMoveRepeated ||
		p.Title != l.ServerErrorsMoveRepeated() {
		t.Errorf("move repeated = %d %+v, want 422 %s titled %q", res.StatusCode, p, clientapi.ProblemTypeMoveRepeated, l.ServerErrorsMoveRepeated())
	}
	if _, ok := doc["can_move_at"]; ok {
		t.Errorf("move-repeated = %v, want no can_move_at", doc)
	}

	for _, tc := range []struct{ target, problem string }{
		{noAlias, clientapi.ProblemTypeMoveAliasMissing},
		{moved, clientapi.ProblemTypeMoveTargetMoved},
		{gone, clientapi.ProblemTypeMoveTargetUnavailable},
	} {
		res, p, doc := move(tc.target)
		if res.StatusCode != http.StatusUnprocessableEntity || p.Type != tc.problem {
			t.Errorf("move to %s = %d %+v, want 422 %s", tc.target, res.StatusCode, p, tc.problem)
		}
		if _, ok := doc["can_move_at"]; ok {
			t.Errorf("%s = %v, want no can_move_at", tc.problem, doc)
		}
	}
	if len(moves.moved) != 0 {
		t.Fatalf("moved = %+v, want none", moves.moved)
	}

	res = e.sendJSON(t, http.MethodPost, "/accounts/move", "write", `{"target_account_id":"`+bobAccountID+`","password":"secret"}`)
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("move = %d, want 204", res.StatusCode)
	}
}

// TestCredentialAccountSaysWhenItCanMoveAgain checks last_moved_at and can_move_at of the
// credential account, on verify_credentials and on update_credentials, in UTC: neither for an
// account that never moved, both within 30 days of its last move and after that move is
// cancelled, last_moved_at alone 30 days after it, and neither on a server without moves.
func TestCredentialAccountSaysWhenItCanMoveAgain(t *testing.T) {
	movedAt := time.Date(2026, 10, 15, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	canMoveAt := movedAt.Add(account.LocalMoveInterval)
	waits := map[string]appaccount.MoveWait{}
	states := map[string]account.MoveState{}
	newEnv := func(withMoves bool) *authEnv {
		return newAuthEnv(t, func(o *clientapi.Options) {
			o.Profiles = &fakeProfiles{actor: o.Accounts.(fakeAccounts)[aliceAccountID]}
			o.Accounts.(fakeAccounts)[bobAccountID] = o.Accounts.(fakeAccounts)[aliceAccountID]
			if withMoves {
				o.Moves = fakeMoves{states: states, waits: waits}
			}
		})
	}
	endpoints := []string{"verify_credentials", "update_credentials"}
	// read returns the members of the credential account that each of endpoints answers.
	read := func(e *authEnv) []map[string]any {
		t.Helper()
		var out []map[string]any
		for i, res := range []*http.Response{
			e.get(t, "/accounts/verify_credentials", map[string]string{"Authorization": "Bearer read"}),
			e.sendJSON(t, http.MethodPatch, "/accounts/update_credentials", "write", `{}`),
		} {
			if res.StatusCode != http.StatusOK {
				t.Fatalf("%s = %d, want 200", endpoints[i], res.StatusCode)
			}
			var got clientapi.CredentialAccount
			doc := readJSON(t, res, &got)
			if (got.LastMovedAt == nil) != (doc["last_moved_at"] == nil) || (got.CanMoveAt == nil) != (doc["can_move_at"] == nil) {
				t.Errorf("%s decodes as %v / %v from %v / %v", endpoints[i], got.LastMovedAt, got.CanMoveAt, doc["last_moved_at"], doc["can_move_at"])
			}
			out = append(out, doc)
		}
		return out
	}
	utc := func(at time.Time) string { return at.UTC().Format(time.RFC3339) }
	want := func(what string, docs []map[string]any, last, next string) {
		t.Helper()
		for i, doc := range docs {
			for _, m := range []struct{ name, want string }{{"last_moved_at", last}, {"can_move_at", next}} {
				got, present := doc[m.name]
				switch {
				case m.want == "" && present:
					t.Errorf("%s: %s of %s = %v, want none", what, m.name, endpoints[i], got)
				case m.want != "" && got != m.want:
					t.Errorf("%s: %s of %s = %v, want %s", what, m.name, endpoints[i], got, m.want)
				}
			}
		}
	}

	e := newEnv(true)
	want("never moved", read(e), "", "")

	waits[aliceAccountID] = appaccount.MoveWait{LastMovedAt: movedAt, CanMoveAt: canMoveAt}
	states[aliceAccountID] = account.MoveState{AccountID: aliceAccountID, MoveID: testkit.NamedID("move"), TargetAccountID: bobAccountID, Since: movedAt}
	want("within 30 days", read(e), utc(movedAt), utc(canMoveAt))

	delete(states, aliceAccountID)
	want("after a cancelled move", read(e), utc(movedAt), utc(canMoveAt))

	waits[aliceAccountID] = appaccount.MoveWait{LastMovedAt: movedAt}
	want("after 30 days", read(e), utc(movedAt), "")

	waits[aliceAccountID] = appaccount.MoveWait{LastMovedAt: movedAt, CanMoveAt: canMoveAt}
	want("without moves", read(newEnv(false)), "", "")
}
