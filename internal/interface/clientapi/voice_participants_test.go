// SPDX-License-Identifier: AGPL-3.0-only

package clientapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
)

// The participant operations of fakeVoiceRooms record "<operation>:<account>".

func (f *fakeVoiceRooms) record(op, account string) { f.ops = append(f.ops, op+":"+account) }

func (f *fakeVoiceRooms) Speak(_ context.Context, me, _ string) (appvoice.RoomView, error) {
	f.record("speak", me)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) Listen(_ context.Context, me, _ string) (appvoice.RoomView, error) {
	f.record("listen", me)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) RaiseHand(_ context.Context, me, _ string) (appvoice.RoomView, error) {
	f.record("raise", me)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) LowerHand(_ context.Context, me, _ string) (appvoice.RoomView, error) {
	f.record("lower", me)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) Grant(_ context.Context, _, _, to string) (appvoice.RoomView, error) {
	f.record("grant", to)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) Revoke(_ context.Context, _, _, to string) (appvoice.RoomView, error) {
	f.record("revoke", to)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) Dismiss(_ context.Context, _, _, to string) (appvoice.RoomView, error) {
	f.record("dismiss", to)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) Mute(_ context.Context, _, _, to string) error {
	f.record("mute", to)
	return f.err
}

func (f *fakeVoiceRooms) Remove(_ context.Context, _, _, to string) error {
	f.record("remove", to)
	return f.err
}

func (f *fakeVoiceRooms) AssignCohost(_ context.Context, _, _, to string) (appvoice.RoomView, error) {
	f.record("cohost", to)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) RemoveCohost(_ context.Context, _, _, to string) (appvoice.RoomView, error) {
	f.record("uncohost", to)
	return f.room(), f.err
}

func (f *fakeVoiceRooms) invitation(to string, role voice.Role) appvoice.Invitation {
	return appvoice.Invitation{
		ID: voiceRoomID, RoomID: voiceRoomID, InviteeAccountID: to, Role: role, InvitedByAccountID: aliceAccountID,
		InvitedAt: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC),
	}
}

func (f *fakeVoiceRooms) Invite(_ context.Context, _, _, to string, role voice.Role) (appvoice.Invitation, error) {
	f.record("invite-"+string(role), to)
	if role == "" {
		role = voice.RoleListener
	}
	return f.invitation(to, role), f.err
}

func (f *fakeVoiceRooms) RevokeInvitation(_ context.Context, _, _, to string) error {
	f.record("uninvite", to)
	return f.err
}

func (f *fakeVoiceRooms) Invitations(_ context.Context, _, _, after string, limit int) ([]appvoice.Invitation, bool, error) {
	f.record("invitations", after)
	f.limits = append(f.limits, limit)
	return []appvoice.Invitation{f.invitation(aliceAccountID, voice.RoleSpeaker)}, true, f.err
}

func TestVoiceParticipantOperations(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)
	room := "/voice/rooms/" + voiceRoomID
	member := room + "/participants/" + bobAccountID
	for _, tc := range []struct {
		method, path, body string
		status             int
		op                 string
	}{
		{http.MethodPost, room + "/speaker", "", http.StatusOK, "speak:" + aliceAccountID},
		{http.MethodDelete, room + "/speaker", "", http.StatusOK, "listen:" + aliceAccountID},
		{http.MethodPost, room + "/hand", "", http.StatusOK, "raise:" + aliceAccountID},
		{http.MethodDelete, room + "/hand", "", http.StatusOK, "lower:" + aliceAccountID},
		{http.MethodPost, member + "/grant", "", http.StatusOK, "grant:" + bobAccountID},
		{http.MethodDelete, member + "/grant", "", http.StatusOK, "revoke:" + bobAccountID},
		{http.MethodDelete, member + "/hand", "", http.StatusOK, "dismiss:" + bobAccountID},
		{http.MethodPost, member + "/mute", "", http.StatusNoContent, "mute:" + bobAccountID},
		{http.MethodDelete, member, "", http.StatusNoContent, "remove:" + bobAccountID},
		{http.MethodPost, member + "/cohost", "", http.StatusOK, "cohost:" + bobAccountID},
		{http.MethodDelete, member + "/cohost", "", http.StatusOK, "uncohost:" + bobAccountID},
		{http.MethodPost, room + "/invitations", `{"account_id":"` + aliceAccountID + `"}`, http.StatusCreated, "invite-:" + aliceAccountID},
		{http.MethodPost, room + "/invitations", `{"account_id":"` + aliceAccountID + `","role":"speaker"}`, http.StatusCreated, "invite-speaker:" + aliceAccountID},
		{http.MethodDelete, room + "/invitations/" + bobAccountID, "", http.StatusNoContent, "uninvite:" + bobAccountID},
	} {
		var res *http.Response
		if tc.body != "" {
			res = e.sendJSON(t, tc.method, tc.path, "voice", tc.body)
		} else {
			res = e.send(t, tc.method, tc.path, "voice", "")
		}
		if res.StatusCode != tc.status || f.ops[len(f.ops)-1] != tc.op {
			t.Errorf("%s %s = %d, %v; want %d, %s", tc.method, tc.path, res.StatusCode, f.ops, tc.status, tc.op)
		}
		if tc.status == http.StatusOK && res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s %s Cache-Control = %q", tc.method, tc.path, res.Header.Get("Cache-Control"))
		}
		_ = res.Body.Close()
	}

	res := e.send(t, http.MethodGet, room+"/invitations?limit=1", "read-voice", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("invitations = %d", res.StatusCode)
	}
	page := decode[clientapi.VoiceRoomInvitationPage](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].Role != clientapi.VoiceRoleSpeaker || page.Items[0].Account.Id != aliceAccountID {
		t.Fatalf("invitations = %+v", page)
	}
	if res := e.send(t, http.MethodGet, room+"/invitations?cursor="+*page.NextCursor, "read-voice", ""); res.StatusCode != http.StatusOK ||
		f.ops[len(f.ops)-1] != "invitations:"+voiceRoomID || f.limits[0] != 1 {
		t.Errorf("next page = %d, %v, limits %v", res.StatusCode, f.ops, f.limits)
	}
	if res := e.send(t, http.MethodGet, room+"/invitations?cursor=broken", "read-voice", ""); res.StatusCode != http.StatusBadRequest {
		t.Errorf("a broken cursor = %d, want 400", res.StatusCode)
	}
	if res := e.send(t, http.MethodPost, member+"/mute", "read-voice", ""); res.StatusCode != http.StatusForbidden {
		t.Errorf("mute with read:voice = %d, want 403", res.StatusCode)
	}
}

