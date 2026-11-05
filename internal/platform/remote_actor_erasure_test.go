// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appaccount "github.com/mycroblog/my/internal/app/account"
	appfederation "github.com/mycroblog/my/internal/app/federation"
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

// goneActor is a remote actor with a post that a local user can see, marked deleted by a 410.
type goneActor struct {
	c       *statusesClient
	network *relayNet
	clock   laterClock
	carol   remoteActor
	noteID  string
}

func newGoneActor(t *testing.T) *goneActor {
	t.Helper()
	network := &relayNet{docs: map[string][]byte{}, gone: map[string]bool{}}
	clock := laterClock{steadyClock: newSteadyClock(), ahead: &atomic.Int64{}}
	c := newStatusesClientWith(t, map[string]string{"MY_QUEUE": "inline"}, platform.AppOptions{HTTPClient: network, Clock: clock}, "alice")
	carol := newRemoteActor(t, network, "https://remote.example/users/carol", "Person", "https://remote.example/users/carol/inbox")
	aliceIRI := "https://example.test/ap/actors/" + c.ids["alice"]
	note := carol.id + "/statuses/1"
	postInbox(t, c, carol, map[string]any{"id": carol.id + "/follows/1", "type": "Follow", "actor": carol.id, "object": aliceIRI})
	postInbox(t, c, carol, map[string]any{
		"id": note + "/activity", "type": "Create", "actor": carol.id,
		"object": map[string]any{
			"id": note, "type": "Note", "attributedTo": carol.id, "published": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"}, "cc": []string{aliceIRI},
			"content": "<p>hi alice</p>",
			"tag":     []any{map[string]any{"type": "Mention", "href": aliceIRI}},
		},
	})
	g := &goneActor{c: c, network: network, clock: clock, carol: carol}
	g.drain(t)
	var notes struct {
		Items []struct {
			Status *clientapi.Status `json:"status"`
		} `json:"items"`
	}
	if res := c.do("alice", http.MethodGet, "/notifications", nil, &notes); res.StatusCode != http.StatusOK {
		t.Fatalf("GET /notifications = %d", res.StatusCode)
	}
	for _, n := range notes.Items {
		if n.Status != nil && strings.Contains(n.Status.Content, "hi alice") {
			g.noteID = n.Status.Id
		}
	}
	if g.noteID == "" {
		t.Fatalf("notifications = %+v, want the mention of the post", notes.Items)
	}
	network.taken()

	g.setGone(true)
	if _, err := c.app.RemoteActors.Refresh(t.Context(), carol.id); !errors.Is(err, appfederation.ErrFetchGone) {
		t.Fatalf("Refresh(410) = %v", err)
	}
	if a := g.actor(t); !a.Deleted() || a.Profile == nil {
		t.Fatalf("after the 410: %+v; want deleted, with its data", a)
	}
	if res := c.do("alice", http.MethodGet, "/statuses/"+g.noteID, nil, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET the post of the marked actor = %d, want 404", res.StatusCode)
	}
	return g
}

func (g *goneActor) setGone(gone bool) {
	g.network.mu.Lock()
	defer g.network.mu.Unlock()
	g.network.gone[g.carol.id] = gone
}

