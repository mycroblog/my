// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/mycroblog/my/internal/domain/federation"
	"github.com/mycroblog/my/internal/domain/graph"
)

// getSigned serves one GET whose fake signature names the requester's domain ("" for none). The
// request arrives on an internal address, as behind a reverse proxy.
func (s *server) getSigned(t *testing.T, target, accept, domain string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://internal:8080"+target, nil)
	req.Header.Set("Accept", accept)
	if domain != "" {
		req.Header.Set("Signature", domain)
	}
	return serve(s, req)
}

func TestObjects(t *testing.T) {
	s := newServer(t, accountDomain)
	object := "/ap/objects/"
	tests := []struct {
		name, target, signer string
		status               int
		cache, typ           string
	}{
		{"public note", object + publicStatusID, "", http.StatusOK, "public, max-age=60", "Note"},
		{"public create", object + publicStatusID + "/activity", "", http.StatusOK, "public, max-age=60", "Create"},
		{"public note, signed", object + publicStatusID, strangerDomain, http.StatusOK, "public, max-age=60", "Note"},
		{"followers-only note for a follower's server", object + followersOnlyID, followerDomain, http.StatusOK, "private, no-store", "Note"},
		{"followers-only create for a follower's server", object + followersOnlyID + "/activity", followerDomain, http.StatusOK, "private, no-store", "Create"},
		{"deleted", object + deletedStatusID, "", http.StatusGone, "public, max-age=60", "Tombstone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := s.getSigned(t, tt.target, acceptMastodon, tt.signer)
			if rec.Code != tt.status {
				t.Fatalf("GET %s = %d %s, want %d", tt.target, rec.Code, rec.Body, tt.status)
			}
			h := rec.Header()
			if h.Get("Content-Type") != "application/activity+json" || h.Get("Cache-Control") != tt.cache || h.Get("Vary") != "Accept" {
				t.Errorf("headers = %v, want activity+json, Cache-Control %q, Vary Accept", h, tt.cache)
			}
			if got := decode(t, rec.Body.Bytes())["type"]; got != tt.typ {
				t.Errorf("type = %v, want %s", got, tt.typ)
			}
		})
	}

	t.Run("signature covers the public origin", func(t *testing.T) {
		s.getSigned(t, object+publicStatusID+"?x=1", acceptMastodon, followerDomain)
		got := s.objects.requests[len(s.objects.requests)-1]
		if got.URL.String() != "https://"+host+object+publicStatusID+"?x=1" || got.Header.Get("Host") != host || got.Method != http.MethodGet {
			t.Errorf("signed request = %s %s Host %q, want a GET of https://%s", got.Method, got.URL, got.Header.Get("Host"), host)
		}
	})

	t.Run("HTML", func(t *testing.T) {
		rec := s.get(t, object+publicStatusID, acceptBrowser)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != statusPagePrefix+publicStatusID {
			t.Errorf("GET = %d Location %q, want 303 to the status page", rec.Code, rec.Header().Get("Location"))
		}
		// A browser carries no signature: only what anyone may see is redirected.
		for _, id := range []string{followersOnlyID, deletedStatusID, missingStatusID} {
			if rec := s.getSigned(t, object+id, acceptBrowser, followerDomain); rec.Code != http.StatusNotFound {
				t.Errorf("GET %s as HTML = %d, want 404", id, rec.Code)
			}
		}
	})

	t.Run("errors are 500", func(t *testing.T) {
		s := newServer(t, accountDomain)
		s.objects.err = errors.New("database is down")
		if rec := s.get(t, object+publicStatusID, acceptMastodon); rec.Code != http.StatusInternalServerError {
			t.Errorf("GET = %d, want 500", rec.Code)
		}
	})
}

// TestObjectNotFoundParity checks that a status hidden from the requester, a missing one, and a
// malformed id get byte-identical 404 responses (ADR-0018).
func TestObjectNotFoundParity(t *testing.T) {
	s := newServer(t, accountDomain)
	type target struct{ path, signer string }
	targets := []target{
		{"/ap/objects/" + missingStatusID, ""},
		{"/ap/objects/" + followersOnlyID, ""},
		{"/ap/objects/" + followersOnlyID, strangerDomain},
		{"/ap/objects/" + followersOnlyID + "/activity", strangerDomain},
		{"/ap/objects/" + missingStatusID + "/activity", followerDomain},
		{"/ap/objects/0192F3B0-2C3D-7E4F-8A00-112233445501", ""}, // not the canonical form
		{"/ap/objects/not-a-uuid", followerDomain},
	}
	first := s.getSigned(t, targets[0].path, acceptMastodon, targets[0].signer)
	if first.Code != http.StatusNotFound || first.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET %s = %d Cache-Control %q, want 404 no-store", targets[0].path, first.Code, first.Header().Get("Cache-Control"))
	}
	for _, tg := range targets[1:] {
		rec := s.getSigned(t, tg.path, acceptMastodon, tg.signer)
		if rec.Code != first.Code || !reflect.DeepEqual(rec.Header(), first.Header()) || rec.Body.String() != first.Body.String() {
			t.Errorf("GET %s signed by %q = %d %v %q, want the response of the missing status", tg.path, tg.signer, rec.Code, rec.Header(), rec.Body)
		}
	}
}