func TestVoiceParticipantProblems(t *testing.T) {
	f := &fakeVoiceRooms{}
	e := newVoiceEnv(t, f)
	member := "/voice/rooms/" + voiceRoomID + "/participants/" + bobAccountID
	cases := []struct {
		err    error
		status int
		typ    string
	}{
		{appvoice.ErrNotHostOrCohost, http.StatusForbidden, clientapi.ProblemTypeVoiceNotHostOrCohost},
		{appvoice.ErrCannotSpeak, http.StatusForbidden, clientapi.ProblemTypeVoiceCannotSpeak},
		{appvoice.ErrInvalidParticipant, http.StatusUnprocessableEntity, clientapi.ProblemTypeVoiceInvalidParticipant},
		{appvoice.ErrTooManyCohosts, http.StatusConflict, clientapi.ProblemTypeVoiceTooManyCohosts},
		{appvoice.ErrInvalidInvitee, http.StatusUnprocessableEntity, clientapi.ProblemTypeVoiceInvalidInvitee},
		{appvoice.ErrAlreadyInvited, http.StatusConflict, clientapi.ProblemTypeVoiceAlreadyInvited},
		{appvoice.ErrInvalidRole, http.StatusUnprocessableEntity, "about:blank"},
		{appvoice.ErrNotHost, http.StatusForbidden, clientapi.ProblemTypeVoiceNotHost},
		{appvoice.ErrNotFound, http.StatusNotFound, "about:blank"},
	}
	for _, tc := range cases {
		f.err = tc.err
		res := e.send(t, http.MethodPost, member+"/cohost", "voice", "")
		if res.StatusCode != tc.status || problemType(t, res) != tc.typ {
			t.Errorf("%v: %d, want %d %s", tc.err, res.StatusCode, tc.status, tc.typ)
		}
	}
	f.err = &kernel.RateLimitedError{RetryAfter: 90 * time.Second}
	res := e.sendJSON(t, http.MethodPost, "/voice/rooms/"+voiceRoomID+"/invitations", "voice", `{"account_id":"`+bobAccountID+`"}`)
	if res.StatusCode != http.StatusTooManyRequests || res.Header.Get("Retry-After") != "90" {
		t.Errorf("invite past the limit = %d, Retry-After %q", res.StatusCode, res.Header.Get("Retry-After"))
	}
}

func TestVoiceParticipantOperationsOff(t *testing.T) {
	e := newVoiceEnv(t, nil)
	room := "/voice/rooms/" + voiceRoomID
	member := room + "/participants/" + bobAccountID
	for _, req := range []struct{ method, path, body string }{
		{http.MethodPost, room + "/speaker", ""},
		{http.MethodDelete, room + "/speaker", ""},
		{http.MethodPost, room + "/hand", ""},
		{http.MethodDelete, room + "/hand", ""},
		{http.MethodDelete, member, ""},
		{http.MethodPost, member + "/grant", ""},
		{http.MethodDelete, member + "/grant", ""},
		{http.MethodDelete, member + "/hand", ""},
		{http.MethodPost, member + "/mute", ""},
		{http.MethodPost, member + "/cohost", ""},
		{http.MethodDelete, member + "/cohost", ""},
		{http.MethodGet, room + "/invitations", ""},
		{http.MethodPost, room + "/invitations", `{"account_id":"` + bobAccountID + `"}`},
		{http.MethodDelete, room + "/invitations/" + bobAccountID, ""},
	} {
		if res := e.send(t, req.method, req.path, "voice", req.body); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s with the voice chat off = %d, want 404", req.method, req.path, res.StatusCode)
		}
	}
}