func (g *goneActor) drain(t *testing.T) {
	t.Helper()
	if _, err := g.c.app.Runner.Drain(t.Context()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}

func (g *goneActor) actor(t *testing.T) appaccount.Actor {
	t.Helper()
	a, err := g.c.app.Accounts.FindByURI(t.Context(), g.carol.id)
	if err != nil {
		t.Fatalf("FindByURI: %v", err)
	}
	return a
}

// eraseAfter runs the erasure the given time after the 410, and comes back to the present, where
// alice's token still holds.
func (g *goneActor) eraseAfter(t *testing.T, after time.Duration) {
	t.Helper()
	handler, _, ok := g.c.app.Handlers.Lookup(appaccount.JobEraseDeletedAccounts)
	if !ok {
		t.Fatalf("no handler of %s", appaccount.JobEraseDeletedAccounts)
	}
	g.clock.ahead.Store(int64(after))
	if res := handler(t.Context(), queue.Job{Type: appaccount.JobEraseDeletedAccounts}); res.Err() != nil {
		t.Fatalf("erasure: %v", res.Err())
	}
	g.drain(t)
	g.clock.ahead.Store(0)
}

func TestRemoteActorGoneIsAskedBeforeErasure(t *testing.T) {
	t.Parallel()
	delay := appaccount.ErasureDelay + time.Minute

	t.Run("served again", func(t *testing.T) {
		t.Parallel()
		g := newGoneActor(t)
		g.setGone(false)
		g.eraseAfter(t, delay)
		if a := g.actor(t); a.Deleted() || a.Profile == nil || len(a.Keys) == 0 {
			t.Errorf("after the erasure: %+v; want the actor back with its data", a)
		}
		if res := g.c.do("alice", http.MethodGet, "/statuses/"+g.noteID, nil, nil); res.StatusCode != http.StatusOK {
			t.Errorf("GET the post of the restored actor = %d, want 200", res.StatusCode)
		}
		// Its server's actor is none of this server's to announce.
		for inbox, posts := range g.network.taken() {
			for _, p := range posts {
				t.Errorf("the restoration sent %v to %s", p["type"], inbox)
			}
		}
		// A restored actor is never erased, and its own Delete still is.
		g.eraseAfter(t, 2*delay)
		if a := g.actor(t); a.Deleted() || a.Profile == nil {
			t.Errorf("after a later erasure: %+v; want it kept", a)
		}
		postInbox(t, g.c, g.carol, map[string]any{
			"id": g.carol.id + "#delete", "type": "Delete", "actor": g.carol.id, "object": g.carol.id,
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"},
		})
		g.drain(t)
		g.eraseAfter(t, 2*delay)
		if a := g.actor(t); !a.Deleted() || a.Profile != nil {
			t.Errorf("after its Delete and the erasure: %+v; want it erased", a)
		}
	})

	t.Run("confirmed 410", func(t *testing.T) {
		t.Parallel()
		g := newGoneActor(t)
		g.eraseAfter(t, delay)
		if a := g.actor(t); !a.Deleted() || a.Profile != nil || len(a.Keys) != 0 {
			t.Errorf("after the erasure: %+v; want the account and its lifecycle only", a)
		}
	})

	t.Run("no answer until the limit", func(t *testing.T) {
		t.Parallel()
		g := newGoneActor(t)
		g.setGone(false)
		g.network.mu.Lock()
		delete(g.network.docs, g.carol.id) // 404
		g.network.mu.Unlock()
		g.eraseAfter(t, delay)
		if a := g.actor(t); !a.Deleted() || a.Profile == nil {
			t.Errorf("within the limit: %+v; want it kept, deleted", a)
		}
		g.eraseAfter(t, appaccount.ErasureRecheckLimit+2*time.Hour)
		if a := g.actor(t); !a.Deleted() || a.Profile != nil {
			t.Errorf("after the limit: %+v; want it erased", a)
		}
	})

	t.Run("signed Delete after the 410", func(t *testing.T) {
		t.Parallel()
		g := newGoneActor(t)
		postInbox(t, g.c, g.carol, map[string]any{
			"id": g.carol.id + "#delete", "type": "Delete", "actor": g.carol.id, "object": g.carol.id,
			"to": []string{"https://www.w3.org/ns/activitystreams#Public"},
		})
		g.drain(t)
		g.setGone(false) // its own word stands, whatever its server serves
		g.eraseAfter(t, delay)
		if a := g.actor(t); !a.Deleted() || a.Profile != nil {
			t.Errorf("after the erasure: %+v; want it erased", a)
		}
	})
}
