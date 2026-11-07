// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	appfederation "github.com/mycroblog/my/internal/app/federation"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/domain/federation"
)

// The cases of RemoteStatusFlows that mirror the featured collections (the pinned posts) of
// remote accounts when the accounts are fetched or refreshed (federation.md §6.2, ADR-0006):
// federation.fetch_featured through the real RemoteActors and Fetcher over an in-memory network.

// featuredActor serves the actor uri with the featured collection featured ("" for none),
// fetches it as RemoteActors.Refresh does, and returns its account ID. The fetch must enqueue
// one fetch of the featured collection when it declares one, and none otherwise.
func (fe *remoteFetchEnv) featuredActor(t *testing.T, uri, featured string) string {
	t.Helper()
	doc := map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "id": uri, "type": "Person",
		"preferredUsername": uri[strings.LastIndexByte(uri, '/')+1:], "inbox": uri + "/inbox", "outbox": uri + "/outbox",
		"followers": uri + "/followers",
	}
	if featured != "" {
		doc["featured"] = featured
	}
	fe.serve(t, uri, doc)
	a, err := fe.remoteActors.Refresh(t.Context(), uri)
	if err != nil {
		t.Fatalf("Refresh(%s): %v", uri, err)
	}
	jobs := fe.jobs.take(appfederation.JobFetchFeatured)
	want := 0
	if featured != "" && federation.SameOrigin(federation.IRI(featured), federation.IRI(uri)) {
		want = 1
	}
	if len(jobs) != want {
		t.Fatalf("Refresh(%s) enqueued %d fetches of the featured collection, want %d", uri, len(jobs), want)
	}
	if err := fe.jobs.Enqueue(t.Context(), jobs...); err != nil {
		t.Fatal(err)
	}
	return a.Account.ID
}

// runFeatured runs the pending fetches of featured collections and returns how many ran. Every
// one must complete.
func (fe *remoteFetchEnv) runFeatured(t *testing.T) int {
	t.Helper()
	h, _, ok := fe.handlers.Lookup(appfederation.JobFetchFeatured)
	if !ok {
		t.Fatalf("no handler for %s", appfederation.JobFetchFeatured)
	}
	jobs := fe.jobs.take(appfederation.JobFetchFeatured)
	for _, j := range jobs {
		if res := h(t.Context(), queue.Job{Type: j.Type, Payload: j.Payload, IdempotencyKey: j.IdempotencyKey, Attempt: 1}); !res.IsDone() {
			t.Fatalf("%s %s: %v", j.Type, j.Payload, res.Err())
		}
	}
	return len(jobs)
}

// pins returns the IDs of the statuses that accountID pins, the newest pin first.
func (fe *remoteFetchEnv) pins(t *testing.T, accountID string) []string {
	t.Helper()
	pins, err := fe.h.Pins.ListPins(t.Context(), accountID)
	if err != nil {
		t.Fatalf("ListPins(%s): %v", accountID, err)
	}
	ids := make([]string, 0, len(pins))
	for _, p := range pins {
		ids = append(ids, p.StatusID)
	}
	return ids
}

// statusIDs returns the IDs of the stored posts uris.
func (fe *remoteFetchEnv) statusIDs(t *testing.T, uris ...string) []string {
	t.Helper()
	ids := make([]string, 0, len(uris))
	for _, uri := range uris {
		ids = append(ids, fe.post(t, uri).Status.ID)
	}
	return ids
}

func remotePinsFeaturedFixtures(t *testing.T, e *remoteStatusEnv) {
	if e.h.Pins == nil {
		t.Skip("the harness has no interaction repository")
	}
	fe := newRemoteFetchEnv(t, e)
	for _, tc := range []struct {
		implementation, version string
		actor                   string
		// pinned are the posts that the collection lists, the newest pin first; served are the
		// documents served at the IRIs of those that it does not embed.
		pinned, served []string
	}{
		{
			implementation: "mastodon", version: "4.7",
			actor: "https://mastodon.example/ap/users/117454922805771612",
			pinned: []string{
				"https://mastodon.example/ap/users/117454922805771612/statuses/117454925496168351",
				"https://mastodon.example/ap/users/117454922805771612/statuses/117454936765897454",
			},
		},
		{
			implementation: "misskey", version: "2026.10",
			actor:  "https://misskey.example/users/asfo7no84zv70002",
			pinned: []string{"https://misskey.example/notes/asfo829i4zv7000d", "https://misskey.example/notes/asfoesv84zv7001s"},
		},
		{
			implementation: "gotosocial", version: "0.22",
			actor: "https://gotosocial.example/users/admin",
			pinned: []string{
				"https://gotosocial.example/users/admin/statuses/01F8MHAAY43M6RJ473VQFCVH37",
				"https://gotosocial.example/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R",
			},
			served: []string{
				"https://gotosocial.example/users/admin/statuses/01F8MHAAY43M6RJ473VQFCVH37",
				"https://gotosocial.example/users/admin/statuses/01F8MH75CBF9JFX4ZAD54N0W0R",
			},
		},
	} {
		t.Run(tc.implementation+"/"+tc.version, func(t *testing.T) {
			featured := tc.actor + "/collections/featured"
			fe.serveFixture(t, featured, tc.implementation, tc.version, "featured")
			for i, uri := range tc.served {
				fe.serve(t, uri, remoteNote(uri, tc.actor, remoteNow.Add(-time.Duration(i+1)*time.Hour), nil))
			}
			account := fe.featuredActor(t, tc.actor, featured)
			if n := fe.runFeatured(t); n != 1 {
				t.Fatalf("featured fetches = %d, want 1", n)
			}
			want := fe.statusIDs(t, tc.pinned...)
			for i, uri := range tc.pinned {
				if p := fe.post(t, uri); p.Status.AuthorAccountID != account || p.Status.IsLocal {
					t.Errorf("%s = %+v, want a fetched post of the account", uri, p.Status)
				} else if p.Status.ID != want[i] {
					t.Errorf("%s has ID %s, want %s", uri, p.Status.ID, want[i])
				}
			}
			// The pins are in the collection's order, which is not that of the posts.
			if got := fe.pins(t, account); !slices.Equal(got, want) {
				t.Errorf("pins = %v, want %v", got, want)
			}
		})
	}
}

