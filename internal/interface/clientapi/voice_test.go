// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

const voiceRoomID = "01890a5d-ac96-774b-bcce-b302099a8301"

// fakeVoiceRooms is one room hosted by alice, and the errors of the next calls.
type fakeVoiceRooms struct {
	err      error
	opened   []appvoice.SettingsInput
	patches  []appvoice.SettingsPatch
	leaves   []appvoice.LeaveInput
	beats    []string
	handover []string
	queries  []appvoice.ListQuery
	ops      []string
	limits   []int
	// summary is the summary of the room, nil for an unknown room; historyAfter records the
	// cursors of the history.
	summary      *appvoice.Summary
	historyAfter []string
}

func (f *fakeVoiceRooms) room() appvoice.RoomView {
	return appvoice.RoomView{
		RoomRecord: appvoice.RoomRecord{
			Room: appvoice.Room{ID: voiceRoomID, OpenedByAccountID: aliceAccountID, OpenedAt: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC), Transport: voice.TransportP2P},
			Head: appvoice.Head{RoomID: voiceRoomID, HostAccountID: aliceAccountID, State: voice.StateOpen, ParticipantsCount: 1},
			Revision: appvoice.Revision{Settings: voice.Settings{
				Name: "Talk", Description: "About", Hashtags: []voice.Hashtag{{Name: "Go", NameNormalized: "go"}},
				Visibility: voice.VisibilityLocal, Speakers: voice.SpeakersManual,
			}},
		},
		Participants: []appvoice.Participant{{AccountID: aliceAccountID, RoomID: voiceRoomID, ParticipationID: voiceRoomID, Role: voice.RoleSpeaker}},
	}
}

func (f *fakeVoiceRooms) joined() appvoice.Joined {
	return appvoice.Joined{Room: f.room(), Session: "session-1", Grant: appvoice.Grant{
		Kind: "p2p", ICE: []appvoice.ICEServer{{URLs: []string{"stun:stun.example.org"}}}, ExpiresAt: time.Date(2026, 10, 26, 15, 0, 0, 0, time.UTC),
	}}
}

func (f *fakeVoiceRooms) Open(_ context.Context, _ string, in appvoice.SettingsInput) (appvoice.Joined, error) {
	f.opened = append(f.opened, in)
	return f.joined(), f.err
}

func (f *fakeVoiceRooms) Get(context.Context, string, string) (appvoice.RoomView, error) {
	return f.room(), f.err
}

func (f *fakeVoiceRooms) List(_ context.Context, _ string, q appvoice.ListQuery) ([]appvoice.ListedRoom, bool, error) {
	f.queries = append(f.queries, q)
	return []appvoice.ListedRoom{{RoomView: f.room(), Key: appvoice.ListKey{Followed: true, Participants: 1, RoomID: voiceRoomID}}}, true, f.err
}

func (f *fakeVoiceRooms) Update(_ context.Context, _, _ string, p appvoice.SettingsPatch) (appvoice.RoomView, error) {
	f.patches = append(f.patches, p)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) End(context.Context, string, string) error { return f.err }

func (f *fakeVoiceRooms) Join(context.Context, string, string) (appvoice.Joined, error) {
	return f.joined(), f.err
}

func (f *fakeVoiceRooms) Leave(_ context.Context, _, _ string, in appvoice.LeaveInput) error {
	f.leaves = append(f.leaves, in)
	return f.err
}

func (f *fakeVoiceRooms) Heartbeat(_ context.Context, _, _, session string) error {
	f.beats = append(f.beats, session)
	return f.err
}

func (f *fakeVoiceRooms) HandOver(_ context.Context, _, _, to string) (appvoice.RoomView, error) {
	f.handover = append(f.handover, to)
	return f.room(), f.err
}

func newVoiceEnv(t *testing.T, f clientapi.VoiceRooms) *authEnv {
	t.Helper()
	e := newAuthEnv(t, func(o *clientapi.Options) { o.Voice = f })
	e.auth.tokens["voice"] = bearer("read:voice write:voice", aliceAccountID)
	e.auth.tokens["read-voice"] = bearer("read:voice", aliceAccountID)
	return e
}

