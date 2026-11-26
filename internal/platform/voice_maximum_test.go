// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"testing"

	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestVoiceRoomMaximumThroughAPI drives the room's maximum through the client API on both
// dialects (ADR-0100 §3, §5, ADR-0101 §1): GET /instance gives the server's maximum; opening and
// changing a room take max_participants from 2 to it and refuse anything else with a 422 about
// the field; the room gives the maximum in effect; a join at it is a 409 voice-room-full; and
// lowering it below the people in the room removes nobody.
func TestVoiceRoomMaximumThroughAPI(t *testing.T) {
	t.Parallel()
	for _, db := range []struct {
		name string
		env  func(t *testing.T) map[string]string
	}{
		{"sqlite", func(t *testing.T) map[string]string {
			return map[string]string{"MY_DATABASE_URL": newDatabase(t, true)}
		}},
		{"postgres", func(t *testing.T) map[string]string {
			return map[string]string{"MY_DATABASE": "postgres", "MY_DATABASE_URL": newPostgresDatabase(t)}
		}},
	} {
		t.Run(db.name, func(t *testing.T) {
			t.Parallel()
			env := db.env(t)
			for k, v := range map[string]string{
				"MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org", "MY_VOICE_MAX_PARTICIPANTS": "4",
			} {
				env[k] = v
			}
			app, err := platform.NewApp(t.Context(), loadConfig(t, env), platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
			if err != nil {
				t.Fatalf("NewApp: %v", err)
			}
			t.Cleanup(func() { _ = app.Close() })
			voiceRoomMaximum(t, &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}, 4)
		})
	}
}

// wantMaximumProblem fails the test unless res is a 422 about max_participants.
func (e *accountsEnv) wantMaximumProblem(res *http.Response, what string) {
	e.t.Helper()
	if res.StatusCode != http.StatusUnprocessableEntity {
		e.t.Errorf("%s = %d, want 422", what, res.StatusCode)
		return
	}
	var p clientapi.Problem
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		e.t.Fatal(err)
	}
	if p.Field == nil || *p.Field != "max_participants" {
		e.t.Errorf("%s: field = %v, want max_participants", what, p.Field)
	}
}

func voiceRoomMaximum(t *testing.T, e *accountsEnv, server int) {
	host := e.user("alice")
	member := func(name string) string { return e.member(name).token }

	var inst clientapi.Instance
	e.want(e.do(http.MethodGet, "/instance", "", nil, &inst), http.StatusOK, "GET /instance")
	if inst.VoiceMaxParticipants == nil || *inst.VoiceMaxParticipants != server {
		t.Errorf("voice_max_participants = %v, want %d", inst.VoiceMaxParticipants, server)
	}

	for _, n := range []int{0, 1, server + 1} {
		e.wantMaximumProblem(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Talk", "max_participants": n}, nil),
			"open with max_participants "+strconv.Itoa(n))
	}

	// Without a maximum, the room holds the server's.
	var free clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Free"}, &free), http.StatusCreated, "open without a maximum")
	if free.Room.MaxParticipants != server {
		t.Errorf("max_participants without the host's = %d, want the server's %d", free.Room.MaxParticipants, server)
	}
	e.want(e.do(http.MethodDelete, "/voice/rooms/"+free.Room.Id, host.token, nil, nil), http.StatusNoContent, "end the room without a maximum")

	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", host.token, map[string]any{"name": "Three", "max_participants": 3}, &opened), http.StatusCreated, "open with 3")
	room := "/voice/rooms/" + opened.Room.Id
	if opened.Room.MaxParticipants != 3 {
		t.Errorf("max_participants on open = %d, want 3", opened.Room.MaxParticipants)
	}
	get := func(what string) clientapi.VoiceRoom {
		t.Helper()
		var got clientapi.VoiceRoom
		e.want(e.do(http.MethodGet, room, host.token, nil, &got), http.StatusOK, what)
		return got
	}
	if got := get("GET the room"); got.MaxParticipants != 3 {
		t.Errorf("GET: max_participants = %d, want 3", got.MaxParticipants)
	}
	e.want(e.do(http.MethodPost, room+"/participation", member("bob"), nil, nil), http.StatusOK, "join of bob")
	e.want(e.do(http.MethodPost, room+"/participation", member("carol"), nil, nil), http.StatusOK, "join of carol")
	if p := e.want(e.do(http.MethodPost, room+"/participation", member("dave"), nil, nil), http.StatusConflict, "join at the maximum"); p != clientapi.ProblemTypeVoiceRoomFull {
		t.Errorf("join at the maximum = %s, want %s", p, clientapi.ProblemTypeVoiceRoomFull)
	}

	// Lowering it below the people in the room removes nobody, and refuses the next join.
	var lowered clientapi.VoiceRoom
	e.want(e.do(http.MethodPatch, room, host.token, map[string]any{"max_participants": 2}, &lowered), http.StatusOK, "lower to 2")
	if lowered.MaxParticipants != 2 || lowered.ParticipantsCount != 3 {
		t.Errorf("lowered: max_participants %d, participants %d; want 2 and the 3 who were in", lowered.MaxParticipants, lowered.ParticipantsCount)
	}
	if p := e.want(e.do(http.MethodPost, room+"/participation", member("eve"), nil, nil), http.StatusConflict, "join after lowering"); p != clientapi.ProblemTypeVoiceRoomFull {
		t.Errorf("join after lowering = %s, want %s", p, clientapi.ProblemTypeVoiceRoomFull)
	}
	for _, n := range []int{0, 1, server + 1} {
		e.wantMaximumProblem(e.do(http.MethodPatch, room, host.token, map[string]any{"max_participants": n}, nil),
			"change to max_participants "+strconv.Itoa(n))
	}
	var renamed clientapi.VoiceRoom
	e.want(e.do(http.MethodPatch, room, host.token, map[string]any{"name": "Renamed"}, &renamed), http.StatusOK, "rename")
	if renamed.MaxParticipants != 2 {
		t.Errorf("max_participants after a change without it = %d, want 2", renamed.MaxParticipants)
	}
	if got := get("GET after the refused changes"); got.MaxParticipants != 2 || got.ParticipantsCount != 3 {
		t.Errorf("GET: max_participants %d, participants %d; want 2 and 3", got.MaxParticipants, got.ParticipantsCount)
	}

	// Raising it to the server's admits again, up to it.
	e.want(e.do(http.MethodPatch, room, host.token, map[string]any{"max_participants": server}, nil), http.StatusOK, "raise to the server's")
	e.want(e.do(http.MethodPost, room+"/participation", member("frank"), nil, nil), http.StatusOK, "join after raising")
	if p := e.want(e.do(http.MethodPost, room+"/participation", member("grace"), nil, nil), http.StatusConflict, "join at the server's maximum"); p != clientapi.ProblemTypeVoiceRoomFull {
		t.Errorf("join at the server's maximum = %s, want %s", p, clientapi.ProblemTypeVoiceRoomFull)
	}
	if got := get("GET at the server's maximum"); got.MaxParticipants != server || got.ParticipantsCount != server {
		t.Errorf("GET: max_participants %d, participants %d; want %d and %d", got.MaxParticipants, got.ParticipantsCount, server, server)
	}
}