func remoteMirrorsFeaturedWithinBounds(t *testing.T, e *remoteStatusEnv) {
	if e.h.Pins == nil {
		t.Skip("the harness has no interaction repository")
	}
	fe := newRemoteFetchEnv(t, e)
	const bob, carol = "https://other.example/users/bob", "https://third.example/users/carol"
	featured := bob + "/collections/featured"
	note := func(n int) string { return fmt.Sprintf("%s/notes/%d", bob, n) }
	newest := remoteNow.Add(-time.Minute)
	items := []any{
		// An embedded post of someone else, an embedded followers-only post, and a post of
		// another origin are not pinned.
		remoteNote(bob+"/notes/carols", carol, newest, nil),
		remoteNote(bob+"/notes/private", bob, newest, map[string]any{"to": []string{bob + "/followers"}, "cc": []string{}}),
		carol + "/notes/1",
	}
	const posts = appfederation.FeaturedMaxPosts + 2
	for n := range posts {
		fe.serve(t, note(n), remoteNote(note(n), bob, remoteNow.Add(-time.Duration(n+1)*time.Hour), nil))
		items = append(items, note(n))
	}
	// The collection lists its items in a first page.
	page := featured + "?page=1"
	fe.serve(t, featured, map[string]any{"id": featured, "type": "OrderedCollection", "totalItems": len(items), "first": page})
	fe.serve(t, page, map[string]any{"id": page, "type": "OrderedCollectionPage", "partOf": featured, "orderedItems": items})
	fe.actor(t, carol)
	account := fe.featuredActor(t, bob, featured)
	// One of the pinned posts is stored already.
	if _, err := fe.resolver.Resolve(t.Context(), note(2)); err != nil {
		t.Fatalf("Resolve(%s): %v", note(2), err)
	}
	before := fe.requests("https://")
	fe.runFeatured(t)

	// The collection, its page, and the posts not stored.
	if n := fe.requests("https://") - before; n > appfederation.FeaturedMaxDocuments {
		t.Errorf("the featured fetch made %d requests, more than %d", n, appfederation.FeaturedMaxDocuments)
	}
	if fe.net.count(note(2)) != 1 {
		t.Error("the stored post was fetched again")
	}
	var pinned []string
	for n := range appfederation.FeaturedMaxPosts {
		pinned = append(pinned, note(n))
	}
	if got, want := fe.pins(t, account), fe.statusIDs(t, pinned...); !slices.Equal(got, want) {
		t.Errorf("pins = %v, want the first %d posts of bob %v", got, appfederation.FeaturedMaxPosts, want)
	}
	for _, uri := range []string{carol + "/notes/1", note(posts - 1)} {
		if fe.net.count(uri) != 0 {
			t.Errorf("%s was fetched", uri)
		}
	}
	for _, uri := range []string{bob + "/notes/carols", bob + "/notes/private"} {
		if fe.stored(t, uri) {
			t.Errorf("%s was stored", uri)
		}
	}

	// A refresh makes the pins those the collection lists then.
	fe.serve(t, featured, map[string]any{"id": featured, "type": "OrderedCollection", "totalItems": 2, "orderedItems": []any{note(posts - 1), note(0)}})
	fe.featuredActor(t, bob, featured)
	fe.runFeatured(t)
	if got, want := fe.pins(t, account), fe.statusIDs(t, note(posts-1), note(0)); !slices.Equal(got, want) {
		t.Errorf("pins after a refresh = %v, want %v", got, want)
	}

	// A collection that cannot be fetched leaves the pins as they are.
	fe.net.set(featured, remoteResponse{status: http.StatusServiceUnavailable})
	fe.clock.Advance(time.Hour)
	fe.featuredActor(t, bob, featured)
	fe.runFeatured(t)
	if got := fe.pins(t, account); len(got) != 2 {
		t.Errorf("pins after a failed fetch = %v, want the 2 pins kept", got)
	}

	// An empty collection, and an actor that no longer declares one, remove the pins.
	fe.serve(t, featured, map[string]any{"id": featured, "type": "OrderedCollection", "totalItems": 0, "orderedItems": []any{}})
	fe.clock.Advance(time.Hour)
	fe.featuredActor(t, bob, featured)
	fe.runFeatured(t)
	if got := fe.pins(t, account); len(got) != 0 {
		t.Errorf("pins after an empty collection = %v, want none", got)
	}
	fe.serve(t, featured, map[string]any{"id": featured, "type": "OrderedCollection", "totalItems": 1, "orderedItems": []any{note(0)}})
	fe.clock.Advance(time.Hour)
	fe.featuredActor(t, bob, featured)
	fe.runFeatured(t)
	if got := fe.pins(t, account); len(got) != 1 {
		t.Fatalf("pins = %v, want 1", got)
	}
	fe.featuredActor(t, bob, "")
	if h, _, _ := fe.handlers.Lookup(appfederation.JobFetchFeatured); h != nil {
		// The job of an account without a collection, as one enqueued before the refresh runs.
		if res := h(t.Context(), queue.Job{Type: appfederation.JobFetchFeatured, Payload: []byte(`{"account_id":"` + account + `"}`), Attempt: 1}); !res.IsDone() {
			t.Fatalf("%s: %v", appfederation.JobFetchFeatured, res.Err())
		}
	}
	if got := fe.pins(t, account); len(got) != 0 {
		t.Errorf("pins of an actor without a featured collection = %v, want none", got)
	}
}

