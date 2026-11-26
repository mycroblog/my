// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// newVoiceEnv starts the real App with voice = p2p on SQLite, rooms of 6.
func newVoiceEnv(t *testing.T) *accountsEnv {
	t.Helper()
	cfg := loadConfig(t, map[string]string{
		"MY_DATABASE_URL": newDatabase(t, true), "MY_WEB_ORIGIN": webOrigin, "MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org",
		"MY_VOICE_MAX_PARTICIPANTS": "6",
	})
	app, err := platform.NewApp(t.Context(), cfg, platform.AppOptions{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return &accountsEnv{t: t, app: app, h: app.Handler(), spec: apitest.Load(t)}
}

// member registers a local user without a role.
func (e *accountsEnv) member(name string) testUser {
	e.t.Helper()
	r, err := e.app.Identity.RegisterLocalUser(e.t.Context(), identity.RegisterInput{Username: name, Password: "correct horse battery staple"})
	if err != nil {
		e.t.Fatalf("RegisterLocalUser(%s): %v", name, err)
	}
	return testUser{id: r.AccountID, token: issueTokens(e.t, e.app, r.User.ID, "").AccessToken}
}

// canSpeak is the can_speak of GET /voice/rooms/{id} for who.
func (e *accountsEnv) canSpeak(room string, who testUser, what string) bool {
	e.t.Helper()
	var got clientapi.VoiceRoom
	e.want(e.do(http.MethodGet, "/voice/rooms/"+room, who.token, nil, &got), http.StatusOK, "GET the room: "+what)
	return got.CanSpeak
}

// roleOf is the role of account in the room's participants, "" when it is not one.
func roleOf(r clientapi.VoiceRoom, account string) clientapi.VoiceRole {
	for _, p := range r.Participants {
		if p.Account.Id == account {
			return p.Role
		}
	}
	return ""
}

// untilState reads the state events of s until done holds for one, and fails after 15 seconds.
// every is called on each state read, done included.
func (s *voiceStream) untilState(what string, every func(clientapi.VoiceRoom), done func(clientapi.VoiceRoom) bool) clientapi.VoiceRoom {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if time.Now().After(deadline) {
			s.t.Fatalf("no state within 15 seconds: %s", what)
		}
		var state clientapi.VoiceRoom
		s.next("state", "VoiceRoom", &state)
		if every != nil {
			every(state)
		}
		if done(state) {
			return state
		}
	}
}

// TestVoiceCanSpeakAfterAGrant pins the per-viewer can_speak of a room whose speakers are manual
// (VoiceRoom.yaml, ADR-0100 §4, ADR-0101 §3): the host's is true; a listener's is false until the
// host grants its raised hand, then true on its GET, in the answers of its own operations, and in
// the state of its own room stream, while another listener's stays false on both; going back to
// listening keeps the grant; a revoke turns it off again on the GET and the stream.
func TestVoiceCanSpeakAfterAGrant(t *testing.T) {
	t.Parallel()
	e := newVoiceEnv(t)
	alice := e.user("alice")
	bob, carol := e.member("bob"), e.member("carol")

	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk", "speakers": "manual"}, &opened), http.StatusCreated, "open")
	if !opened.Room.CanSpeak {
		t.Error("the answer to the opening: can_speak = false for the host, want true")
	}
	room := opened.Room.Id
	path := "/voice/rooms/" + room
	var bobJoin, carolJoin clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, path+"/participation", bob.token, nil, &bobJoin), http.StatusOK, "join by bob")
	e.want(e.do(http.MethodPost, path+"/participation", carol.token, nil, &carolJoin), http.StatusOK, "join by carol")
	if bobJoin.Room.CanSpeak || carolJoin.Room.CanSpeak {
		t.Errorf("the answers to the joins: can_speak = %v (bob), %v (carol), want false", bobJoin.Room.CanSpeak, carolJoin.Room.CanSpeak)
	}
	if !e.canSpeak(room, alice, "the host") {
		t.Error("the host's can_speak = false, want true")
	}
	if e.canSpeak(room, bob, "bob before the grant") || e.canSpeak(room, carol, "carol before the grant") {
		t.Error("a listener's can_speak before any grant = true, want false")
	}

	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	bobStream := openVoiceStream(t, e.app, srv, e.spec, bob.token, room, bobJoin.Session)
	carolStream := openVoiceStream(t, e.app, srv, e.spec, carol.token, room, carolJoin.Session)
	var state clientapi.VoiceRoom
	if bobStream.next("state", "VoiceRoom", &state); state.CanSpeak {
		t.Error("bob's first state: can_speak = true, want false")
	}
	if carolStream.next("state", "VoiceRoom", &state); state.CanSpeak {
		t.Error("carol's first state: can_speak = true, want false")
	}

	var got clientapi.VoiceRoom
	e.want(e.do(http.MethodPost, path+"/hand", bob.token, nil, &got), http.StatusOK, "raise a hand")
	if got.CanSpeak {
		t.Error("the answer to bob's raised hand: can_speak = true, want false")
	}
	e.want(e.do(http.MethodPost, path+"/participants/"+bob.id+"/speaking_permission", alice.token, nil, &got), http.StatusOK, "grant")
	if !got.CanSpeak {
		t.Error("the answer to the grant, the host's: can_speak = false, want true")
	}
	if !e.canSpeak(room, bob, "bob after the grant") {
		t.Error("bob's can_speak after the grant = false, want true")
	}
	if e.canSpeak(room, carol, "carol after bob's grant") {
		t.Error("carol's can_speak after bob's grant = true, want false")
	}
	bobStream.untilState("bob's can_speak after the grant", nil, func(s clientapi.VoiceRoom) bool { return s.CanSpeak })

	e.want(e.do(http.MethodPost, path+"/speaker", bob.token, nil, &got), http.StatusOK, "speak after the grant")
	if !got.CanSpeak {
		t.Error("the answer to bob's becoming a speaker: can_speak = false, want true")
	}
	// Every state carol's stream reads up to bob's turn as a speaker is hers: not a speaker's.
	carolStream.untilState("bob as a speaker on carol's stream", func(s clientapi.VoiceRoom) {
		if s.CanSpeak {
			t.Errorf("a state of carol's stream after bob's grant: can_speak = true, want false")
		}
	}, func(s clientapi.VoiceRoom) bool { return roleOf(s, bob.id) == clientapi.VoiceRoleSpeaker })

	e.want(e.do(http.MethodDelete, path+"/speaker", bob.token, nil, &got), http.StatusOK, "listen again")
	if !got.CanSpeak {
		t.Error("the answer to bob's listening again: can_speak = false, want true (the grant stays)")
	}

	e.want(e.do(http.MethodDelete, path+"/participants/"+bob.id+"/speaking_permission", alice.token, nil, nil), http.StatusOK, "revoke")
	if e.canSpeak(room, bob, "bob after the revoke") {
		t.Error("bob's can_speak after the revoke = true, want false")
	}
	bobStream.untilState("bob's can_speak after the revoke", nil, func(s clientapi.VoiceRoom) bool { return !s.CanSpeak })
	if !e.canSpeak(room, alice, "the host after the revoke") {
		t.Error("the host's can_speak after the revoke = false, want true")
	}
}

