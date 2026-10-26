// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// The connections of fakeVoiceRooms: the room stream is verified end to end by the platform
// tests (TestVoiceStreamsAcrossProcesses).

func (f *fakeVoiceRooms) BlockedAmong(context.Context, string, []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (f *fakeVoiceRooms) OpenStream(context.Context, string, string, string) (appvoice.RoomStream, error) {
	return appvoice.RoomStream{}, f.err
}

func (f *fakeVoiceRooms) StreamCurrent(context.Context, appvoice.RoomStream) (bool, error) {
	return true, nil
}

func (f *fakeVoiceRooms) RenewStream(context.Context, appvoice.RoomStream) (bool, error) {
	return true, nil
}

func (f *fakeVoiceRooms) StreamState(context.Context, appvoice.RoomStream, time.Time) (appvoice.StreamState, error) {
	return appvoice.StreamState{Room: f.room()}, f.err
}

func (f *fakeVoiceRooms) Message(context.Context, appvoice.RoomStream, string) (appvoice.Message, bool, error) {
	return appvoice.Message{}, false, nil
}

func (f *fakeVoiceRooms) Blocked(context.Context, string, string) (bool, error) { return false, nil }

func (f *fakeVoiceRooms) Signal(_ context.Context, _, _ string, in appvoice.SignalInput) error {
	f.ops = append(f.ops, "signal:"+string(in.Kind)+":"+in.To)
	return f.err
}

func (f *fakeVoiceRooms) Mic(_ context.Context, _, _, session string, on bool) error {
	state := "off"
	if on {
		state = "on"
	}
	f.ops = append(f.ops, "mic:"+session+":"+state)
	return f.err
}

func (f *fakeVoiceRooms) React(_ context.Context, _, _, emoji string) error {
	f.ops = append(f.ops, "react:"+emoji)
	return f.err
}

func (f *fakeVoiceRooms) RefreshGrant(_ context.Context, _, _, session string) (appvoice.Grant, error) {
	f.ops = append(f.ops, "grant:"+session)
	return f.joined().Grant, f.err
}

func TestVoiceConnections(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)
	room := "/voice/rooms/" + voiceRoomID
	if res := e.sendJSON(t, http.MethodPost, room+"/signals", "voice", `{"session":"s1","to":"s2","kind":"offer","sdp":"v=0"}`); res.StatusCode != http.StatusNoContent {
		t.Errorf("signal = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, room+"/mic", "voice", `{"session":"s1","on":true}`); res.StatusCode != http.StatusNoContent {
		t.Errorf("mic = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, room+"/reactions", "voice", `{"emoji":"👏"}`); res.StatusCode != http.StatusNoContent {
		t.Errorf("react = %d", res.StatusCode)
	}
	res := e.sendJSON(t, http.MethodPost, room+"/grant", "voice", `{"session":"s1"}`)
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("grant = %d", res.StatusCode)
	}
	if g := decode[clientapi.VoiceGrant](t, res); g.Kind != "p2p" || len(g.IceServers) != 1 {
		t.Errorf("grant = %+v", g)
	}
	want := []string{"signal:offer:s2", "mic:s1:on", "react:👏", "grant:s1"}
	if len(f.ops) != len(want) {
		t.Fatalf("ops = %v, want %v", f.ops, want)
	}
	for i := range want {
		if f.ops[i] != want[i] {
			t.Errorf("ops = %v, want %v", f.ops, want)
		}
	}
	if res := e.sendJSON(t, http.MethodPost, room+"/mic", "read-voice", `{"session":"s1","on":false}`); res.StatusCode != http.StatusForbidden {
		t.Errorf("mic with read:voice = %d, want 403", res.StatusCode)
	}

	cases := []struct {
		err    error
		status int
	}{
		{&appvoice.SignalError{Field: "sdp"}, http.StatusUnprocessableEntity},
		{appvoice.ErrInvalidEmoji, http.StatusUnprocessableEntity},
		{&kernel.RateLimitedError{RetryAfter: time.Second}, http.StatusTooManyRequests},
		{appvoice.ErrNotFound, http.StatusNotFound},
		{appvoice.ErrCannotSpeak, http.StatusForbidden},
	}
	for _, tc := range cases {
		f.err = tc.err
		if res := e.sendJSON(t, http.MethodPost, room+"/signals", "voice", `{"session":"s1","to":"s2","kind":"restart"}`); res.StatusCode != tc.status {
			t.Errorf("signal with %v = %d, want %d", tc.err, res.StatusCode, tc.status)
		}
	}
	f.err = appvoice.ErrNotFound
	if res := e.send(t, http.MethodGet, room+"/stream?session=s1", "read-voice", ""); res.StatusCode != http.StatusNotFound {
		t.Errorf("stream of an unknown session = %d, want 404", res.StatusCode)
	}
}

func TestVoiceConnectionsOff(t *testing.T) {
	e := newVoiceEnv(t, nil)
	room := "/voice/rooms/" + voiceRoomID
	for _, req := range []struct{ method, path, body string }{
		{http.MethodGet, room + "/stream?session=s", ""},
		{http.MethodPost, room + "/signals", `{"session":"s","to":"t","kind":"restart"}`},
		{http.MethodPost, room + "/mic", `{"session":"s","on":true}`},
		{http.MethodPost, room + "/grant", `{"session":"s"}`},
		{http.MethodPost, room + "/reactions", `{"emoji":"👏"}`},
	} {
		for _, token := range []string{"voice", ""} {
			if res := e.send(t, req.method, req.path, token, req.body); res.StatusCode != http.StatusNotFound {
				t.Errorf("%s %s with the voice chat off and token %q = %d, want 404", req.method, req.path, token, res.StatusCode)
			}
		}
	}
}
