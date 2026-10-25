// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestServiceClosureFederation closes a server whose federation mode is off (ADR-0088 §2, §3)
// with the inline queue: the closure's Delete of the local actor still reaches its remote
// follower; while it is delivered, the inbox and WebFinger are 410 and the actor is its
// suspended document with its key; the progress counts the account and its delivery; and once
// the periodic check sees every delivery ended, it records delivery_completed and the actor is
// 410 too.
func TestServiceClosureFederation(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network}, "alice")
	ctx := t.Context()
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	aliceID := c.ids["alice"]
	alice := "https://example.test/ap/actors/" + aliceID
	if _, err := c.app.Accounts.EnsureInstanceActor(ctx); err != nil {
		t.Fatal(err)
	}
	if code := inboxCode(t, c, carol, map[string]any{"id": carol.id + "/follows/1", "type": "Follow", "actor": carol.id, "object": alice}); code != http.StatusAccepted {
		t.Fatalf("Follow = %d", code)
	}
	// sent takes what carol's inbox received since the last call.
	sent := func() ([]string, []map[string]any) {
		t.Helper()
		var types []string
		docs := network.taken()[carol.id+"/inbox"]
		for _, p := range docs {
			types = append(types, p["type"].(string))
		}
		return types, docs
	}
	if got, _ := sent(); len(got) != 1 || got[0] != "Accept" {
		t.Fatalf("sent to carol after her Follow = %v, want an Accept", got)
	}

	// The operator narrowed the federation to off during the announcement, then closes.
	if res := c.do("alice", http.MethodPatch, "/admin/settings", map[string]any{"values": map[string]any{"federation_mode": "off"}}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /admin/settings = %d", res.StatusCode)
	}
	if res := c.do("alice", http.MethodPut, "/admin/closure/announcement", map[string]any{"closes_at": time.Now().Add(-time.Minute)}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("announce = %d", res.StatusCode)
	}
	var inst clientapi.Instance
	if res := c.do("alice", http.MethodGet, "/instance", nil, &inst); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /instance = %d", res.StatusCode)
	}
	if res := c.do("alice", http.MethodPost, "/admin/closure/close", map[string]any{"confirm_domain": inst.Domain}, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("close = %d", res.StatusCode)
	}
	got, docs := sent()
	if len(got) != 1 || got[0] != "Delete" {
		t.Fatalf("sent to carol after the closure = %v, want the Delete of alice", got)
	}
	if del := docs[0]; del["actor"] != alice || del["object"] != alice {
		t.Errorf("the closure's Delete = %v, want the Delete of alice's actor", del)
	}

	// While the Deletes are delivered: the inbox and WebFinger are gone, the actor serves its key.
	if code := inboxCode(t, c, carol, map[string]any{"id": carol.id + "/likes/1", "type": "Like", "actor": carol.id, "object": alice}); code != http.StatusGone {
		t.Errorf("POST /ap/inbox while closed = %d, want 410", code)
	}
	if code, _ := apGet(t, c, "/.well-known/webfinger?resource=acct:alice@example.test"); code != http.StatusGone {
		t.Errorf("WebFinger while closed = %d, want 410", code)
	}
	code, doc := apGet(t, c, "/ap/actors/"+aliceID)
	if key, _ := doc["publicKey"].(map[string]any); code != http.StatusOK || doc["suspended"] != true || key["publicKeyPem"] == nil {
		t.Errorf("GET of alice while delivering = %d %v, want her suspended document with its key", code, doc)
	}
	var closure clientapi.AdminServiceClosure
	if res := c.do("alice", http.MethodGet, "/admin/closure", nil, &closure); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/closure = %d", res.StatusCode)
	}
	if p := closure.Progress; closure.State != clientapi.ServiceClosureStateClosed || p == nil || p.Accounts != 1 || p.FanoutsPending != 0 ||
		p.Deliveries.Succeeded != 1 || p.Deliveries.Pending != 0 {
		t.Fatalf("closure while delivering = %+v (progress %+v), want one account with one delivery that succeeded", closure, closure.Progress)
	}

	// The periodic check records delivery_completed; then the actor is gone too.
	if err := c.app.ClosureDeletes.Complete(ctx); err != nil {
		t.Fatal(err)
	}
	if res := c.do("alice", http.MethodGet, "/admin/closure", nil, &closure); res.StatusCode != http.StatusOK || closure.State != clientapi.ServiceClosureStateDeliveryCompleted {
		t.Fatalf("closure after the check = %d %+v, want delivery_completed", res.StatusCode, closure)
	}
	if code, doc := apGet(t, c, "/ap/actors/"+aliceID); code != http.StatusGone || doc["type"] != "Tombstone" {
		t.Errorf("GET of alice after the deliveries = %d %v, want 410 and a Tombstone", code, doc)
	}
	if got, _ := sent(); len(got) != 0 {
		t.Errorf("sent to carol in the end = %v, want nothing after the Delete", got)
	}
}
