// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"testing"

	"github.com/mycroblog/my/internal/domain/gathering"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/platform"
)

// TestEventBoardPostsNeedReadEvents checks that a token without read:events reads no post of the
// board of an event (ADR-0090 §4): by its ID, its history, source, reactions, and thread answer 404,
// the author's own included, and a notification about one carries no status; write:events alone
// posts on a board and shows the post just written, and a token with read:events reads it all.
func TestEventBoardPostsNeedReadEvents(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	ev := e.createEvent("readers", nil)
	e.answer("gina", ev.Id, gathering.AnswerGoing)
	e.drain()

	writer := e.appToken("alice", "read:statuses write:statuses write:events")
	var post clientapi.Status
	if res := e.postOnBoard(writer, ev.Id, map[string]any{"text": "on the board"}, &post); res.StatusCode != http.StatusCreated {
		t.Fatalf("POST board with write:events = %s, want 201", e.outcome(res))
	}
	e.drain()

	narrow := e.appToken("alice", "read:statuses")
	wide := e.appToken("alice", "read:statuses read:events")
	for _, suffix := range []string{"", "/history", "/source", "/reactions", "/context"} {
		if got := e.outcome(e.send(narrow, http.MethodGet, "/statuses/"+post.Id+suffix, nil, nil, nil)); got[:3] != "404" {
			t.Errorf("GET /statuses/{id}%s of the own board post without read:events = %s, want 404", suffix, got)
		}
		if got := e.outcome(e.send(wide, http.MethodGet, "/statuses/"+post.Id+suffix, nil, nil, nil)); got != "200" {
			t.Errorf("GET /statuses/{id}%s of the own board post with read:events = %s, want 200", suffix, got)
		}
	}
	if got := e.outcome(e.send(writer, http.MethodPut, "/statuses/"+post.Id, map[string]any{"text": "edited"}, nil, nil)); got[:3] != "404" {
		t.Errorf("edit of the own board post without read:events = %s, want 404", got)
	}

	statusOf := func(token string) *clientapi.Status {
		t.Helper()
		var page clientapi.NotificationPage
		if res := e.send(token, http.MethodGet, "/notifications?types=event_board_posted", nil, nil, &page); res.StatusCode != http.StatusOK {
			t.Fatalf("GET /notifications = %s", e.outcome(res))
		}
		if len(page.Items) != 1 {
			t.Fatalf("event_board_posted notifications = %d, want 1", len(page.Items))
		}
		return page.Items[0].Status
	}
	if st := statusOf(e.appToken("gina", "read:notifications")); st != nil {
		t.Errorf("the notification shows the board post %s without read:events", st.Id)
	}
	if st := statusOf(e.appToken("gina", "read:notifications read:events")); st == nil || st.Id != post.Id {
		t.Errorf("the notification with read:events shows %v, want the board post %s", st, post.Id)
	}
}
