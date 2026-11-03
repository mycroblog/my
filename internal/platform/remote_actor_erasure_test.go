// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/queue"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// laterClock is a steady clock that can be moved ahead.
type laterClock struct {
	steadyClock
	ahead *atomic.Int64
}

func (c laterClock) Now() time.Time { return c.steadyClock.Now().Add(time.Duration(c.ahead.Load())) }

// TestRemoteActorDeleteIsErased runs a remote actor's Delete and the erasure through the whole
// server: every projector and module on the path, on the transaction's own connection (the
// package's check of second connections). Its post goes although a local user bookmarked it and
// replied to it, the reply stays, and no Delete goes out for what the actor's server deleted.
func TestRemoteActorDeleteIsErased(t *testing.T) {
	t.Parallel()
	network := &relayNet{docs: map[string][]byte{}}
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network, Clock: clock}, "alice")
	ctx := t.Context()
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	aliceIRI := "https://example.test/ap/actors/" + c.ids["alice"]
	note := carol.id + "/statuses/1"
	drain := func() {
		t.Helper()
		if _, err := c.app.Runner.Drain(ctx); err != nil {
			t.Fatalf("Drain: %v", err)
		}
	}

	// Carol follows alice, and mentions her in a post that alice bookmarks and replies to.
	postInbox(t, c, carol, map[string]any{"id": carol.id + "/follows/1", "type": "Follow", "actor": carol.id, "object": aliceIRI})
	postInbox(t, c, carol, map[string]any{
		"id": note + "/activity", "type": "Create", "actor": carol.id,
		"object": map[string]any{
			"id": note, "type": "Note", "attributedTo": carol.id, "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{aliceIRI},
			"content": "<p>hi alice</p>",
			"tag":     []any{map[string]any{"type": "Mention", "href": aliceIRI}},
			"attachment": []any{map[string]any{
				"type": "Document", "mediaType": "image/png", "url": "https://remote.example/files/carol.png", "name": "carol at home",
			}},
		},
	})
	drain()
	var notes struct {
		Items []struct {
			Status *clientapi.Status `json:"status"`
		} `json:"items"`
	}
	if res := c.do("alice", http.MethodGet, "/notifications", nil, &notes); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /notifications = %d", res.StatusCode)
	}
	var noteID string
	for _, n := range notes.Items {
		if n.Status != nil && strings.Contains(n.Status.Content, "hi alice") {
			noteID = n.Status.Id
		}
	}
	if noteID == "" {
		t.Fatalf("notifications = %+v, want the mention of the post", notes.Items)
	}
	if res := c.do("alice", http.MethodPost, "/statuses/"+noteID+"/bookmark", nil, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("POST bookmark = %d", res.StatusCode)
	}
	reply := c.post("alice", map[string]any{"text": "hi carol", "visibility": "public", "in_reply_to_id": noteID})
	drain()
	network.taken()

	// Her server deletes her.
	postInbox(t, c, carol, map[string]any{
		"id": carol.id + "#delete", "type": "Delete", "actor": carol.id, "object": carol.id,
		"to": []string{"https://www.w3.org/ns/activitystreams#Public"},
	})
	drain()
	actor, err := c.app.Accounts.FindByURI(ctx, carol.id)
	if err != nil || !actor.Deleted() || actor.Profile == nil {
		t.Fatalf("after the Delete: %+v, %v; want deleted, with its data until the erasure", actor, err)
	}
	handler, _, ok := c.app.Handlers.Lookup(appaccount.JobEraseDeletedAccounts)
	if !ok {
		t.Fatalf("no handler of %s", appaccount.JobEraseDeletedAccounts)
	}
	erase := func() {
		t.Helper()
		if res := handler(ctx, queue.Job{Type: appaccount.JobEraseDeletedAccounts}); res.Err() != nil {
			t.Fatalf("erasure: %v", res.Err())
		}
		drain()
	}
	erase()
	if a, err := c.app.Accounts.FindByURI(ctx, carol.id); err != nil || a.Profile == nil {
		t.Errorf("within the delay: %+v, %v; want its data", a, err)
	}

	clock.ahead.Store(int64(appaccount.ErasureDelay + time.Minute))
	erase()
	clock.ahead.Store(0) // alice's token has expired by then
	actor, err = c.app.Accounts.FindByURI(ctx, carol.id)
	if err != nil || !actor.Deleted() || actor.Profile != nil || actor.Handle != nil || len(actor.Keys) != 0 {
		t.Errorf("after the erasure: %+v, %v; want the account and its lifecycle only", actor, err)
	}
	if res := c.do("alice", http.MethodGet, "/statuses/"+noteID, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET the erased post = %d, want 404", res.StatusCode)
	}
	var bookmarks struct {
		Items []json.RawMessage `json:"items"`
	}
	if res := c.do("alice", http.MethodGet, "/bookmarks", nil, &bookmarks); res.StatusCode != http.StatusOK || len(bookmarks.Items) != 0 {
		t.Errorf("GET /bookmarks = %d, %+v; want none", res.StatusCode, bookmarks.Items)
	}
	if res := c.do("alice", http.MethodGet, "/statuses/"+reply.Id, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("GET alice's reply = %d, want 200", res.StatusCode)
	}
	if res := c.do("alice", http.MethodGet, "/statuses/"+reply.Id+"/context", nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("GET the context of alice's reply = %d, want 200", res.StatusCode)
	}
	var followers struct {
		Items []json.RawMessage `json:"items"`
	}
	if res := c.do("alice", http.MethodGet, "/accounts/"+c.ids["alice"]+"/followers", nil, &followers); res.StatusCode != http.StatusOK || len(followers.Items) != 0 {
		t.Errorf("alice's followers = %d, %+v; want none", res.StatusCode, followers.Items)
	}
	for inbox, posts := range network.taken() {
		for _, p := range posts {
			if p["type"] == "Delete" || p["type"] == "Undo" {
				t.Errorf("the erasure sent %v to %s", p["type"], inbox)
			}
		}
	}
}
