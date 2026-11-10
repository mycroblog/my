// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestClientAPILookupStatus checks lookupStatus, which the page of the Create intents of WebFinger
// (FEP-3b86) reads: a remote post is fetched once and found again without a fetch, a local post
// is found by its id, and an actor, an unknown URL, and a post the viewer may not see are 404s.
func TestClientAPILookupStatus(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network}, "alice", "bob")
	const carol = "https://remote.example/users/carol"
	newRemoteActorWith(t, network, carol, "Person", func(doc map[string]any) {
		doc["preferredUsername"] = "carol"
		doc["inbox"] = carol + "/inbox"
	})
	network.serve(t, "https://remote.example/.well-known/webfinger?resource="+url.QueryEscape("acct:carol@remote.example"), map[string]any{
		"subject": "acct:carol@remote.example",
		"links":   []map[string]string{{"rel": "self", "type": "application/activity+json", "href": carol}},
	})
	note := carol + "/statuses/1"
	network.serve(t, note, map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       note, "type": "Note", "attributedTo": carol, "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{carol + "/followers"},
		"content": "<p>hello from carol</p>",
	})
	lookup := func(user, uri string, out *clientapi.Status) *http.Response {
		t.Helper()
		return c.do(user, http.MethodGet, "/statuses/lookup?uri="+url.QueryEscape(uri), nil, out)
	}

	var got clientapi.Status
	if res := lookup("alice", note, &got); res.StatusCode != http.StatusOK {
		t.Fatalf("lookup of the remote post = %d", res.StatusCode)
	}
	if got.Uri != note || got.Account.Acct != "carol@remote.example" {
		t.Errorf("lookup of the remote post = %s by %s", got.Uri, got.Account.Acct)
	}
	// The stored post is found without fetching it again.
	network.mu.Lock()
	delete(network.docs, note)
	network.mu.Unlock()
	var again clientapi.Status
	if res := lookup("alice", note, &again); res.StatusCode != http.StatusOK || again.Id != got.Id {
		t.Errorf("lookup of the stored post = %d %s, want %s", res.StatusCode, again.Id, got.Id)
	}

	own := c.post("bob", map[string]any{"text": "public", "visibility": "public"})
	var local clientapi.Status
	if res := lookup("alice", own.Uri, &local); res.StatusCode != http.StatusOK || local.Id != own.Id {
		t.Errorf("lookup of a local post = %d %s, want %s", res.StatusCode, local.Id, own.Id)
	}
	hidden := c.post("bob", map[string]any{"text": "direct", "visibility": "direct"})
	for name, uri := range map[string]string{
		"an actor":            carol,
		"an unknown URL":      "https://remote.example/users/carol/statuses/2",
		"a direct post":       hidden.Uri,
		"a page of this host": "https://example.test/@bob",
	} {
		if res := lookup("alice", uri, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("lookup of %s = %d, want 404", name, res.StatusCode)
		}
	}
	if res := lookup("", note, nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anonymous lookup = %d, want 401", res.StatusCode)
	}
}
