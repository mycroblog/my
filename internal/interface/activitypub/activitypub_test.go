// SPDX-License-Identifier: AGPL-3.0-only

package activitypub_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/mycroblog/my/internal/domain/account"
	"github.com/mycroblog/my/internal/interface/activitypub"
)

// TestGolden pins every document the endpoints serve (ADR-0015).
func TestGolden(t *testing.T) {
	s := newServer(t, accountDomain)
	actor := "/ap/actors/" + aliceID
	tests := []struct {
		golden, target, accept, contentType string
	}{
		{"webfinger-acct.json", "/.well-known/webfinger?resource=acct:alice@" + accountDomain, "", "application/jrd+json; charset=utf-8"},
		{"webfinger-instance-actor.json", "/.well-known/webfinger?resource=acct:" + accountDomain + "@" + accountDomain, "", "application/jrd+json; charset=utf-8"},
		{"host-meta.xml", "/.well-known/host-meta", "", "application/xrd+xml; charset=utf-8"},
		{"host-meta.json", "/.well-known/host-meta.json", "", "application/jrd+json; charset=utf-8"},
		{"nodeinfo-index.json", "/.well-known/nodeinfo", "", "application/json; charset=utf-8"},
		{"nodeinfo.json", "/nodeinfo/2.1", "", `application/json; profile="http://nodeinfo.diaspora.software/ns/schema/2.1#"`},
		{"actor-person.json", actor, acceptMastodon, "application/activity+json"},
		{"actor-bot.json", "/ap/actors/" + botID, acceptMastodon, "application/activity+json"},
		{"actor-instance.json", "/ap/actors/" + instanceID, acceptMastodon, "application/activity+json"},
		{"collection-outbox.json", actor + "/outbox", acceptMastodon, "application/activity+json"},
		{"collection-followers.json", actor + "/followers", acceptMastodon, "application/activity+json"},
		{"collection-following.json", actor + "/following", acceptMastodon, "application/activity+json"},
		{"collection-featured.json", actor + "/featured", acceptMastodon, "application/activity+json"},
		{"collection-featured-tags.json", actor + "/featured-tags", acceptMastodon, "application/activity+json"},
	}
	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			rec := s.get(t, tt.target, tt.accept)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d %s", tt.target, rec.Code, rec.Body)
			}
			h := rec.Header()
			if got := h.Get("Content-Type"); got != tt.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.contentType)
			}
			if got := h.Get("Cache-Control"); got != "public, max-age=60" {
				t.Errorf("Cache-Control = %q, want public, max-age=60", got)
			}
			if got := h.Get("Cross-Origin-Resource-Policy"); got != "cross-origin" {
				t.Errorf("Cross-Origin-Resource-Policy = %q, want cross-origin", got)
			}
			checkGolden(t, tt.golden, rec.Body.Bytes())
		})
	}
}

func TestWellKnownAllowsAnyOrigin(t *testing.T) {
	s := newServer(t, accountDomain)
	for _, target := range []string{
		"/.well-known/webfinger?resource=acct:alice@" + accountDomain,
		"/.well-known/webfinger?resource=acct:nobody@" + accountDomain,
		"/.well-known/host-meta",
		"/.well-known/host-meta.json",
		"/.well-known/nodeinfo",
		"/nodeinfo/2.1",
	} {
		if got := s.get(t, target, "").Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("GET %s: Access-Control-Allow-Origin = %q, want *", target, got)
		}
	}
}

