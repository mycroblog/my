// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/instance"
)

// TestClosedServer checks the federation endpoints of a closed server (ADR-0088 §2), in the
// federation mode off, which the closure comes before: the inboxes answer 410 before anything
// reads the request; the statuses and the collections are 410 with a Tombstone; WebFinger is
// 410; while the closure's Deletes are delivered, a local actor is its suspended document with
// its key (and the movedTo of an actor that moved) and the instance actor is served whole; once
// they have been delivered, the actors are 410 with a Tombstone too; NodeInfo answers in both.
func TestClosedServer(t *testing.T) {
	s := newServer(t, accountDomain)
	s.mode = federation.ModeOff
	s.closure = instance.ClosureClosed
	s.moves.targets[botID] = remoteID

	for _, path := range []string{"/ap/inbox", "/ap/actors/" + aliceID + "/inbox"} {
		if rec := s.post(t, path, "application/activity+json", []byte(`{"type":"Delete"}`)); rec.Code != http.StatusGone {
			t.Errorf("POST %s while closed = %d, want 410", path, rec.Code)
		}
	}
	if n := len(s.inbox.received()); n != 0 {
		t.Errorf("the inbox received %d requests while closed, want none", n)
	}
	for _, path := range []string{
		"/ap/objects/" + publicStatusID, "/ap/objects/" + publicStatusID + "/activity", "/ap/actors/" + aliceID + "/outbox",
		"/ap/actors/" + aliceID + "/followers", "/ap/objects/" + missingID,
	} {
		rec := s.get(t, path, acceptMastodon)
		doc := decode(t, rec.Body.Bytes())
		if rec.Code != http.StatusGone || doc["type"] != "Tombstone" || doc["id"] != "https://"+host+path {
			t.Errorf("GET %s while closed = %d %s, want 410 and its Tombstone", path, rec.Code, rec.Body)
		}
	}
	for _, path := range []string{"/.well-known/webfinger?resource=acct:alice@" + accountDomain, "/.well-known/host-meta", "/@alice"} {
		if rec := s.get(t, path, acceptMastodon); rec.Code != http.StatusGone {
			t.Errorf("GET %s while closed = %d, want 410", path, rec.Code)
		}
	}

	// While the Deletes are delivered, the actors are served for their keys, whatever the mode.
	rec := s.get(t, "/ap/actors/"+aliceID, acceptMastodon)
	doc := decode(t, rec.Body.Bytes())
	key, _ := doc["publicKey"].(map[string]any)
	if rec.Code != http.StatusOK || doc["suspended"] != true || doc["name"] != "" || key["publicKeyPem"] == nil ||
		rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("GET of alice while delivering = %d %v %s, want her suspended document with its key", rec.Code, rec.Header(), rec.Body)
	}
	rec = s.get(t, "/ap/actors/"+botID, acceptGoToSocial)
	if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusOK || doc["suspended"] != true || doc["movedTo"] != "https://remote.example/users/erin" {
		t.Errorf("GET of the moved bot while delivering = %d %s, want its suspended document with movedTo", rec.Code, rec.Body)
	}
	rec = s.get(t, "/ap/actors/"+instanceID, acceptMastodon)
	if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusOK || doc["suspended"] != nil || doc["publicKey"] == nil {
		t.Errorf("GET of the instance actor while delivering = %d %s, want it whole", rec.Code, rec.Body)
	}
	if rec := s.get(t, "/ap/actors/"+daveID, acceptMastodon); rec.Code != http.StatusNotFound {
		t.Errorf("GET of the deleted dave while delivering = %d, want 404", rec.Code)
	}
	if rec := s.get(t, "/nodeinfo/2.1", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /nodeinfo/2.1 while delivering = %d, want 200", rec.Code)
	}

	// Once the Deletes have been delivered, the actors are gone too.
	s.closure = instance.ClosureDeliveryCompleted
	for _, id := range []string{aliceID, botID, instanceID} {
		rec := s.get(t, "/ap/actors/"+id, acceptMastodon)
		if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusGone || doc["type"] != "Tombstone" || doc["id"] != "https://"+host+"/ap/actors/"+id {
			t.Errorf("GET of %s after the deliveries = %d %s, want 410 and its Tombstone", id, rec.Code, rec.Body)
		}
	}
	if rec := s.get(t, "/nodeinfo/2.1", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /nodeinfo/2.1 after the deliveries = %d, want 200", rec.Code)
	}

	// Announced, the server federates as before.
	s.closure, s.mode = instance.ClosureAnnounced, federation.ModeBoth
	if rec := s.get(t, "/ap/actors/"+aliceID, acceptMastodon); rec.Code != http.StatusOK || decode(t, rec.Body.Bytes())["suspended"] != nil {
		t.Errorf("GET of alice while announced = %d %s, want her document", rec.Code, rec.Body)
	}
}
