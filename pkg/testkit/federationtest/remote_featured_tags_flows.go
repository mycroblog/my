// SPDX-License-Identifier: AGPL-3.0-only

package federationtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The cases of RemoteStatusFlows that mirror the featuredTags collections (the featured
// hashtags) of remote accounts when the accounts are fetched or refreshed and when they send an
// Add or Remove to their featured collection (federation.md §6.2, ADR-0006 §17):
// federation.fetch_featured_tags through the real RemoteActors and Fetcher over an in-memory
// network. The featured hashtags themselves are the timeline module's (timelinetest), so a
// recorder stands in for it.

// remoteFeaturedTags records the names that ReplaceRemoteFeaturedTags receives, by account; it
// implements appfederation.RemoteFeaturedTags.
type remoteFeaturedTags struct {
	mu        sync.Mutex
	byAccount map[string][]string
	calls     int
}

func (r *remoteFeaturedTags) ReplaceRemoteFeaturedTags(_ context.Context, accountID string, names []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.byAccount[accountID] = slices.Clone(names[:min(len(names), appfederation.FeaturedTagsMaxTags)])
	return nil
}

func (r *remoteFeaturedTags) of(accountID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byAccount[accountID]
}

// featuredTagsActor serves the actor uri with the featured collection featured and the
// featuredTags collection tags ("" for none of either), fetches it as RemoteActors.Refresh does,
// and returns its account ID. The fetch must enqueue one fetch of the featuredTags collection
// when it declares one of the actor's origin, and none otherwise.
func (fe *remoteFetchEnv) featuredTagsActor(t *testing.T, uri, featured, tags string) string {
	t.Helper()
	doc := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": uri, "type": "Person",
		"preferredUsername": uri[strings.LastIndexByte(uri, '/')+1:], "inbox": uri + "/inbox", "outbox": uri + "/outbox",
		"followers": uri + "/followers",
	}
	if featured != "" {
		doc["featured"] = featured
	}
	if tags != "" {
		doc["featuredTags"] = tags
	}
	fe.serve(t, uri, doc)
	a, err := fe.remoteActors.Refresh(t.Context(), uri)
	if err != nil {
		t.Fatalf("Refresh(%s): %v", uri, err)
	}
	fe.jobs.take(appfederation.JobFetchFeatured)
	jobs := fe.jobs.take(appfederation.JobFetchFeaturedTags)
	want := 0
	if tags != "" && federation.SameOrigin(federation.IRI(tags), federation.IRI(uri)) {
		want = 1
	}
	if len(jobs) != want {
		t.Fatalf("Refresh(%s) enqueued %d fetches of the featuredTags collection, want %d", uri, len(jobs), want)
	}
	if err := fe.jobs.Enqueue(t.Context(), jobs...); err != nil {
		t.Fatal(err)
	}
	return a.Account.ID
}

// runFeaturedTags runs the pending fetches of featuredTags collections and returns how many ran.
// Every one must complete.
func (fe *remoteFetchEnv) runFeaturedTags(t *testing.T) int {
	t.Helper()
	h, _, ok := fe.handlers.Lookup(appfederation.JobFetchFeaturedTags)
	if !ok {
		t.Fatalf("no handler for %s", appfederation.JobFetchFeaturedTags)
	}
	jobs := fe.jobs.take(appfederation.JobFetchFeaturedTags)
	for _, j := range jobs {
		if res := h(t.Context(), queue.Job{Type: j.Type, Payload: j.Payload, IdempotencyKey: j.IdempotencyKey, Attempt: 1}); !res.IsDone() {
			t.Fatalf("%s %s: %v", j.Type, j.Payload, res.Err())
		}
	}
	return len(jobs)
}

func remoteFeaturedTagsFixture(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const actor = "https://mastodon.example/ap/users/117454922805771612"
	tags := actor + "/collections/tags"
	fe.serveFixture(t, tags, "mastodon", "4.7", "featured-tags")
	account := fe.featuredTagsActor(t, actor, "", tags)
	if n := fe.runFeaturedTags(t); n != 1 {
		t.Fatalf("featuredTags fetches = %d, want 1", n)
	}
	if got := fe.featuredTags.of(account); !slices.Equal(got, []string{"cats", "Photography"}) {
		t.Errorf("featured hashtags = %v, want cats and Photography", got)
	}
}

