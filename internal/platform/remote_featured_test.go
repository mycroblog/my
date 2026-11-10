// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/platform"
)

// TestRemoteActorDropsFeaturedCollections follows a remote actor that declares its pinned posts
// (featured) and its featured hashtags (featuredTags), then stops declaring each: the fetch of
// the actor mirrors both, a fetch again of a document without featured clears the pins, and an
// Update without featuredTags clears the featured hashtags, each at once in the transaction that
// stores the document.
func TestRemoteActorDropsFeaturedCollections(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network}, "alice")
	ctx := t.Context()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	bob := actorDoc{id: "https://remote.example/users/bob", name: "Bob", summary: "<p>hello</p>", key: key}
	featured, featuredTags := bob.id+"/collections/featured", bob.id+"/collections/tags"
	network.serve(t, featured, map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": featured, "type": "OrderedCollection", "totalItems": 1,
		"orderedItems": []any{map[string]any{
			"id": bob.id + "/statuses/1", "type": "Note", "attributedTo": bob.id, "content": "<p>pinned</p>",
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{bob.id + "/followers"},
			"published": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		}},
	})
	network.serve(t, featuredTags, map[string]any{
		"@context": []any{"https://www.w3.org/ns/activitystreams", map[string]any{"Hashtag": "as:Hashtag"}},
		"id":       featuredTags, "type": "Collection", "totalItems": 1,
		"items": []any{map[string]any{"type": "Hashtag", "name": "#cats", "href": "https://remote.example/tags/cats"}},
	})
	// doc is bob's document, declaring the collections that are set.
	doc := func(withFeatured, withTags bool) map[string]any {
		d := bob.json(t)
		if withFeatured {
			d["featured"] = featured
		}
		if withTags {
			d["featuredTags"] = featuredTags
		}
		return d
	}
	refresh := func(d map[string]any) string {
		t.Helper()
		network.serve(t, bob.id, d)
		a, err := c.app.RemoteActors.Refresh(ctx, bob.id)
		if err != nil {
			t.Fatalf("Refresh(%s) = %v", bob.id, err)
		}
		return a.Account.ID
	}
	pins := func(accountID string) int {
		t.Helper()
		views, err := c.app.Statuses.Pinned(ctx, appcontent.Viewer{Kind: appcontent.ViewerAnonymous}, accountID)
		if err != nil {
			t.Fatalf("Pinned(%s) = %v", accountID, err)
		}
		return len(views)
	}
	tags := func(accountID string) int {
		t.Helper()
		got, err := c.app.Timeline.FeaturedTags(ctx, accountID)
		if err != nil {
			t.Fatalf("FeaturedTags(%s) = %v", accountID, err)
		}
		return len(got)
	}

	id := refresh(doc(true, true))
	if got, want := pins(id), 1; got != want {
		t.Fatalf("pins of an actor that declares featured = %d, want %d", got, want)
	}
	if got, want := tags(id), 1; got != want {
		t.Fatalf("featured hashtags of an actor that declares featuredTags = %d, want %d", got, want)
	}

	// A fetch again of a document without featured clears the pins and keeps the hashtags. The
	// collection still answers, so only the document says the pins are gone.
	refresh(doc(false, true))
	if got := pins(id); got != 0 {
		t.Errorf("pins after the document dropped featured = %d, want 0", got)
	}
	if got, want := tags(id), 1; got != want {
		t.Errorf("featured hashtags after the document dropped featured = %d, want %d", got, want)
	}
	refresh(doc(false, true)) // clearing again changes nothing
	if got := pins(id); got != 0 {
		t.Errorf("pins after the same document again = %d, want 0", got)
	}

	// The actor's own Update without featuredTags clears its featured hashtags.
	code := inboxCode(t, c, remoteActor{id: bob.id, key: bob.key}, map[string]any{
		"id": bob.id + "#update-1", "type": "Update", "actor": bob.id, "object": doc(false, false),
	})
	if code != http.StatusAccepted {
		t.Fatalf("Update = %d", code)
	}
	if got := tags(id); got != 0 {
		t.Errorf("featured hashtags after the Update dropped featuredTags = %d, want 0", got)
	}

	// Declaring them again mirrors them again.
	refresh(doc(true, true))
	if pins(id) != 1 || tags(id) != 1 {
		t.Errorf("pins and featured hashtags after declaring them again = %d, %d; want 1, 1", pins(id), tags(id))
	}
}