func TestWebFingerResources(t *testing.T) {
	s := newServer(t, accountDomain)
	actorURI := "https://" + host + "/ap/actors/" + aliceID
	for _, resource := range []string{
		"acct:alice@" + accountDomain,
		"acct:Alice@" + accountDomain, // usernames compare case-insensitively
		"acct:alice@" + host,          // the serving domain also answers (ADR-0018)
		"acct:alice@EXAMPLE.ORG",
		"alice@" + accountDomain, // Mastodon sometimes omits the scheme
		actorURI,
		"https://" + host + "/@alice",
	} {
		rec := s.get(t, "/.well-known/webfinger?resource="+url.QueryEscape(resource), "")
		if rec.Code != http.StatusOK {
			t.Errorf("resource %s: status %d, want 200", resource, rec.Code)
			continue
		}
		doc := decode(t, rec.Body.Bytes())
		if doc["subject"] != "acct:alice@"+accountDomain {
			t.Errorf("resource %s: subject = %v, want the account-domain handle", resource, doc["subject"])
		}
	}

	t.Run("rel filter", func(t *testing.T) {
		rec := s.get(t, "/.well-known/webfinger?rel=self&resource=acct:alice@"+accountDomain, "")
		links := decode(t, rec.Body.Bytes())["links"].([]any)
		if len(links) != 1 || links[0].(map[string]any)["rel"] != "self" {
			t.Errorf("links = %v, want only self", links)
		}
	})

	t.Run("missing resource", func(t *testing.T) {
		if rec := s.get(t, "/.well-known/webfinger", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

// TestAccountDomainRedirect checks that a separate account domain sends its well-known
// endpoints to the host (ADR-0018).
func TestAccountDomainRedirect(t *testing.T) {
	s := newServer(t, accountDomain)
	for _, target := range []string{
		"/.well-known/webfinger?resource=acct%3Aalice%40example.org",
		"/.well-known/host-meta",
		"/.well-known/host-meta.json",
		"/.well-known/nodeinfo",
	} {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+accountDomain+target, nil)
		rec := serve(s, req)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "https://"+host+target {
			t.Errorf("GET %s on %s = %d Location %q, want 301 to the host", target, accountDomain, rec.Code, rec.Header().Get("Location"))
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("GET %s on %s lacks Access-Control-Allow-Origin", target, accountDomain)
		}
	}

	t.Run("no redirect without a separate domain", func(t *testing.T) {
		s := newServer(t, "")
		rec := s.get(t, "/.well-known/webfinger?resource=acct:alice@"+host, "")
		if rec.Code != http.StatusOK || decode(t, rec.Body.Bytes())["subject"] != "acct:alice@"+host {
			t.Errorf("status = %d, body %s; want the handle on the host", rec.Code, rec.Body)
		}
	})
}

func TestActorNegotiation(t *testing.T) {
	s := newServer(t, accountDomain)
	actor := "/ap/actors/" + aliceID
	profile := "https://" + host + "/@alice"
	tests := []struct {
		name, accept string
		status       int
		location     string
	}{
		{"Mastodon", acceptMastodon, http.StatusOK, ""},
		{"GoToSocial", acceptGoToSocial, http.StatusOK, ""},
		{"Misskey", acceptMisskey, http.StatusOK, ""},
		{"ld+json without profile", "application/ld+json", http.StatusOK, ""},
		{"plain JSON", "application/json", http.StatusOK, ""},
		{"no Accept", "", http.StatusOK, ""},
		{"anything", "*/*", http.StatusOK, ""},
		{"browser", acceptBrowser, http.StatusSeeOther, profile},
		{"HTML preferred over JSON", "application/activity+json;q=0.5, text/html", http.StatusSeeOther, profile},
		{"JSON preferred over HTML", "application/activity+json, text/html;q=0.5", http.StatusOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := s.get(t, actor, tt.accept)
			if rec.Code != tt.status || rec.Header().Get("Location") != tt.location {
				t.Fatalf("status = %d Location %q, want %d %q", rec.Code, rec.Header().Get("Location"), tt.status, tt.location)
			}
			if got := rec.Header().Values("Vary"); !reflect.DeepEqual(got, []string{"Accept"}) {
				t.Errorf("Vary = %q, want Accept", got)
			}
			if rec.Code == http.StatusOK && rec.Header().Get("Content-Type") != "application/activity+json" {
				t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
			}
		})
	}

	t.Run("the instance actor has no profile page", func(t *testing.T) {
		if rec := s.get(t, "/ap/actors/"+instanceID, acceptBrowser); rec.Code != http.StatusOK {
			t.Errorf("status = %d, want the document", rec.Code)
		}
	})
}

func TestProfilePageRedirectsActivityPub(t *testing.T) {
	s := newServer(t, accountDomain)
	tests := []struct {
		name, path, accept string
		status             int
		location           string
	}{
		{"Mastodon", "/@alice", acceptMastodon, http.StatusFound, "https://" + host + "/ap/actors/" + aliceID},
		{"GoToSocial", "/@alice", acceptGoToSocial, http.StatusFound, "https://" + host + "/ap/actors/" + aliceID},
		{"Misskey", "/@alice", acceptMisskey, http.StatusFound, "https://" + host + "/ap/actors/" + aliceID},
		{"case-insensitive", "/@ALICE", acceptMastodon, http.StatusFound, "https://" + host + "/ap/actors/" + aliceID},
		{"bot", "/@botty", acceptMastodon, http.StatusFound, "https://" + host + "/ap/actors/" + botID},
		{"browser", "/@alice", acceptBrowser, http.StatusNotFound, ""},
		{"wildcard", "/@alice", "*/*", http.StatusNotFound, ""},
		{"no Accept", "/@alice", "", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := s.get(t, tt.path, tt.accept)
			if rec.Code != tt.status || rec.Header().Get("Location") != tt.location {
				t.Fatalf("status = %d Location %q, want %d %q", rec.Code, rec.Header().Get("Location"), tt.status, tt.location)
			}
			if got := rec.Header().Values("Vary"); !reflect.DeepEqual(got, []string{"Accept"}) {
				t.Errorf("Vary = %q, want Accept", got)
			}
		})
	}
}

// TestProfilePageServesHTML checks that requests that do not ask for ActivityPub get the web
// client's HTML at profile pages and other single-segment paths, while ActivityPub requests
// keep their redirects and 404s.
func TestProfilePageServesHTML(t *testing.T) {
	s := &server{mux: http.NewServeMux(), accounts: newAccounts(t)}
	activitypub.New(activitypub.Options{
		Host: host, Accounts: s.accounts, Problem: problem,
		HTML: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("shell")) }),
	}).Register(s.mux)
	tests := []struct {
		name, path, accept string
		status             int
		shell              bool
		vary               []string
	}{
		{"browser", "/@alice", acceptBrowser, http.StatusOK, true, []string{"Accept"}},
		{"wildcard", "/@alice", "*/*", http.StatusOK, true, []string{"Accept"}},
		{"no Accept", "/@alice", "", http.StatusOK, true, []string{"Accept"}},
		{"missing account", "/@nobody", acceptBrowser, http.StatusOK, true, []string{"Accept"}},
		{"Mastodon", "/@alice", acceptMastodon, http.StatusFound, false, []string{"Accept"}},
		{"Mastodon, missing account", "/@nobody", acceptMastodon, http.StatusNotFound, false, []string{"Accept"}},
		{"SPA route", "/home", acceptBrowser, http.StatusOK, true, []string{"Accept"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := s.get(t, tt.path, tt.accept)
			if rec.Code != tt.status || (rec.Body.String() == "shell") != tt.shell {
				t.Fatalf("GET %s = %d %q, want %d (shell: %v)", tt.path, rec.Code, rec.Body, tt.status, tt.shell)
			}
			if got := rec.Header().Values("Vary"); !reflect.DeepEqual(got, tt.vary) {
				t.Errorf("Vary = %q, want %q", got, tt.vary)
			}
		})
	}
}