func TestVoiceRooms(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)

	res := e.send(t, http.MethodGet, "/voice/rooms?tag=go&limit=1", "read-voice", "")
	if res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("list = %d, Cache-Control %q", res.StatusCode, res.Header.Get("Cache-Control"))
	}
	page := decode[clientapi.VoiceRoomPage](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].Host.Id != aliceAccountID ||
		len(page.Items[0].Participants) != 1 || !page.Items[0].Participants[0].Host || page.Items[0].Hashtags[0] != "Go" {
		t.Fatalf("list = %+v", page)
	}
	res = e.send(t, http.MethodGet, "/voice/rooms?cursor="+*page.NextCursor, "read-voice", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("next page = %d", res.StatusCode)
	}
	if q := f.queries[len(f.queries)-1]; q.After == nil || !q.After.Followed || q.After.RoomID != voiceRoomID || f.queries[0].Tag != "go" || f.queries[0].Limit != 1 {
		t.Errorf("queries = %+v", f.queries)
	}
	if res := e.send(t, http.MethodGet, "/voice/rooms?cursor=broken", "read-voice", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a broken cursor = %d, want 400", res.StatusCode)
	}

	res = e.sendJSON(t, http.MethodPost, "/voice/rooms", "voice", `{"name":"Talk","description":"a\r\nb","hashtags":["#Go"],"visibility":"followers"}`)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("open = %d", res.StatusCode)
	}
	j := decode[clientapi.VoiceRoomJoin](t, res)
	if j.Session != "session-1" || j.Grant.Kind != "p2p" || len(j.Grant.IceServers) != 1 || j.Grant.Url != nil {
		t.Errorf("open = %+v", j)
	}
	if in := f.opened[0]; in.Name != "Talk" || in.Visibility != voice.VisibilityFollowers || in.Speakers != "" || in.Hashtags[0] != "#Go" {
		t.Errorf("open input = %+v", in)
	}
	if res := e.send(t, http.MethodGet, "/voice/rooms/"+voiceRoomID, "read-voice", ""); res.StatusCode != http.StatusOK {
		t.Errorf("get = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPatch, "/voice/rooms/"+voiceRoomID, "voice", `{"name":"New"}`); res.StatusCode != http.StatusOK || *f.patches[0].Name != "New" {
		t.Errorf("update = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodDelete, "/voice/rooms/"+voiceRoomID, "voice", ""); res.StatusCode != http.StatusNoContent {
		t.Errorf("end = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/voice/rooms/"+voiceRoomID+"/participation", "voice", ""); res.StatusCode != http.StatusOK {
		t.Errorf("join = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodDelete, "/voice/rooms/"+voiceRoomID+"/participation?next_host_id="+bobAccountID, "voice", ""); res.StatusCode != http.StatusNoContent ||
		f.leaves[0].NextHostID != bobAccountID || f.leaves[0].End {
		t.Errorf("leave = %d, %+v", res.StatusCode, f.leaves)
	}
	if res := e.sendJSON(t, http.MethodPost, "/voice/rooms/"+voiceRoomID+"/heartbeat", "voice", `{"session":"session-1"}`); res.StatusCode != http.StatusNoContent || f.beats[0] != "session-1" {
		t.Errorf("heartbeat = %d", res.StatusCode)
	}
	if res := e.sendJSON(t, http.MethodPost, "/voice/rooms/"+voiceRoomID+"/host", "voice", `{"account_id":"`+bobAccountID+`"}`); res.StatusCode != http.StatusOK || f.handover[0] != bobAccountID {
		t.Errorf("hand over = %d", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, "/voice/rooms/"+voiceRoomID+"/participation", "read-voice", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("join with read:voice = %d, want 403", res.StatusCode)
	}
}

func TestVoiceRoomProblems(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)
	cases := []struct {
		err    error
		status int
		typ    string
	}{
		{appvoice.ErrNotFound, http.StatusNotFound, "about:blank"},
		{appvoice.ErrNotEligible, http.StatusForbidden, clientapi.ProblemTypeVoiceNotEligible},
		{appvoice.ErrNotHost, http.StatusForbidden, clientapi.ProblemTypeVoiceNotHost},
		{appvoice.ErrRoomFull, http.StatusConflict, clientapi.ProblemTypeVoiceRoomFull},
		{appvoice.ErrTooManyRooms, http.StatusConflict, clientapi.ProblemTypeVoiceTooManyRooms},
		{appvoice.ErrHostingElsewhere, http.StatusConflict, clientapi.ProblemTypeVoiceHostingElsewhere},
		{appvoice.ErrHostMustChoose, http.StatusConflict, clientapi.ProblemTypeVoiceHostMustChoose},
		{appvoice.ErrInvalidNextHost, http.StatusUnprocessableEntity, clientapi.ProblemTypeVoiceInvalidNextHost},
		{appvoice.ErrConflict, http.StatusConflict, clientapi.ProblemTypeVoiceConflict},
		{appvoice.ErrTransportUnavailable, http.StatusServiceUnavailable, clientapi.ProblemTypeVoiceUnavailable},
		{&voice.SettingsError{Field: voice.FieldName}, http.StatusUnprocessableEntity, "about:blank"},
	}
	for _, tc := range cases {
		f.err = tc.err
		res := e.send(t, http.MethodPost, "/voice/rooms/"+voiceRoomID+"/participation", "voice", "")
		if res.StatusCode != tc.status || problemType(t, res) != tc.typ {
			t.Errorf("%v: %d, want %d %s", tc.err, res.StatusCode, tc.status, tc.typ)
		}
	}
	f.err = appvoice.ErrHostMustChoose
	res := e.send(t, http.MethodDelete, "/voice/rooms/"+voiceRoomID+"/participation", "voice", "")
	if res.StatusCode != http.StatusConflict || res.Header.Get("Content-Language") != "ja" {
		t.Errorf("leave = %d, Content-Language %q", res.StatusCode, res.Header.Get("Content-Language"))
	}
}

func TestVoiceRoomsOff(t *testing.T) {
	e := newVoiceEnv(t, nil)
	for _, req := range []struct{ method, path, body string }{
		{http.MethodGet, "/voice/rooms", ""},
		{http.MethodPost, "/voice/rooms", `{"name":"Talk"}`},
		{http.MethodGet, "/voice/rooms/" + voiceRoomID, ""},
		{http.MethodPatch, "/voice/rooms/" + voiceRoomID, `{}`},
		{http.MethodDelete, "/voice/rooms/" + voiceRoomID, ""},
		{http.MethodPost, "/voice/rooms/" + voiceRoomID + "/participation", ""},
		{http.MethodDelete, "/voice/rooms/" + voiceRoomID + "/participation", ""},
		{http.MethodPost, "/voice/rooms/" + voiceRoomID + "/heartbeat", `{"session":"s"}`},
		{http.MethodPost, "/voice/rooms/" + voiceRoomID + "/host", `{"account_id":"` + bobAccountID + `"}`},
	} {
		for _, token := range []string{"voice", "", "read"} {
			if res := e.send(t, req.method, req.path, token, req.body); res.StatusCode != http.StatusNotFound {
				t.Errorf("%s %s with the voice chat off and token %q = %d, want 404", req.method, req.path, token, res.StatusCode)
			}
		}
	}
}
