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
// link preview, an ended one only its name and that it ended, and neither is indexed; a room
// that shows no summary (voice_public_summary off, a room not for every local user), open or
// ended, is the generic shell with 200 and no preview of the room, for the web client to show
// that only the local users join; an unknown room or a path that names none is the generic
// shell with 404. The signed-in viewers' web client loads the room itself, so the shell is the
// same for every visitor.
func TestVoiceRoomPages(t *testing.T) {
	summaries := map[string]web.VoiceRoomSummary{}
	mux := previewHandler(t, &fakePreviews{}, web.Options{
		VoiceRooms: func(_ context.Context, id string) (web.VoiceRoomSummary, bool, error) {
			s, ok := summaries[id]
			return s, ok, nil
		},
	})
	generic := get(mux, "/home", nil)
	shell := func(path string, status int) {
		t.Helper()
		rec := get(mux, path, nil)
		if rec.Code != status || rec.Body.String() != generic.Body.String() || rec.Header().Get("X-Robots-Tag") != "noindex, noarchive" {
			t.Errorf("GET %s = %d, want %d with the generic shell", path, rec.Code, status)
		}
		if strings.Contains(rec.Body.String(), "og:title") {
			t.Errorf("GET %s carries a link preview:\n%s", path, rec.Body)
		}
	}
	// Unknown rooms, and paths that name none (the voice chat off reports none either).
	shell("/voice/"+voiceRoomID, http.StatusNotFound)
	shell("/voice/not-a-room", http.StatusNotFound)
	shell("/voice/"+strings.ToUpper(voiceRoomID), http.StatusNotFound)

	// A room that shows no summary, open or ended.
	summaries[voiceRoomID] = web.VoiceRoomSummary{}
	shell("/voice/"+voiceRoomID, http.StatusOK)
	summaries[voiceRoomID] = web.VoiceRoomSummary{Ended: true}
	shell("/voice/"+voiceRoomID, http.StatusOK)

	summaries[voiceRoomID] = web.VoiceRoomSummary{Shown: true, Name: "Go talk", Description: "About\nGo", HostName: "Alice"}
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

	summaries[voiceRoomID] = web.VoiceRoomSummary{Shown: true, Name: "Go talk", Ended: true}
	m = metas(t, get(mux, "/voice/"+voiceRoomID, nil).Body.String())
	if m["og:title"] != "Go talk — this voice chat has ended" || m["og:description"] != "" {
		t.Errorf("metas of an ended room = %v", m)
	}
}