func remoteMirrorsFeaturedTagsWithinBounds(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const bob, carol = "https://other.example/users/bob", "https://third.example/users/carol"
	tags := bob + "/collections/tags"
	hashtag := func(name string) map[string]any {
		return map[string]any{"type": "Hashtag", "href": bob + "/tagged/" + name, "name": "#" + name}
	}
	// The collection lists its Hashtags in a first page, more than are kept.
	var items []any
	for n := range appfederation.FeaturedTagsMaxTags + 3 {
		items = append(items, hashtag(fmt.Sprintf("tag%d", n)))
	}
	page := tags + "?page=1"
	fe.serve(t, tags, map[string]any{"id": tags, "type": "Collection", "totalItems": len(items), "first": page})
	fe.serve(t, page, map[string]any{"id": page, "type": "CollectionPage", "partOf": tags, "items": items})
	// A collection of another origin is not fetched.
	fe.featuredTagsActor(t, carol, "", bob+"/collections/carols")
	if n := fe.runFeaturedTags(t); n != 0 {
		t.Errorf("fetches of a collection of another origin = %d, want 0", n)
	}
	account := fe.featuredTagsActor(t, bob, "", tags)
	before := fe.requests("https://")
	fe.runFeaturedTags(t)
	if n := fe.requests("https://") - before; n > appfederation.FeaturedTagsMaxDocuments {
		t.Errorf("the featuredTags fetch made %d requests, more than %d", n, appfederation.FeaturedTagsMaxDocuments)
	}
	if got := fe.featuredTags.of(account); len(got) != appfederation.FeaturedTagsMaxTags || got[0] != "tag0" {
		t.Errorf("featured hashtags = %v, want the first %d", got, appfederation.FeaturedTagsMaxTags)
	}

	// A refresh makes them those the collection lists then.
	fe.serve(t, tags, map[string]any{"id": tags, "type": "Collection", "totalItems": 2, "items": []any{hashtag("dogs"), hashtag("cats")}})
	fe.featuredTagsActor(t, bob, "", tags)
	fe.runFeaturedTags(t)
	if got := fe.featuredTags.of(account); !slices.Equal(got, []string{"dogs", "cats"}) {
		t.Errorf("featured hashtags after a refresh = %v, want dogs, cats", got)
	}

	// A collection that cannot be fetched leaves them as they are.
	fe.net.set(tags, remoteResponse{status: http.StatusServiceUnavailable})
	fe.Clock.Advance(time.Hour)
	fe.featuredTagsActor(t, bob, "", tags)
	calls := fe.featuredTags.calls
	fe.runFeaturedTags(t)
	if fe.featuredTags.calls != calls || len(fe.featuredTags.of(account)) != 2 {
		t.Errorf("featured hashtags after a failed fetch = %v, want the 2 kept", fe.featuredTags.of(account))
	}

	// An empty collection, and an actor that no longer declares one, remove them.
	fe.serve(t, tags, map[string]any{"id": tags, "type": "Collection", "totalItems": 0, "items": []any{}})
	fe.Clock.Advance(time.Hour)
	fe.featuredTagsActor(t, bob, "", tags)
	fe.runFeaturedTags(t)
	if got := fe.featuredTags.of(account); len(got) != 0 {
		t.Errorf("featured hashtags after an empty collection = %v, want none", got)
	}
	fe.serve(t, tags, map[string]any{"id": tags, "type": "Collection", "totalItems": 1, "items": []any{hashtag("cats")}})
	fe.Clock.Advance(time.Hour)
	fe.featuredTagsActor(t, bob, "", tags)
	fe.runFeaturedTags(t)
	if got := fe.featuredTags.of(account); len(got) != 1 {
		t.Fatalf("featured hashtags = %v, want 1", got)
	}
	fe.Clock.Advance(time.Hour)
	fe.featuredTagsActor(t, bob, "", "")
	if h, _, _ := fe.handlers.Lookup(appfederation.JobFetchFeaturedTags); h != nil {
		// The job of an account without a collection, as one enqueued before the refresh runs.
		if res := h(t.Context(), queue.Job{Type: appfederation.JobFetchFeaturedTags, Payload: []byte(`{"account_id":"` + account + `"}`), Attempt: 1}); !res.IsDone() {
			t.Fatalf("%s: %v", appfederation.JobFetchFeaturedTags, res.Err())
		}
	}
	if got := fe.featuredTags.of(account); len(got) != 0 {
		t.Errorf("featured hashtags of an actor without a featuredTags collection = %v, want none", got)
	}
}

func remoteFeaturedTagsChanges(t *testing.T, e *remoteStatusEnv) {
	fe := newRemoteFetchEnv(t, e)
	const bob = "https://other.example/users/bob"
	featured, tags := bob+"/collections/featured", bob+"/collections/tags"
	fe.serve(t, featured, map[string]any{"id": featured, "type": "OrderedCollection", "totalItems": 0, "orderedItems": []any{}})
	collection := func(names ...string) {
		items := []any{}
		for _, n := range names {
			items = append(items, map[string]any{"type": "Hashtag", "href": bob + "/tagged/" + n, "name": "#" + n})
		}
		fe.serve(t, tags, map[string]any{"id": tags, "type": "Collection", "totalItems": len(items), "items": items})
	}
	collection("cats")
	account := fe.featuredTagsActor(t, bob, featured, tags)
	fe.runFeaturedTags(t)
	if got := fe.featuredTags.of(account); !slices.Equal(got, []string{"cats"}) {
		t.Fatalf("featured hashtags = %v, want cats", got)
	}

	// Mastodon sends a new featured hashtag as the Add of a Hashtag to the featured collection.
	collection("cats", "dogs")
	add, err := json.Marshal(map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "type": "Add", "actor": bob,
		"object": map[string]any{"type": "Hashtag", "href": bob + "/tagged/dogs", "name": "#dogs"}, "target": featured,
	})
	if err != nil {
		t.Fatal(err)
	}
	fe.expect(t, add, appfederation.InboxHandled)
	if n := fe.runFeaturedTags(t); n != 1 {
		t.Fatalf("featuredTags fetches after an Add = %d, want 1", n)
	}
	if got := fe.featuredTags.of(account); !slices.Equal(got, []string{"cats", "dogs"}) {
		t.Errorf("featured hashtags after an Add = %v, want cats, dogs", got)
	}
	// The Remove of a hashtag, and an Add to another collection, which is dropped.
	collection("dogs")
	fe.expect(t, featuredChange(t, "Remove", bob, bob+"/tagged/cats", featured), appfederation.InboxHandled)
	fe.expect(t, featuredChange(t, "Add", bob, bob+"/tagged/cats", tags), appfederation.InboxDropped)
	if n := fe.runFeaturedTags(t); n != 1 {
		t.Fatalf("featuredTags fetches after a Remove = %d, want 1", n)
	}
	if got := fe.featuredTags.of(account); !slices.Equal(got, []string{"dogs"}) {
		t.Errorf("featured hashtags after a Remove = %v, want dogs", got)
	}
}