// featuredChange returns an Add or Remove (typ) of the post object to the collection target by
// actor, without an id as Mastodon sends it.
func featuredChange(t *testing.T, typ, actor, object, target string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams", "type": typ, "actor": actor, "object": object, "target": target,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func remoteAppliesFeaturedChanges(t *testing.T, e *remoteStatusEnv) {
	if e.h.Pins == nil {
		t.Skip("the harness has no interaction repository")
	}
	fe := newRemoteFetchEnv(t, e)
	const bob, carol = "https://other.example/users/bob", "https://third.example/users/carol"
	featured := bob + "/collections/featured"
	note := func(n int) string { return fmt.Sprintf("%s/notes/%d", bob, n) }
	for n := range 3 {
		fe.serve(t, note(n), remoteNote(note(n), bob, remoteNow.Add(-time.Duration(n+1)*time.Hour), nil))
	}
	collection := func(notes ...string) {
		fe.serve(t, featured, map[string]any{"id": featured, "type": "OrderedCollection", "totalItems": len(notes), "orderedItems": notes})
	}
	collection(note(0))
	fe.actor(t, carol)
	account := fe.featuredActor(t, bob, featured)
	fe.runFeatured(t)
	if got, want := fe.pins(t, account), fe.statusIDs(t, note(0)); !slices.Equal(got, want) {
		t.Fatalf("pins = %v, want %v", got, want)
	}

	// An Add to the featured collection of its actor fetches the collection again.
	collection(note(1), note(0))
	fe.expect(t, featuredChange(t, "Add", bob, note(1), featured), appfederation.InboxHandled)
	if n := fe.runFeatured(t); n != 1 {
		t.Fatalf("featured fetches after an Add = %d, want 1", n)
	}
	if got, want := fe.pins(t, account), fe.statusIDs(t, note(1), note(0)); !slices.Equal(got, want) {
		t.Errorf("pins after an Add = %v, want %v", got, want)
	}

	// Removes in a row enqueue one fetch while it is pending.
	collection()
	fe.expect(t, featuredChange(t, "Remove", bob, note(1), featured), appfederation.InboxHandled)
	fe.expect(t, featuredChange(t, "Remove", bob, note(0), featured), appfederation.InboxHandled)
	if n := fe.runFeatured(t); n != 1 {
		t.Fatalf("featured fetches after two Removes = %d, want 1", n)
	}
	if got := fe.pins(t, account); len(got) != 0 {
		t.Errorf("pins after the Removes = %v, want none", got)
	}

	// An Add by someone else than the owner, to another collection of the actor, or to a
	// collection of another origin is dropped.
	collection(note(2))
	for _, doc := range [][]byte{
		featuredChange(t, "Add", carol, note(2), featured),
		featuredChange(t, "Add", bob, note(2), bob+"/collections/other"),
		featuredChange(t, "Add", bob, note(2), carol+"/collections/featured"),
		featuredChange(t, "Remove", bob, note(2), ""),
	} {
		fe.expect(t, doc, appfederation.InboxDropped)
	}
	if n := fe.runFeatured(t); n != 0 {
		t.Errorf("featured fetches after the Adds of others = %d, want 0", n)
	}
	if got := fe.pins(t, account); len(got) != 0 {
		t.Errorf("pins after the Adds of others = %v, want none", got)
	}
}
