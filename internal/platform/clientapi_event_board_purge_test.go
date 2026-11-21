// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/platform"
)

// TestEventBoardPostsGoWithTheRecords checks that the purge of the boards (content.purge_boards)
// deletes the posts of the boards that are due, those of an event nobody answered too, and keeps
// those of the others and the ordinary posts (ADR-0161 §17).
func TestEventBoardPostsGoWithTheRecords(t *testing.T) {
	t.Parallel()
	e := newEventBoardEnv(t, platform.AppOptions{}, "alice", "gina")
	ctx := t.Context()
	r := e.records()
	cancelled, ended, quiet, polling, posts, ordinary := r.cancelled, r.ended, r.quiet, r.polling, r.posts, r.ordinary
	e.at(eventBoardEndsIn + 89*24*time.Hour)
	if _, err := e.app.BoardPurge.Run(ctx); err != nil {
		t.Fatalf("BoardPurge.Run = %v", err)
	}
	for _, id := range []string{cancelled, ended, quiet, polling} {
		if got := e.boardIDs("alice", id); !slices.Equal(got, posts[id]) {
			t.Errorf("within the retention, the board of %s = %v, want %v", id, got, posts[id])
		}
	}
	e.at(eventBoardEndsIn + 91*24*time.Hour)
	if _, err := e.app.BoardPurge.Run(ctx); err != nil {
		t.Fatalf("BoardPurge.Run = %v", err)
	}
	e.drain()
	for _, id := range []string{cancelled, ended, quiet} {
		if got := e.boardIDs("alice", id); len(got) != 0 {
			t.Errorf("after the retention, the board of %s = %v, want none", id, got)
		}
		for _, post := range posts[id] {
			if got := e.outcome(e.send("alice", http.MethodGet, "/statuses/"+post, nil, nil, nil)); !strings.HasPrefix(got, "404") {
				t.Errorf("after the retention, GET the board post %s = %s, want 404", post, got)
			}
		}
	}
	if got := e.boardIDs("alice", polling); !slices.Equal(got, posts[polling]) {
		t.Errorf("the board of a date poll within the retention of its last candidate = %v, want %v", got, posts[polling])
	}
	// A date poll's board goes with the retention of its last candidate date.
	e.at(eventBoardPollEndsIn + 91*24*time.Hour)
	if _, err := e.app.BoardPurge.Run(ctx); err != nil {
		t.Fatalf("BoardPurge.Run = %v", err)
	}
	e.drain()
	if got := e.boardIDs("alice", polling); len(got) != 0 {
		t.Errorf("after the retention of its last candidate, the board of a date poll = %v, want none", got)
	}
	if res := e.send("alice", http.MethodGet, "/statuses/"+ordinary, nil, nil, nil); res.StatusCode != http.StatusOK {
		t.Errorf("an ordinary post after the purge of the boards = %d, want 200", res.StatusCode)
	}
	// The purge is safe to run again.
	if _, err := e.app.BoardPurge.Run(ctx); err != nil {
		t.Errorf("BoardPurge.Run again = %v", err)
	}
}