// TestPagesAnswerActivityPub checks the web client's pages (Handler.Pages): requests for HTML
// get the page, and requests for ActivityPub get 404, except that a post page that an anonymous
// visitor may see redirects to the post's id with 302 (ADR-0053).
func TestPagesAnswerActivityPub(t *testing.T) {
	s := &server{mux: http.NewServeMux(), accounts: newAccounts(t), objects: newObjects()}
	h := activitypub.New(activitypub.Options{
		Host: host, Accounts: s.accounts, Objects: s.objects, Problem: problem,
		HTML: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("shell")) }),
	})
	h.Register(s.mux)
	s.mux.Handle("GET /", h.Pages())
	id := "https://" + host + "/ap/objects/" + publicStatusID
	tests := []struct {
		name, path, accept string
		status             int
		location           string
		shell              bool
	}{
		{"browser", "/@alice/status/" + publicStatusID, acceptBrowser, http.StatusOK, "", true},
		{"Mastodon", "/@alice/status/" + publicStatusID, acceptMastodon, http.StatusFound, id, false},
		{"Mastodon, another name", "/@renamed/status/" + publicStatusID, acceptMastodon, http.StatusFound, id, false},
		{"Mastodon, followers-only", "/@alice/status/" + followersOnlyID, acceptMastodon, http.StatusNotFound, "", false},
		{"Mastodon, deleted", "/@alice/status/" + deletedStatusID, acceptMastodon, http.StatusNotFound, "", false},
		{"Mastodon, missing", "/@alice/status/" + missingStatusID, acceptMastodon, http.StatusNotFound, "", false},
		{"Mastodon, not an ID", "/@alice/status/abc", acceptMastodon, http.StatusNotFound, "", false},
		{"Mastodon, the form without status", "/@alice/" + publicStatusID, acceptMastodon, http.StatusNotFound, "", false},
		{"Mastodon, a page under the post", "/@alice/status/" + publicStatusID + "/boosts", acceptMastodon, http.StatusNotFound, "", false},
		{"Mastodon, another page", "/@alice/followers", acceptMastodon, http.StatusNotFound, "", false},
		{"browser, SPA route", "/settings/profile", acceptBrowser, http.StatusOK, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := s.get(t, tt.path, tt.accept)
			if rec.Code != tt.status || rec.Header().Get("Location") != tt.location || (rec.Body.String() == "shell") != tt.shell {
				t.Fatalf("GET %s = %d Location %q %q, want %d %q (shell: %v)", tt.path, rec.Code, rec.Header().Get("Location"), rec.Body, tt.status, tt.location, tt.shell)
			}
			if got := rec.Header().Values("Vary"); !reflect.DeepEqual(got, []string{"Accept"}) {
				t.Errorf("Vary = %q, want Accept", got)
			}
		})
	}
}

