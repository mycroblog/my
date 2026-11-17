// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/account"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestAcceptedBoostIsCountedInTheAnswer checks that POST /interaction_requests/{id}/accept
// answers with a status that counts the boost it just approved: the web client shows the counts
// of the answer as they are, so the answer's boosts_count is the count that a GET of the status
// gives next, one more than before the approval.
func TestAcceptedBoostIsCountedInTheAnswer(t *testing.T) {
	t.Parallel()
	e := newTimelineAPI(t)
	ctx := t.Context()
	alice := e.userWithScope(t, "alice", "read write")
	statusID := e.post(t, alice, "boost me", domcontent.VisibilityPublic, "en")

	call := func(method, path, body string) *http.Response {
		t.Helper()
		req := httptest.NewRequest(method, clientapi.BasePath+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+alice.token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		return e.spec.Serve(t, e.h, req)
	}
	// Only the manual rule admits the boost of a stranger: it waits for alice.
	res := call(http.MethodPut, "/statuses/"+statusID+"/interaction_policy", `{"announce":{"automatic":"nobody","manual":"everyone"}}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT interaction_policy = %d", res.StatusCode)
	}

	const actor = "https://remote.example/users/carol"
	var carol account.Account
	err := e.app.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		var err error
		carol, err = e.app.Accounts.UpsertRemoteActor(ctx, tx, appaccount.RemoteActorInput{
			URI: actor, Domain: "remote.example", Type: "Person", PreferredUsername: "carol", Name: "Carol",
			ProfileURL: "https://remote.example/@carol", InboxURL: actor + "/inbox", FollowersURL: actor + "/followers",
			Document: []byte(`{"id":"` + actor + `"}`),
			Handle:   appaccount.HandleCheck{Outcome: account.HandleVerified, Username: "carol", Domain: "remote.example"},
		})
		return err
	})
	if err != nil {
		t.Fatalf("UpsertRemoteActor: %v", err)
	}
	const announce = actor + "/announces/1"
	_, rbErr := e.app.Interactions.RecordRemoteBoost(ctx, appcontent.RemoteBoostInput{
		URI: announce, BoosterAccountID: carol.ID, StatusID: statusID, Visibility: domcontent.VisibilityPublic,
	})
	if rbErr != nil && !errors.Is(rbErr, appcontent.ErrInteractionAwaitsApproval) {
		t.Fatalf("RecordRemoteBoost: %v", rbErr)
	}

	page := decodeJSON[clientapi.InteractionRequestPage](t, call(http.MethodGet, "/interaction_requests", ""))
	if len(page.Items) != 1 || page.Items[0].State != clientapi.InteractionRequestStatePending {
		t.Fatalf("interaction requests = %+v, want carol's boost, pending", page.Items)
	}
	before := decodeJSON[clientapi.Status](t, call(http.MethodGet, "/statuses/"+statusID, "")).BoostsCount
	if before != 0 {
		t.Fatalf("boosts_count before the approval = %d, want 0", before)
	}

	res = call(http.MethodPost, "/interaction_requests/"+page.Items[0].Id+"/accept", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST accept = %d", res.StatusCode)
	}
	accepted := decodeJSON[clientapi.InteractionRequest](t, res)
	if accepted.State != clientapi.InteractionRequestStateAccepted {
		t.Errorf("state = %s, want accepted", accepted.State)
	}
	after := decodeJSON[clientapi.Status](t, call(http.MethodGet, "/statuses/"+statusID, "")).BoostsCount
	if after != before+1 {
		t.Fatalf("GET boosts_count after the approval = %d, want %d", after, before+1)
	}
	if accepted.Status.BoostsCount != after {
		t.Errorf("the answer's boosts_count = %d, want %d: the answer counts the boost it approved", accepted.Status.BoostsCount, after)
	}
}
