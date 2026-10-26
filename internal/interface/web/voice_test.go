// SPDX-License-Identifier: AGPL-3.0-only

package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mycroblog/my/internal/interface/web"
)

const voiceRoomID = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e70"

// TestVoiceRoomPages checks the shells of the pages of the voice chat rooms (ADR-0100 §9,
// ADR-0028): an open room that shows its summary carries its name, host, and description in the
// link preview, an ended one only its name and that it ended, neither is indexed, and a room
// that shows no summary (voice_public_summary off, a room not for every local user), an unknown
// one, or a path that is no room is the generic shell with 404. The signed-in viewers' web client
// loads the room itself, so the shell is the same for every visitor.
func TestVoiceRoomPages(t *testing.T) {
	summaries := map[string]web.VoiceRoomSummary{}
	mux := previewHandler(t, &fakePreviews{}, web.Options{
		VoiceRooms: func(_ context.Context, id string) (web.VoiceRoomSummary, bool, error) {
			s, ok := summaries[id]
			return s, ok, nil
		},
	})
	generic := get(mux, "/home", nil)
	hidden := func(path string) {
		t.Helper()
		rec := get(mux, path, nil)
		if rec.Code != http.StatusNotFound || rec.Body.String() != generic.Body.String() || rec.Header().Get("X-Robots-Tag") != "noindex, noarchive" {
			t.Errorf("GET %s = %d, want 404 with the generic shell", path, rec.Code)
		}
	}
	// voice_public_summary off, or a room not for every local user: the platform reports no
	// summary, as for an unknown room.
	hidden("/voice/" + voiceRoomID)
	hidden("/voice/not-a-room")
	hidden("/voice/" + strings.ToUpper(voiceRoomID))

	summaries[voiceRoomID] = web.VoiceRoomSummary{Name: "Go talk", Description: "About\nGo", HostName: "Alice"}
	rec := get(mux, "/voice/"+voiceRoomID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET an open room = %d", rec.Code)
	}
	m := metas(t, rec.Body.String())
	if m["og:title"] != "Go talk — a voice chat hosted by Alice, open now" || m["og:description"] != "About Go" ||
		!strings.HasSuffix(m["og:url"], "/voice/"+voiceRoomID) || !strings.Contains(m["robots"], "noindex") || m["og:type"] != "website" {
		t.Errorf("metas of an open room = %v", m)
	}
	if !strings.Contains(rec.Body.String(), "<title>Go talk — a voice chat hosted by Alice, open now</title>") {
		t.Errorf("the page title of an open room is not its preview title:\n%s", rec.Body)
	}

	summaries[voiceRoomID] = web.VoiceRoomSummary{Name: "Go talk", Ended: true}
	m = metas(t, get(mux, "/voice/"+voiceRoomID, nil).Body.String())
	if m["og:title"] != "Go talk — this voice chat has ended" || m["og:description"] != "" {
		t.Errorf("metas of an ended room = %v", m)
	}
}
