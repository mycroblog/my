// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	domcontent "github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/platform"
	"github.com/mycroblog/my/pkg/testkit"
)

// collectionScene builds alice with n followers and n statuses, among them posts with custom
// emoji, quotes of her own posts, and posts whose quote policy she set, and counts the database
// round trips of one GET of a page of her followers, of her outbox, and of her featured
// collection, with her five newest statuses pinned. Every query is one round trip (one call in
// pg_stat_statements).
func collectionScene(t *testing.T, n int) (map[string]int, *statusesClient) {
	t.Helper()
	tracer := testkit.NewTracer()
	users := []string{"alice"}
	for i := range n {
		users = append(users, fmt.Sprintf("f%d", i))
	}
	c := newStatusesClientWith(t, nil, platform.AppOptions{Tracer: tracer}, users...)
	ctx := t.Context()
	for _, shortcode := range []string{"blobcat", "blobfox"} {
		m, err := c.app.Media.Upload(ctx, appcontent.UploadInput{OwnerAccountID: c.ids["alice"], Content: bytes.NewReader(testkit.FixturePNG(32, 32, true)), Purpose: domcontent.PurposeEmoji})
		if err != nil {
			t.Fatal(err)
		}
		if res := c.app.Media.Process(ctx, m.File.ID, domcontent.PurposeEmoji); !res.IsDone() {
			t.Fatalf("Process = %+v", res)
		}
		if res := c.do("alice", http.MethodPost, "/admin/custom_emojis", map[string]any{"shortcode": shortcode, "media_id": m.File.ID}, nil); res.StatusCode != http.StatusCreated {
			t.Fatalf("POST /admin/custom_emojis = %d", res.StatusCode)
		}
	}
	for _, f := range users[1:] {
		if res := c.do(f, http.MethodPost, "/accounts/"+c.ids["alice"]+"/follow", nil, nil); res.StatusCode/100 != 2 {
			t.Fatalf("%s follows alice = %d", f, res.StatusCode)
		}
	}
	var posts []string
	for i := range n {
		body := map[string]any{"text": fmt.Sprintf("post %d", i), "visibility": "public"}
		switch i % 3 {
		case 0:
			body["text"] = fmt.Sprintf("post %d :blobcat:", i)
		case 1:
			body["text"] = fmt.Sprintf("post %d :blobfox: :blobcat:", i)
		}
		if i%4 == 2 && len(posts) > 0 {
			body["quoted_status_id"] = posts[len(posts)-1]
		}
		if i%5 == 3 {
			body["interaction_policy"] = map[string]any{"quote": map[string]any{"automatic": "followers"}}
		}
		posts = append(posts, c.post("alice", body).Id)
	}
	for _, id := range posts[len(posts)-min(5, len(posts)):] {
		if res := c.do("alice", http.MethodPost, "/statuses/"+id+"/pin", nil, nil); res.StatusCode/100 != 2 {
			t.Fatalf("pin %s = %d", id, res.StatusCode)
		}
	}
	queries := func() int {
		count := 0
		for _, sp := range tracer.Spans() {
			if sp.Attrs["db.operation.name"] != "" {
				count++
			}
		}
		return count
	}
	out := map[string]int{}
	actor := "/ap/actors/" + c.ids["alice"]
	for name, want := range map[string]int{"followers?page=true": n, "outbox?page=true": min(n, 20), "featured": min(n, 5)} {
		before := queries()
		code, doc := apGet(t, c, actor+"/"+name)
		out[name] = queries() - before
		items, _ := doc["orderedItems"].([]any)
		if code != http.StatusOK || len(items) != want {
			t.Fatalf("GET %s = %d with %d items, want %d", name, code, len(items), want)
		}
	}
	return out, c
}

// TestCollectionQueries checks that the round trips of a page of the followers, outbox, and
// featured collections do not grow with the items on it.
func TestCollectionQueries(t *testing.T) {
	t.Parallel()
	small, _ := collectionScene(t, 6)
	large, _ := collectionScene(t, 40)
	for name, n := range small {
		t.Logf("%s: %d queries for the small page, %d for the large", name, n, large[name])
		if large[name] > n {
			t.Errorf("%s: %d queries for the large page, more than the %d for the small", name, large[name], n)
		}
	}
}

// TestOutboxNotesAreThoseOfOneStatus checks that the outbox and the featured collection, which
// read the parts of their Notes together, present each Note exactly as the GET of the status
// presents it alone: its custom emoji, its quote, and who may quote it.
func TestOutboxNotesAreThoseOfOneStatus(t *testing.T) {
	t.Parallel()
	_, c := collectionScene(t, 12)
	actor := "/ap/actors/" + c.ids["alice"]
	_, outbox := apGet(t, c, actor+"/outbox?page=true")
	_, featured := apGet(t, c, actor+"/featured")
	var notes []map[string]any
	for _, it := range outbox["orderedItems"].([]any) {
		notes = append(notes, it.(map[string]any)["object"].(map[string]any))
	}
	for _, it := range featured["orderedItems"].([]any) {
		notes = append(notes, it.(map[string]any))
	}
	quoting, emoji := 0, 0
	for _, note := range notes {
		id := note["id"].(string)
		code, one := apGet(t, c, id[len("https://"+c.app.Config.Host):])
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d", id, code)
		}
		delete(one, "@context")
		if !reflect.DeepEqual(note, one) {
			t.Errorf("the collection's Note of %s differs from its GET:\ncollection %v\none        %v", id, note, one)
		}
		if note["quote"] != nil {
			quoting++
		}
		if tags, _ := note["tag"].([]any); len(tags) > 0 {
			emoji++
		}
	}
	if quoting == 0 || emoji == 0 {
		t.Errorf("the Notes have %d quotes and %d with tags, want some of each", quoting, emoji)
	}
}
