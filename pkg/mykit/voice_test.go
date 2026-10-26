// SPDX-License-Identifier: MIT

package mykit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// status is the status code of a generated operation's result, or its error.
func status[T interface{ StatusCode() int }](r T, err error) (int, error) {
	if err != nil {
		return 0, err
	}
	return r.StatusCode(), nil
}

// TestVoiceOperations calls every voice operation of the REST API (the stream and the signals belong to the WebRTC
// connection, which mykit does not have) and checks the method, the path, the query, and the body that reach the server.
func TestVoiceOperations(t *testing.T) {
	type received struct{ method, uri, body string }
	var got received
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = received{r.Method, r.URL.RequestURI(), strings.TrimSpace(string(b))}
		if r.Header.Get("Authorization") != "Bearer at" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	api, _ := newTestAPI(t, srv, Config{TokenSource: StaticToken("at")})
	ctx := context.Background()

	name, tag, cursor, id, who, session := "Late night", "go", "c1", "r1", "a1", "s1"
	end := true
	cases := []struct {
		name string
		do   func() (int, error)
		want received
	}{
		{"list", func() (int, error) {
			return status(api.ListVoiceRoomsWithResponse(ctx, &ListVoiceRoomsParams{Tag: &tag, Cursor: &cursor}))
		}, received{"GET", "/api/v1/voice/rooms?tag=go&cursor=c1", ""}},
		{"open", func() (int, error) {
			return status(api.OpenVoiceRoomWithResponse(ctx, OpenVoiceRoomJSONRequestBody{Name: name}))
		}, received{"POST", "/api/v1/voice/rooms", `{"name":"Late night"}`}},
		{"get", func() (int, error) { return status(api.GetVoiceRoomWithResponse(ctx, id)) },
			received{"GET", "/api/v1/voice/rooms/r1", ""}},
		{"update", func() (int, error) {
			return status(api.UpdateVoiceRoomWithResponse(ctx, id, UpdateVoiceRoomJSONRequestBody{Name: &name}))
		}, received{"PATCH", "/api/v1/voice/rooms/r1", `{"name":"Late night"}`}},
		{"end", func() (int, error) { return status(api.EndVoiceRoomWithResponse(ctx, id)) },
			received{"DELETE", "/api/v1/voice/rooms/r1", ""}},
		{"summary", func() (int, error) { return status(api.GetVoiceRoomSummaryWithResponse(ctx, id)) },
			received{"GET", "/api/v1/voice/rooms/r1/summary", ""}},
		{"history", func() (int, error) {
			return status(api.ListVoiceHistoryWithResponse(ctx, &ListVoiceHistoryParams{}))
		}, received{"GET", "/api/v1/voice/history", ""}},
		{"join", func() (int, error) { return status(api.JoinVoiceRoomWithResponse(ctx, id)) },
			received{"POST", "/api/v1/voice/rooms/r1/participation", ""}},
		{"leave", func() (int, error) {
			return status(api.LeaveVoiceRoomWithResponse(ctx, id, &LeaveVoiceRoomParams{NextHostId: &who, End: &end}))
		}, received{"DELETE", "/api/v1/voice/rooms/r1/participation?next_host_id=a1&end=true", ""}},
		{"heartbeat", func() (int, error) {
			return status(api.SendVoiceHeartbeatWithResponse(ctx, id, VoiceSession{Session: session}))
		}, received{"POST", "/api/v1/voice/rooms/r1/heartbeat", `{"session":"s1"}`}},
		{"hand over", func() (int, error) {
			return status(api.HandOverVoiceRoomWithResponse(ctx, id, VoiceHostChoice{AccountId: who}))
		}, received{"POST", "/api/v1/voice/rooms/r1/host", `{"account_id":"a1"}`}},
		{"become speaker", func() (int, error) { return status(api.BecomeVoiceSpeakerWithResponse(ctx, id)) },
			received{"POST", "/api/v1/voice/rooms/r1/speaker", ""}},
		{"become listener", func() (int, error) { return status(api.BecomeVoiceListenerWithResponse(ctx, id)) },
			received{"DELETE", "/api/v1/voice/rooms/r1/speaker", ""}},
		{"raise hand", func() (int, error) { return status(api.RaiseVoiceHandWithResponse(ctx, id)) },
			received{"POST", "/api/v1/voice/rooms/r1/hand", ""}},
		{"lower hand", func() (int, error) { return status(api.LowerVoiceHandWithResponse(ctx, id)) },
			received{"DELETE", "/api/v1/voice/rooms/r1/hand", ""}},
		{"grant speaking", func() (int, error) { return status(api.GrantVoiceSpeakingWithResponse(ctx, id, who)) },
			received{"POST", "/api/v1/voice/rooms/r1/participants/a1/grant", ""}},
		{"revoke speaking", func() (int, error) { return status(api.RevokeVoiceSpeakingWithResponse(ctx, id, who)) },
			received{"DELETE", "/api/v1/voice/rooms/r1/participants/a1/grant", ""}},
		{"dismiss hand", func() (int, error) { return status(api.DismissVoiceHandWithResponse(ctx, id, who)) },
			received{"DELETE", "/api/v1/voice/rooms/r1/participants/a1/hand", ""}},
		{"mute", func() (int, error) { return status(api.MuteVoiceParticipantWithResponse(ctx, id, who)) },
			received{"POST", "/api/v1/voice/rooms/r1/participants/a1/mute", ""}},
		{"remove participant", func() (int, error) { return status(api.RemoveVoiceParticipantWithResponse(ctx, id, who)) },
			received{"DELETE", "/api/v1/voice/rooms/r1/participants/a1", ""}},
		{"assign cohost", func() (int, error) { return status(api.AssignVoiceCohostWithResponse(ctx, id, who)) },
			received{"POST", "/api/v1/voice/rooms/r1/participants/a1/cohost", ""}},
		{"remove cohost", func() (int, error) { return status(api.RemoveVoiceCohostWithResponse(ctx, id, who)) },
			received{"DELETE", "/api/v1/voice/rooms/r1/participants/a1/cohost", ""}},
		{"list invitations", func() (int, error) {
			return status(api.ListVoiceInvitationsWithResponse(ctx, id, &ListVoiceInvitationsParams{}))
		}, received{"GET", "/api/v1/voice/rooms/r1/invitations", ""}},
		{"invite", func() (int, error) {
			return status(api.InviteToVoiceRoomWithResponse(ctx, id, VoiceRoomInvitationCreate{AccountId: who}))
		}, received{"POST", "/api/v1/voice/rooms/r1/invitations", `{"account_id":"a1"}`}},
		{"revoke invitation", func() (int, error) { return status(api.RevokeVoiceInvitationWithResponse(ctx, id, who)) },
			received{"DELETE", "/api/v1/voice/rooms/r1/invitations/a1", ""}},
		{"react", func() (int, error) {
			return status(api.ReactInVoiceRoomWithResponse(ctx, id, VoiceReaction{Emoji: "🎉"}))
		}, received{"POST", "/api/v1/voice/rooms/r1/reactions", `{"emoji":"🎉"}`}},
		{"mic", func() (int, error) {
			return status(api.SetVoiceMicWithResponse(ctx, id, VoiceMic{On: true, Session: session}))
		}, received{"POST", "/api/v1/voice/rooms/r1/mic", `{"on":true,"session":"s1"}`}},
		{"refresh grant", func() (int, error) {
			return status(api.RefreshVoiceGrantWithResponse(ctx, id, VoiceSession{Session: session}))
		}, received{"POST", "/api/v1/voice/rooms/r1/grant", `{"session":"s1"}`}},
		{"end as moderator", func() (int, error) {
			return status(api.EndVoiceRoomAsModeratorWithResponse(ctx, id, AdminVoiceRoomEnd{}))
		}, received{"POST", "/api/v1/admin/voice_rooms/r1/end", `{}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got = received{}
			code, err := tc.do()
			if err != nil {
				t.Fatal(err)
			}
			if code != http.StatusNoContent {
				t.Errorf("status = %d", code)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestVoiceStreamAndSignalsAreNotGenerated keeps the WebRTC operations out of the Go client.
func TestVoiceStreamAndSignalsAreNotGenerated(t *testing.T) {
	typ := reflect.TypeOf(&ClientWithResponses{})
	for _, name := range []string{"StreamVoiceRoomWithResponse", "SendVoiceSignalWithResponse", "SendVoiceSignalWithBodyWithResponse"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("%s is generated", name)
		}
	}
}
