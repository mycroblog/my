// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"context"
	"net/http"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/domain/account"
)

// fakeMoves is an appfederation.MoveReader of the moves it holds: the target of each account
// that moved, by account ID.
type fakeMoves struct {
	accounts *fakeAccounts
	targets  map[string]string
}

func (f fakeMoves) MoveState(_ context.Context, accountID string) (appaccount.MoveState, error) {
	t, ok := f.targets[accountID]
	if !ok {
		return appaccount.MoveState{}, appaccount.ErrNotFound
	}
	return appaccount.MoveState{AccountID: accountID, MoveID: "move-" + accountID, TargetAccountID: t}, nil
}

func (f fakeMoves) FindMove(context.Context, string) (appaccount.Move, error) {
	return appaccount.Move{}, appaccount.ErrNotFound
}

func (f fakeMoves) FindByID(ctx context.Context, id string) (appaccount.Actor, error) {
	return f.accounts.FindByID(ctx, id)
}

// TestMovedActor checks GET of a local actor that moved (ADR-0089, federation.md §10): its
// document names the target in movedTo, also in the minimal document that authorized fetch
// serves to an unsigned request (Misskey reads it when the Move arrives), and has none once the
// move is cancelled.
func TestMovedActor(t *testing.T) {
	s := newServer(t, accountDomain)
	target := "https://remote.example/users/erin"
	s.moves.targets[aliceID] = remoteID
	rec := s.get(t, "/ap/actors/"+aliceID, acceptMastodon)
	if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusOK || doc["movedTo"] != target || doc["name"] != "Alice" {
		t.Fatalf("GET of the moved alice = %d %s, want her actor with movedTo %s", rec.Code, rec.Body, target)
	}
	s.objects.authorizedFetch = true
	rec = s.get(t, "/ap/actors/"+aliceID, acceptMastodon)
	if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusOK || doc["movedTo"] != target || doc["name"] != nil {
		t.Errorf("unsigned GET of the moved alice under authorized fetch = %d %s, want the minimal document with movedTo", rec.Code, rec.Body)
	}
	s.objects.authorizedFetch = false

	delete(s.moves.targets, aliceID)
	rec = s.get(t, "/ap/actors/"+aliceID, acceptMastodon)
	if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusOK || doc["movedTo"] != nil {
		t.Errorf("GET of alice after the move is cancelled = %d %s, want no movedTo", rec.Code, rec.Body)
	}
	if got := account.ActorURI(host, remoteID); got == target {
		t.Fatalf("the remote target has a local URI")
	}
}