func TestCollectionPages(t *testing.T) {
	s := newServer(t, accountDomain)
	actor := "https://" + host + "/ap/actors/" + aliceID
	create := func(id string) federation.ActivityDocument {
		return federation.ActivityDocument{ID: federation.IRI("https://" + host + "/ap/objects/" + id + "/activity"), Type: federation.ActivityCreate}
	}
	s.objects.outbox = []federation.ActivityDocument{create(publicStatusID), create(outboxNext)}
	s.objects.featured = []federation.NoteDocument{{ObjectDocument: federation.ObjectDocument{ID: federation.IRI("https://" + host + "/ap/objects/" + publicStatusID), Type: federation.ObjectNote}}}
	s.objects.featuredTags = []federation.TagDocument{
		{Type: federation.TagHashtag, Href: "https://" + host + "/@alice/tagged/cats", Name: "#Cats"},
		{Type: federation.TagHashtag, Href: "https://" + host + "/@alice/tagged/dogs", Name: "#dogs"},
	}
	// Botty follows Alice; Alice follows Botty and the deactivated Carol, who is left out.
	s.graph.edges = []graph.Edge{
		{FollowerAccountID: botID, FolloweeAccountID: aliceID, FollowID: "0192f3c0-0000-7000-8000-000000000003", State: graph.StateActive, Since: created},
		{FollowerAccountID: aliceID, FolloweeAccountID: carolID, FollowID: "0192f3c0-0000-7000-8000-000000000002", State: graph.StateActive, Since: created},
		{FollowerAccountID: aliceID, FolloweeAccountID: botID, FollowID: "0192f3c0-0000-7000-8000-000000000001", State: graph.StateActive, Since: created},
	}
	tests := []struct{ golden, target string }{
		{"collection-outbox-page.json", "/ap/actors/" + aliceID + "/outbox?page=true"},
		{"collection-outbox-next-page.json", "/ap/actors/" + aliceID + "/outbox?max_id=" + outboxNext + "&page=true"},
		{"collection-followers-page.json", "/ap/actors/" + aliceID + "/followers?page=true"},
		{"collection-following-page.json", "/ap/actors/" + aliceID + "/following?page=true"},
		{"collection-featured-items.json", "/ap/actors/" + aliceID + "/featured"},
		{"collection-featured-tags-items.json", "/ap/actors/" + aliceID + "/featured-tags"},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			rec := s.get(t, tt.target, acceptMastodon)
			if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=60" {
				t.Fatalf("GET %s = %d %v %s", tt.target, rec.Code, rec.Header(), rec.Body)
			}
			checkGolden(t, tt.golden, rec.Body.Bytes())
		})
	}

	t.Run("collection links to its first page", func(t *testing.T) {
		doc := decode(t, s.get(t, "/ap/actors/"+aliceID+"/followers", acceptMastodon).Body.Bytes())
		if doc["first"] != actor+"/followers?page=true" || doc["totalItems"] != 1.0 {
			t.Errorf("followers = %v, want 1 follower and the first page", doc)
		}
	})

	t.Run("malformed cursor", func(t *testing.T) {
		if rec := s.get(t, "/ap/actors/"+aliceID+"/outbox?page=true&max_id=nope", acceptMastodon); rec.Code != http.StatusNotFound {
			t.Errorf("GET = %d, want 404", rec.Code)
		}
	})
}

// TestAnnounceRoute checks GET of an activity id: the handler serves what Objects answers, 404 for
// any other id and for a browser, and authorized fetch asks for a signature first.
func TestAnnounceRoute(t *testing.T) {
	s := newServer(t, accountDomain)
	target := "/ap/activities/" + announceID
	rec := s.get(t, target, acceptMastodon)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/activity+json" ||
		rec.Header().Get("Cache-Control") != "public, max-age=60" || decode(t, rec.Body.Bytes())["type"] != "Announce" {
		t.Errorf("GET the Announce = %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	for name, get := range map[string]func() int{
		"another activity": func() int { return s.get(t, "/ap/activities/"+missingStatusID, acceptMastodon).Code },
		"an invalid id":    func() int { return s.get(t, "/ap/activities/x", acceptMastodon).Code },
		"a browser":        func() int { return s.get(t, target, acceptBrowser).Code },
	} {
		if code := get(); code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", name, code)
		}
	}
	s.objects.authorizedFetch = true
	if rec := s.getSigned(t, target, acceptMastodon, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unsigned GET under authorized fetch = %d, want 401", rec.Code)
	}
	if rec := s.getSigned(t, target, acceptMastodon, followerDomain); rec.Code != http.StatusOK {
		t.Errorf("signed GET under authorized fetch = %d, want 200", rec.Code)
	}
}