// TestVoiceCanSpeakBySpeakers pins can_speak by the room's speakers setting and the other
// explicit permissions (VoiceRoom.yaml, ADR-0100 §4, §5, §7): bob follows the host, carol and the
// host follow each other, dave has no relation, and erin follows the host and is limited.
// everyone covers every participant but erin, followers bob and carol, mutuals and following
// carol only, manual nobody; a grant lets erin speak; an invitation as a speaker lets frank once
// he joins; a co-host may speak under manual; and a user out of the room never may.
func TestVoiceCanSpeakBySpeakers(t *testing.T) {
	t.Parallel()
	e := newVoiceEnv(t)
	alice := e.user("alice")
	bob, carol, dave, erin, frank := e.member("bob"), e.member("carol"), e.member("dave"), e.member("erin"), e.member("frank")
	e.relate("follow", bob, alice, nil)
	e.relate("follow", carol, alice, nil)
	e.relate("follow", alice, carol, nil)
	e.relate("follow", erin, alice, nil)
	e.want(e.do(http.MethodPost, "/admin/accounts/"+erin.id+"/actions", alice.token, map[string]any{"kind": "limit", "public_reason": "rules"}, nil),
		http.StatusCreated, "limit erin")
	if _, err := e.app.Runner.Drain(t.Context()); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var opened clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk", "speakers": "manual"}, &opened), http.StatusCreated, "open")
	room := opened.Room.Id
	path := "/voice/rooms/" + room
	for _, u := range []testUser{bob, carol, dave, erin} {
		e.want(e.do(http.MethodPost, path+"/participation", u.token, nil, nil), http.StatusOK, "join")
	}
	names := map[string]testUser{"bob": bob, "carol": carol, "dave": dave, "erin": erin, "frank (not in the room)": frank}
	check := func(speakers string, want map[string]bool) {
		t.Helper()
		var got clientapi.VoiceRoom
		e.want(e.do(http.MethodPatch, path, alice.token, map[string]any{"speakers": speakers}, &got), http.StatusOK, "speakers = "+speakers)
		if !got.CanSpeak {
			t.Errorf("speakers = %s: the host's can_speak in the answer = false, want true", speakers)
		}
		for name, u := range names {
			if got := e.canSpeak(room, u, name); got != want[name] {
				t.Errorf("speakers = %s: %s's can_speak = %v, want %v", speakers, name, got, want[name])
			}
		}
	}
	check("manual", map[string]bool{})
	check("everyone", map[string]bool{"bob": true, "carol": true, "dave": true})
	check("followers", map[string]bool{"bob": true, "carol": true})
	check("mutuals", map[string]bool{"carol": true})
	check("following", map[string]bool{"carol": true})
	check("manual", map[string]bool{})

	// A limited account speaks with a grant, whatever the setting.
	e.want(e.do(http.MethodPost, path+"/participants/"+erin.id+"/speaking_permission", alice.token, nil, nil), http.StatusOK, "grant erin")
	check("everyone", map[string]bool{"bob": true, "carol": true, "dave": true, "erin": true})
	check("manual", map[string]bool{"erin": true})

	// An invitation as a speaker counts once the invited user is in the room.
	e.want(e.do(http.MethodPost, path+"/invitations", alice.token, map[string]any{"account_id": frank.id, "role": "speaker"}, nil), http.StatusCreated, "invite frank")
	if e.canSpeak(room, frank, "frank invited, not in the room") {
		t.Error("frank's can_speak, invited as a speaker but not in the room = true, want false")
	}
	var joined clientapi.VoiceRoomJoin
	e.want(e.do(http.MethodPost, path+"/participation", frank.token, nil, &joined), http.StatusOK, "join by frank")
	if !joined.Room.CanSpeak {
		t.Error("the answer to frank's join, invited as a speaker: can_speak = false, want true")
	}
	if !e.canSpeak(room, frank, "frank in the room") {
		t.Error("frank's can_speak, invited as a speaker and in the room = false, want true")
	}

	// A co-host speaks under manual without a grant.
	var got clientapi.VoiceRoom
	e.want(e.do(http.MethodPost, path+"/participants/"+dave.id+"/cohost", alice.token, nil, &got), http.StatusOK, "make dave a co-host")
	if !e.canSpeak(room, dave, "dave as a co-host") {
		t.Error("a co-host's can_speak under manual = false, want true")
	}
	if e.canSpeak(room, bob, "bob under manual") {
		t.Error("bob's can_speak under manual = true, want false")
	}
}