// TestNotFoundParity checks that missing resources and resources that may not be shown get
// byte-identical responses (ADR-0018), within each endpoint.
func TestNotFoundParity(t *testing.T) {
	s := newServer(t, accountDomain)
	groups := map[string][]string{
		"actor": {
			"/ap/actors/" + missingID,
			"/ap/actors/" + carolID,
			"/ap/actors/" + remoteID,
			"/ap/actors/0192F3A4-7B1C-7D2E-9F00-1A2B3C4D5E6F", // not the canonical form
			"/ap/actors/not-a-uuid",
		},
		"collection": {
			"/ap/actors/" + missingID + "/outbox",
			"/ap/actors/" + carolID + "/outbox",
			"/ap/actors/" + daveID + "/outbox",
			"/ap/actors/" + remoteID + "/outbox",
			"/ap/actors/" + aliceID + "/inbox",
			"/ap/actors/" + aliceID + "/likes",
			"/ap/actors/" + instanceID + "/featured",
		},
		"profile page": {
			"/@nobody",
			"/@carol",
			"/@erin",
			"/@" + accountDomain,
			"/@bad-name",
		},
		"webfinger": {
			"/.well-known/webfinger?resource=acct:nobody@" + accountDomain,
			"/.well-known/webfinger?resource=acct:carol@" + accountDomain,
			"/.well-known/webfinger?resource=acct:erin@remote.example",
			"/.well-known/webfinger?resource=acct:alice@remote.example",
			"/.well-known/webfinger?resource=https://" + host + "/ap/actors/" + carolID,
			"/.well-known/webfinger?resource=https://remote.example/users/erin",
			"/.well-known/webfinger?resource=acct:nobody",
		},
	}
	for name, targets := range groups {
		t.Run(name, func(t *testing.T) {
			first := s.get(t, targets[0], acceptMastodon)
			if first.Code != http.StatusNotFound || first.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("GET %s = %d Cache-Control %q, want 404 no-store", targets[0], first.Code, first.Header().Get("Cache-Control"))
			}
			for _, target := range targets[1:] {
				rec := s.get(t, target, acceptMastodon)
				if rec.Code != first.Code || !reflect.DeepEqual(rec.Header(), first.Header()) || rec.Body.String() != first.Body.String() {
					t.Errorf("GET %s = %d %v %q\nwant the response of GET %s: %d %v %q",
						target, rec.Code, rec.Header(), rec.Body, targets[0], first.Code, first.Header(), first.Body)
				}
			}
		})
	}
}

func TestLookupErrorsAre500(t *testing.T) {
	s := newServer(t, accountDomain)
	s.accounts.err = errors.New("database is down")
	for _, target := range []string{
		"/ap/actors/" + aliceID,
		"/.well-known/webfinger?resource=acct:alice@" + accountDomain,
		"/nodeinfo/2.1",
	} {
		if rec := s.get(t, target, acceptMastodon); rec.Code != http.StatusInternalServerError {
			t.Errorf("GET %s = %d, want 500", target, rec.Code)
		}
	}
}

