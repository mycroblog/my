// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"net/http"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// TestVoiceRoomMaximum checks max_participants of the client API (ADR-0100 §3): the open and the
// change pass it to the use case, nil when the request has none; the room gives the maximum in
// effect, the smaller of the host's and the server's; and the use case's refusal of it is a 422
// about the field.
func TestVoiceRoomMaximum(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)
	room := "/voice/rooms/" + voiceRoomID

	if res := e.sendJSON(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk","max_participants":3}`); res.StatusCode != http.StatusCreated {
		t.Fatalf("open with max_participants = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk"}`); res.StatusCode != http.StatusCreated {
		t.Fatalf("open without max_participants = %d", res.StatusCode)
	}
	if len(f.opened) != 2 || f.opened[0].MaxParticipants == nil || *f.opened[0].MaxParticipants != 3 || f.opened[1].MaxParticipants != nil {
		t.Errorf("open inputs = %+v, want the maximum 3 and then none", f.opened)
	}
	if res := e.sendJSON(t, http.MethodPatch, room, "voice", `{"max_participants":2}`); res.StatusCode != http.StatusOK {
		t.Fatalf("change of max_participants = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPatch, room, "voice", `{"name":"New"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("change without max_participants = %d", res.StatusCode)
	}
	if len(f.patches) != 2 || f.patches[0].MaxParticipants == nil || *f.patches[0].MaxParticipants != 2 || f.patches[1].MaxParticipants != nil {
		t.Errorf("patches = %+v, want the maximum 2 and then none", f.patches)
	}

	// The server's maximum is the fake's 10.
	for _, tc := range []struct {
		name string
		host int
		want int
	}{
		{"no maximum of the host", 0, 10},
		{"the host's below the server's", 3, 3},
		{"the host's above the server's", 20, 10},
	} {
		f.maxParticipants = tc.host
		res := e.send(t, http.MethodGet, room, "read-voice", "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: get = %d", tc.name, res.StatusCode)
		}
		if got := decode[clientapi.VoiceRoom](t, res).MaxParticipants; got != tc.want {
			t.Errorf("%s: max_participants = %d, want %d", tc.name, got, tc.want)
		}
	}
	f.maxParticipants = 0

	f.err = &voice.SettingsError{Field: voice.FieldMaxParticipants}
	for _, req := range []struct{ method, path, body string }{
		{http.MethodPost, "/voice/rooms", `{"name":"Talk","max_participants":11}`},
		{http.MethodPatch, room, `{"max_participants":11}`},
	} {
		res := e.sendJSON(t, req.method, req.path, "voice", req.body)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s %s refused = %d, want 422", req.method, req.path, res.StatusCode)
			continue
		}
		if p := decodeProblem(t, res); p.Field == nil || *p.Field != "max_participants" {
			t.Errorf("%s %s refused: field = %v, want max_participants", req.method, req.path, p.Field)
		}
	}
	f.err = appvoice.ErrRoomFull
	res := e.send(t, http.MethodPost, room+"/participation", "voice", "")
	if res.StatusCode != http.StatusConflict || problemType(t, res) != clientapi.ProblemTypeVoiceRoomFull {
		t.Errorf("a full room = %d, want 409 %s", res.StatusCode, clientapi.ProblemTypeVoiceRoomFull)
	}
}

// TestInstanceVoiceMaxParticipants checks that GET /instance gives the server's maximum of a room
// while the voice chat rooms are on, and nothing while they are off (ADR-0101 §1).
func TestInstanceVoiceMaxParticipants(t *testing.T) {
	for _, tc := range []struct {
		name  string
		voice clientapi.VoiceRooms
		want  *int
	}{
		{"off", nil, nil},
		{"on", &fakeVoiceRooms{}, new(int)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newAuthEnv(t, func(o *clientapi.Options) {
				o.Instance = &fakeInstance{}
				if tc.voice != nil {
					o.Voice = tc.voice
				}
			})
			res := e.send(t, http.MethodGet, "/instance", "", "")
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET /instance = %d", res.StatusCode)
			}
			got := decode[clientapi.Instance](t, res).VoiceMaxParticipants
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("voice_max_participants = %d, want none", *got)
			case tc.want != nil && (got == nil || *got != 10):
				t.Errorf("voice_max_participants = %v, want the server's 10", got)
			}
		})
	}
}

// TestVoiceSFUGrant checks that the participation and the new grant of voice = sfu answer the
// node's wss: URL and its ticket in the schema of VoiceGrant (ADR-0101 §11).
func TestVoiceSFUGrant(t *testing.T) {
	f := &fakeVoiceRooms{grant: appvoice.Grant{
		Kind: "sfu", URL: "wss://social.example/voice/sfu", Token: "ticket-1",
		ICE:       []appvoice.ICEServer{{URLs: []string{"turns:turn.example.org:5349"}, Username: "u", Credential: "c"}},
		ExpiresAt: time.Date(2026, 10, 26, 9, 0, 30, 0, time.UTC),
	}}
	e := newVoiceEnv(t, f)
	room := "/voice/rooms/" + voiceRoomID
	check := func(what string, g clientapi.VoiceGrant) {
		t.Helper()
		if g.Kind != clientapi.VoiceGrantKind("sfu") || !g.Kind.Valid() || g.Url == nil || *g.Url != "wss://social.example/voice/sfu" ||
			g.Token == nil || *g.Token != "ticket-1" || len(g.IceServers) != 1 {
			t.Errorf("%s: grant = %+v, want sfu with the node's URL and ticket", what, g)
		}
	}
	res := e.send(t, http.MethodPost, room+"/participation", "voice", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("join = %d", res.StatusCode)
	}
	check("join", decode[clientapi.VoiceRoomJoin](t, res).Grant)
	res = e.sendJSON(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d", res.StatusCode)
	}
	check("open", decode[clientapi.VoiceRoomJoin](t, res).Grant)
	res = e.sendJSON(t, http.MethodPost, room+"/grant", "voice", `{"session":"session-1"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("grant = %d", res.StatusCode)
	}
	check("new grant", decode[clientapi.VoiceGrant](t, res))
}