// TestVoiceServiceCanSpeak pins (*appvoice.Service).CanSpeak (ADR-0100 §4, §16): the host of a
// scheduled room may speak before it joins, on the service and on its GET; a user out of the room
// may not; nobody may in an ended room; and the service answers ErrDisabled while voice is off.
func TestVoiceServiceCanSpeak(t *testing.T) {
	t.Parallel()
	e := newVoiceEnv(t)
	alice := e.user("alice")
	bob := e.member("bob")

	v, err := e.app.VoiceRooms.OpenScheduled(t.Context(), appvoice.ScheduledInput{
		HostAccountID: alice.id, Settings: appvoice.SettingsInput{Name: "Later", Speakers: voice.SpeakersEveryone}, Key: "can-speak",
	})
	if err != nil {
		t.Fatalf("OpenScheduled: %v", err)
	}
	if ok, err := e.app.VoiceRooms.CanSpeak(t.Context(), alice.id, v); err != nil || !ok {
		t.Errorf("CanSpeak of the host before it joins = %v, %v; want true", ok, err)
	}
	if !e.canSpeak(v.ID, alice, "the scheduled room's host before it joins") {
		t.Error("the scheduled room's host's can_speak before it joins = false, want true")
	}
	if ok, err := e.app.VoiceRooms.CanSpeak(t.Context(), bob.id, v); err != nil || ok {
		t.Errorf("CanSpeak of a user out of the room, speakers everyone = %v, %v; want false", ok, err)
	}
	if e.canSpeak(v.ID, bob, "a user out of the scheduled room") {
		t.Error("the can_speak of a user out of the room, speakers everyone = true, want false")
	}

	ended := v
	ended.Head.State = voice.StateEnded
	if ok, err := e.app.VoiceRooms.CanSpeak(t.Context(), alice.id, ended); err != nil || ok {
		t.Errorf("CanSpeak of the host of an ended room = %v, %v; want false", ok, err)
	}

	off := newAccountsEnv(t)
	if _, err := off.app.VoiceRooms.CanSpeak(t.Context(), alice.id, v); !errors.Is(err, appvoice.ErrDisabled) {
		t.Errorf("CanSpeak with voice = off: %v, want ErrDisabled", err)
	}
}
