// SPDX-License-Identifier: AGPL-3.0-only

package livekit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeServer is an httptest stand-in for the Twirp API of a LiveKit server's RoomService, with
// the requests and responses of the spike report (#1347) and LiveKit's API reference: it
// checks the path, the token of every call and its grants, and the JSON names of the requests.
type fakeServer struct {
	t      *testing.T
	key    string
	secret string
	now    func() time.Time

	mu    sync.Mutex
	rooms map[string]*fakeRoom
	calls []string
}

type fakeRoom struct {
	opts         map[string]any
	participants map[string]*fakeParticipant
}

type fakeParticipant struct {
	permission map[string]any
	tracks     []map[string]any
}

func newFakeServer(t *testing.T, key, secret string, now func() time.Time) (*fakeServer, *httptest.Server) {
	f := &fakeServer{t: t, key: key, secret: secret, now: now, rooms: map[string]*fakeRoom{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

// connect puts a participant with one microphone track in a room, as a client that connected.
func (f *fakeServer) connect(room, identity string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.rooms[room]
	if r == nil {
		f.t.Fatalf("connect to room %s, which was not created", room)
	}
	r.participants[identity] = &fakeParticipant{tracks: []map[string]any{{"sid": "TR_" + identity, "source": "MICROPHONE"}}}
}

func (f *fakeServer) participant(room, identity string) *fakeParticipant {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r := f.rooms[room]; r != nil {
		return r.participants[identity]
	}
	return nil
}

func twirpError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "msg": msg})
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method, ok := strings.CutPrefix(r.URL.Path, "/twirp/livekit.RoomService/")
	if r.Method != http.MethodPost || !ok {
		twirpError(w, http.StatusNotFound, "bad_route", r.URL.Path)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		twirpError(w, http.StatusBadRequest, "malformed", "content type "+ct)
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		twirpError(w, http.StatusUnauthorized, "unauthenticated", "no bearer token")
		return
	}
	c, err := verify(token, f.key, f.secret, f.now())
	if err != nil || c.Video == nil {
		twirpError(w, http.StatusUnauthorized, "unauthenticated", "bad token")
		return
	}
	b, _ := io.ReadAll(r.Body)
	var req map[string]any
	if err := json.Unmarshal(b, &req); err != nil {
		twirpError(w, http.StatusBadRequest, "malformed", err.Error())
		return
	}
	room, _ := req["room"].(string)
	if method == "CreateRoom" {
		room, _ = req["name"].(string)
	}
	switch method {
	case "CreateRoom", "DeleteRoom":
		if !c.Video.RoomCreate {
			twirpError(w, http.StatusForbidden, "permission_denied", "roomCreate")
			return
		}
	default:
		if !c.Video.RoomAdmin || c.Video.Room != room {
			twirpError(w, http.StatusForbidden, "permission_denied", "roomAdmin of "+room)
			return
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method+" "+room)
	res := map[string]any{}
	rm := f.rooms[room]
	identity, _ := req["identity"].(string)
	var p *fakeParticipant
	if rm != nil {
		p = rm.participants[identity]
	}
	switch method {
	case "CreateRoom":
		for _, k := range []string{"empty_timeout", "departure_timeout", "max_participants"} {
			if _, ok := req[k]; !ok {
				twirpError(w, http.StatusBadRequest, "malformed", "no "+k)
				return
			}
		}
		if rm == nil {
			f.rooms[room] = &fakeRoom{opts: req, participants: map[string]*fakeParticipant{}}
		}
		res = map[string]any{"name": room, "sid": "RM_" + room}
	case "DeleteRoom":
		if rm == nil {
			twirpError(w, http.StatusNotFound, "not_found", "room not found")
			return
		}
		delete(f.rooms, room)
	case "ListParticipants":
		if rm == nil {
			twirpError(w, http.StatusNotFound, "not_found", "room not found")
			return
		}
		var ps []map[string]any
		for id, p := range rm.participants {
			ps = append(ps, map[string]any{"sid": "PA_" + id, "identity": id, "tracks": p.tracks})
		}
		res = map[string]any{"participants": ps}
	case "GetParticipant", "UpdateParticipant", "RemoveParticipant", "MutePublishedTrack":
		if p == nil {
			twirpError(w, http.StatusNotFound, "not_found", "participant not found")
			return
		}
		switch method {
		case "GetParticipant":
			res = map[string]any{"identity": identity, "tracks": p.tracks}
		case "UpdateParticipant":
			perm, ok := req["permission"].(map[string]any)
			if !ok {
				twirpError(w, http.StatusBadRequest, "malformed", "no permission")
				return
			}
			p.permission = perm
		case "RemoveParticipant":
			delete(rm.participants, identity)
		case "MutePublishedTrack":
			sid, _ := req["track_sid"].(string)
			for _, tr := range p.tracks {
				if tr["sid"] == sid {
					tr["muted"] = req["muted"]
				}
			}
		}
	default:
		twirpError(w, http.StatusNotFound, "bad_route", method)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
