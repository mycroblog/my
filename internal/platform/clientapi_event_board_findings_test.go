// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/platform"
)

// TestEventBoardPostsFetchNoLinkCard checks that a board post with a link enqueues no fetch of a
// link card (ADR-0161 §17, ADR-0062): the server never fetches the links of a post that never
// leaves its event.
func TestEventBoardPostsFetchNoLinkCard(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	ev := e.createEvent("readers", nil)
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	e.drain()
	board := e.mustPostOnBoard("alice", ev.Id, map[string]any{"text": "the map: https://example.com/board"})
	edited := e.send("alice", http.MethodPut, "/statuses/"+board.Id, map[string]any{"text": "the map: https://example.com/edited"}, nil, nil)
	if edited.StatusCode != http.StatusOK {
		t.Fatalf("edit = %s", e.outcome(edited))
	}
	for _, j := range e.pendingJobs() {
		if j.Type == appcontent.JobFetchLinkCard || strings.Contains(string(j.Payload), board.Id) {
			t.Errorf("a board post with a link enqueues the job %s %s %s", j.Type, j.IdempotencyKey, j.Payload)
		}
	}
}

// TestEventBoardPostsAreNotCountedInTheOutbox checks that the ActivityPub outbox of an author
// counts none of the author's board posts (ADR-0161 §17): a remote server learns nothing of them,
// their number included.
func TestEventBoardPostsAreNotCountedInTheOutbox(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	ev := e.createEvent("readers", nil)
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	e.post("alice", map[string]any{"text": "an ordinary post", "visibility": "public"})
	board := e.mustPostOnBoard("alice", ev.Id, map[string]any{"text": "on the board"})
	e.mustPostOnBoard("gina", ev.Id, map[string]any{"text": "a reply on the board", "in_reply_to_id": board.Id})
	// NodeInfo counts the one ordinary post (observed: "localPosts":3).
	if _, ni := e.federationGet("/nodeinfo/2.1", "application/json"); !strings.Contains(ni, `"localPosts":1`) {
		t.Errorf("nodeinfo = %s, want 1 local post: the board posts do not count", ni)
	}
	for name, want := range map[string]int{"alice": 1, "gina": 0} {
		_, body := e.federationGet("/ap/actors/"+e.ids[name]+"/outbox", "application/activity+json")
		var outbox struct {
			TotalItems int `json:"totalItems"`
		}
		if err := json.Unmarshal([]byte(body), &outbox); err != nil {
			t.Fatal(err)
		}
		if outbox.TotalItems != want {
			t.Errorf("the outbox of %s counts %d items, want %d: the board posts do not count", name, outbox.TotalItems, want)
		}
	}
}
