// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"context"
	"net/http"
	"testing"
)

// fakeSuspensions is an activitypub.Suspensions that suspends the accounts it holds.
type fakeSuspensions map[string]bool

func (f fakeSuspensions) AccountFederationSuspended(_ context.Context, accountID string) (bool, error) {
	return f[accountID], nil
}

// TestDeactivatedActor checks the federation endpoints of a local actor whose owner deactivated it
// (ADR-0089 §16, §17): GET serves the emptied actor marked suspended, with its key, WebFinger and the
// redirect of the profile page answer, and its inbox takes activities, while its collections are
// missing.
func TestDeactivatedActor(t *testing.T) {
	s := newServer(t, accountDomain)
	rec := s.get(t, "/ap/actors/"+gwenID, acceptMastodon)
	doc := decode(t, rec.Body.Bytes())
	if rec.Code != http.StatusOK || doc["suspended"] != true || doc["publicKey"] == nil || doc["name"] != "" || doc["inbox"] == nil {
		t.Errorf("GET of the deactivated gwen = %d %s, want her suspended actor", rec.Code, rec.Body)
	}
	if rec := s.get(t, "/.well-known/webfinger?resource=acct:gwen@"+accountDomain, "application/jrd+json"); rec.Code != http.StatusOK {
		t.Errorf("WebFinger of gwen = %d, want 200", rec.Code)
	}
	if rec := s.get(t, "/@gwen", acceptMastodon); rec.Code != http.StatusFound {
		t.Errorf("the profile page of gwen for ActivityPub = %d, want 302", rec.Code)
	}
	if rec := s.get(t, "/ap/actors/"+gwenID+"/outbox", acceptMastodon); rec.Code != http.StatusNotFound {
		t.Errorf("the outbox of gwen = %d, want 404", rec.Code)
	}
	if rec := s.post(t, "/ap/actors/"+gwenID+"/inbox", "application/activity+json", []byte(`{"type":"Follow"}`)); rec.Code != http.StatusAccepted {
		t.Errorf("POST to the inbox of gwen = %d, want 202", rec.Code)
	}
}

// TestSuspendedActor checks GET of a local actor that moderation suspends (federation.md §11):
// the emptied actor marked suspended, with its keys, also while its lifecycle hides it (a ban
// schedules the deletion), and 410 once it is deleted.
func TestSuspendedActor(t *testing.T) {
	s := newServer(t, accountDomain)
	for _, id := range []string{aliceID, carolID, daveID} {
		s.suspended[id] = true
	}
	rec := s.get(t, "/ap/actors/"+aliceID, acceptMastodon)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/activity+json" {
		t.Fatalf("GET of the suspended alice = %d %v", rec.Code, rec.Header())
	}
	checkGolden(t, "actor-suspended.json", rec.Body.Bytes())

	rec = s.get(t, "/ap/actors/"+carolID, acceptMastodon)
	if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusOK || doc["suspended"] != true || doc["publicKey"] == nil {
		t.Errorf("GET of the suspended carol, whose deletion is scheduled, = %d %s, want her suspended actor", rec.Code, rec.Body)
	}
	if rec := s.get(t, "/ap/actors/"+daveID, acceptMastodon); rec.Code != http.StatusGone {
		t.Errorf("GET of the suspended, deleted dave = %d, want 410", rec.Code)
	}

	// The suspension ends: the profile is back.
	s.suspended[aliceID] = false
	rec = s.get(t, "/ap/actors/"+aliceID, acceptMastodon)
	if doc := decode(t, rec.Body.Bytes()); rec.Code != http.StatusOK || doc["suspended"] != nil || doc["name"] != "Alice" {
		t.Errorf("GET of alice after the suspension = %d %s, want her profile", rec.Code, rec.Body)
	}
}

// TestDeletedActorAnswersGone checks ADR-0018 §12: a deleted local actor answers 410, with a
// Tombstone of the actor's URI under /ap/, on every way to reach it, and what is merely missing or
// not active stays 404.
func TestDeletedActorAnswersGone(t *testing.T) {
	s := newServer(t, accountDomain)
	uri := "https://" + host + "/ap/actors/" + daveID
	for _, accept := range []string{acceptMastodon, ""} {
		rec := s.get(t, "/ap/actors/"+daveID, accept)
		if rec.Code != http.StatusGone || rec.Header().Get("Content-Type") != "application/activity+json" {
			t.Fatalf("GET of the deleted dave with Accept %q = %d %v", accept, rec.Code, rec.Header())
		}
		if doc := decode(t, rec.Body.Bytes()); doc["type"] != "Tombstone" || doc["id"] != uri {
			t.Errorf("the document of the deleted dave = %v, want a Tombstone of %s", doc, uri)
		}
	}
	for _, target := range []string{
		"/.well-known/webfinger?resource=acct:dave@" + accountDomain,
		"/.well-known/webfinger?resource=acct:dave@" + host,
		"/.well-known/webfinger?resource=" + uri,
		"/.well-known/webfinger?resource=https://" + host + "/@dave",
	} {
		if rec := s.get(t, target, ""); rec.Code != http.StatusGone {
			t.Errorf("GET %s = %d, want 410", target, rec.Code)
		}
	}
	if rec := s.get(t, "/@dave", acceptMastodon); rec.Code != http.StatusGone {
		t.Errorf("GET /@dave as ActivityPub = %d, want 410", rec.Code)
	}
	if rec := s.get(t, "/ap/actors/"+daveID, acceptBrowser); rec.Code != http.StatusNotFound {
		t.Errorf("GET of the deleted dave from a browser = %d, want 404", rec.Code)
	}
	for _, target := range []string{
		"/ap/actors/" + daveID + "/outbox",
		"/ap/actors/" + carolID,
		"/ap/actors/" + remoteID,
		"/ap/actors/" + missingID,
		"/.well-known/webfinger?resource=acct:carol@" + accountDomain,
		"/.well-known/webfinger?resource=acct:nobody@" + accountDomain,
	} {
		if rec := s.get(t, target, acceptMastodon); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, rec.Code)
		}
	}
}