// TestIDsDoNotDependOnUsernames checks that ids are UUID-based (ADR-0018): a username appears
// only in the profile URL.
func TestIDsDoNotDependOnUsernames(t *testing.T) {
	s := newServer(t, accountDomain)
	doc := decode(t, s.get(t, "/ap/actors/"+aliceID, acceptMastodon).Body.Bytes())
	id := "https://" + host + "/ap/actors/" + aliceID
	for key, want := range map[string]string{
		"id": id, "inbox": id + "/inbox", "outbox": id + "/outbox", "followers": id + "/followers",
		"following": id + "/following", "featured": id + "/featured", "featuredTags": id + "/featured-tags",
	} {
		if doc[key] != want {
			t.Errorf("%s = %v, want %s", key, doc[key], want)
		}
	}
	if got := doc["publicKey"].(map[string]any)["id"]; got != account.KeyIRI(id, account.KeyRSASHA256) {
		t.Errorf("publicKey.id = %v, want %s#main-key", got, id)
	}
}

// TestWebFingerCanonicalDomain checks that every spelling of the configured domains answers, on a
// server with internationalized domains and on one whose host has a port (federation.md §1.2).
func TestWebFingerCanonicalDomain(t *testing.T) {
	const (
		aLabel = "xn--r8jz45g.jp"
		uLabel = "例え.jp"
		wide   = "ＸＮ－－ｒ８ｊｚ４５ｇ．ｊｐ"
	)
	serve := func(t *testing.T, serving, domain string) *server {
		t.Helper()
		s := &server{mux: http.NewServeMux(), accounts: newAccounts(t), objects: newObjects(), graph: &fakeGraph{}}
		activitypub.New(activitypub.Options{
			Host: serving, AccountDomain: domain, SoftwareVersion: "0.1.0", SourceURL: "https://github.com/mycroblog/my",
			Accounts: s.accounts, Objects: s.objects, Graph: s.graph, Problem: problem,
		}).Register(s.mux)
		return s
	}
	finger := func(s *server, resource string) *httptest.ResponseRecorder {
		t.Helper()
		return s.get(t, "/.well-known/webfinger?resource="+url.QueryEscape(resource), "")
	}

	s := serve(t, "social."+aLabel, aLabel)
	for _, resource := range []string{
		"acct:alice@" + aLabel,
		"acct:alice@" + uLabel,
		"acct:alice@" + aLabel + ".",
		"acct:alice@" + uLabel + ".",
		"acct:alice@" + wide,
		"acct:alice@social." + uLabel,
		"alice@" + uLabel,
		"https://social." + uLabel + "/@alice",
		"https://social." + uLabel + "/ap/actors/" + aliceID,
	} {
		rec := finger(s, resource)
		if rec.Code != http.StatusOK {
			t.Errorf("resource %s: status %d, want 200", resource, rec.Code)
			continue
		}
		if got := decode(t, rec.Body.Bytes())["subject"]; got != "acct:alice@"+aLabel {
			t.Errorf("resource %s: subject = %v", resource, got)
		}
	}
	for _, resource := range []string{"acct:" + aLabel + "@" + aLabel, "acct:" + uLabel + "@" + uLabel, "acct:" + aLabel + ".@" + uLabel} {
		rec := finger(s, resource)
		if rec.Code != http.StatusOK || decode(t, rec.Body.Bytes())["subject"] != "acct:"+aLabel+"@"+aLabel {
			t.Errorf("instance actor resource %s = %d %s", resource, rec.Code, rec.Body)
		}
	}
	for _, resource := range []string{
		"acct:alice@other." + uLabel,
		"acct:alice@xn--",
		"acct:alice@" + aLabel + "..",
		"acct:alice@",
		"https://social." + uLabel + "/@alice?x=1",
		"https://social." + uLabel + "/@alice#x",
		"https://other." + uLabel + "/@alice",
	} {
		if rec := finger(s, resource); rec.Code != http.StatusNotFound {
			t.Errorf("resource %s: status %d, want 404", resource, rec.Code)
		}
	}

	dev := serve(t, "localhost:3000", "localhost:3000")
	for _, resource := range []string{"acct:alice@localhost:3000", "acct:alice@LOCALHOST:3000"} {
		if rec := finger(dev, resource); rec.Code != http.StatusOK {
			t.Errorf("resource %s: status %d, want 200", resource, rec.Code)
		}
	}
	if rec := finger(dev, "acct:alice@localhost:4000"); rec.Code != http.StatusNotFound {
		t.Errorf("another port: status %d, want 404", rec.Code)
	}
}
