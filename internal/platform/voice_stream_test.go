// SPDX-License-Identifier: AGPL-3.0-only

package platform_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/identity"
	"github.com/mycroblog/my/internal/app/stream"
	"github.com/mycroblog/my/internal/interface/clientapi"
	"github.com/mycroblog/my/internal/interface/clientapi/apitest"
	"github.com/mycroblog/my/internal/platform"
)

// TestVoiceStreamsAcrossProcesses runs two web Apps on one database and one broker, as two
// processes (ADR-0101 §3, §4, §6): alice hosts a room through one and bob joins it through the
// other, each with a room stream of their own process. A raised hand reaches the other
// process's stream as state, and a signal, a reaction, the microphone, and a mute, sent
// through one process, reach the stream of the other; every event matches the spec. On the
// in-memory Bus and on stream = postgres (pgmem in pull requests).
func TestVoiceStreamsAcrossProcesses(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		env  map[string]string
		bus  func() (a, b stream.Bus)
	}{
		{name: "memory", bus: func() (stream.Bus, stream.Bus) {
			b := stream.NewMemoryBroker()
			return b.Bus(), b.Bus()
		}},
		{name: "postgres", env: map[string]string{"MY_STREAM": "postgres"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{
				"MY_DB": "postgres", "MY_DATABASE_URL": newPostgresDatabase(t), "MY_WEB_ORIGIN": webOrigin,
				"MY_VOICE": "p2p", "MY_VOICE_STUN_URLS": "stun:stun.example.org",
			}
			for k, v := range tt.env {
				env[k] = v
			}
			var busA, busB stream.Bus
			if tt.bus != nil {
				busA, busB = tt.bus()
			}
			checkVoiceStreamsAcrossProcesses(t, newRoleApp(t, env, "web", busA), newRoleApp(t, env, "web", busB))
		})
	}
}

// voiceStream is an open room stream of a test.
type voiceStream struct {
	t      *testing.T
	spec   *apitest.Spec
	events <-chan sseEvent
}

// openVoiceStream opens the room stream of session on the server srv of app.
func openVoiceStream(t *testing.T, app *platform.App, srv *httptest.Server, spec *apitest.Spec, token, room, session string) *voiceStream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	before := app.Streams.Len()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+clientapi.BasePath+"/voice/rooms/"+room+"/stream?session="+url.QueryEscape(session), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET the room stream: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("GET the room stream = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	deadline := time.Now().Add(15 * time.Second)
	for app.Streams.Len() <= before {
		if time.Now().After(deadline) {
			t.Fatal("the room stream did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return &voiceStream{t: t, spec: spec, events: readEvents(bufio.NewReader(res.Body))}
}

// next decodes the next event name, skipping the events of other names, after validating its
// data against the schema of the spec.
func (s *voiceStream) next(name, schema string, v any) {
	s.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-s.events:
			if !ok {
				s.t.Fatalf("the stream ended before a %s event", name)
			}
			if ev.name != name {
				continue
			}
			s.spec.ValidateSchema(s.t, schema, ev.data)
			if err := json.Unmarshal(ev.data, v); err != nil {
				s.t.Fatal(err)
			}
			return
		case <-timeout:
			s.t.Fatalf("no %s event within 10 seconds", name)
		}
	}
}

func checkVoiceStreamsAcrossProcesses(t *testing.T, a, b *platform.App) {
	spec := apitest.Load(t)
	ea := &accountsEnv{t: t, app: a, h: a.Handler(), spec: spec}
	eb := &accountsEnv{t: t, app: b, h: b.Handler(), spec: spec}
	alice := ea.user("alice")
	r, err := a.Identity.RegisterLocalUser(t.Context(), identity.RegisterInput{Username: "bob", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatal(err)
	}
	bob := issueTokens(t, a, r.User.ID, "").AccessToken

	var opened, joined clientapi.VoiceRoomJoin
	ea.want(ea.do(http.MethodPost, "/voice/rooms", alice.token, map[string]any{"name": "Talk"}, &opened), http.StatusCreated, "open through a")
	room := opened.Room.Id
	path := "/voice/rooms/" + room
	eb.want(eb.do(http.MethodPost, path+"/participation", bob, nil, &joined), http.StatusOK, "join through b")

	srvA, srvB := httptest.NewServer(ea.h), httptest.NewServer(eb.h)
	t.Cleanup(srvA.Close)
	t.Cleanup(srvB.Close)
	streamA := openVoiceStream(t, a, srvA, spec, alice.token, room, opened.Session)
	streamB := openVoiceStream(t, b, srvB, spec, bob, room, joined.Session)

	var state clientapi.VoiceRoom
	streamB.next("state", "VoiceRoom", &state)
	sessions := map[string]string{}
	for _, p := range state.Participants {
		if p.Session != nil {
			sessions[p.Account.Id] = *p.Session
		}
	}
	if sessions[alice.id] != opened.Session || sessions[r.AccountID] != joined.Session {
		t.Errorf("the sessions of the first state = %v, want alice's %s and bob's %s", sessions, opened.Session, joined.Session)
	}
	streamA.next("state", "VoiceRoom", &state)

	// bob raises a hand through b; alice's stream in a reads the room again.
	eb.want(eb.do(http.MethodPost, path+"/hand", bob, nil, nil), http.StatusOK, "raise a hand through b")
	deadline := time.Now().Add(10 * time.Second)
	for raised := false; !raised; {
		if time.Now().After(deadline) {
			t.Fatal("alice's stream did not see bob's raised hand")
		}
		streamA.next("state", "VoiceRoom", &state)
		for _, p := range state.Participants {
			raised = raised || (p.Account.Id == r.AccountID && p.HandRaised)
		}
	}

	// alice, the host, offers to bob through a; bob's stream in b receives the signal.
	offer := "v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n"
	ea.want(ea.do(http.MethodPost, path+"/signals", alice.token, map[string]any{
		"session": opened.Session, "to": joined.Session, "kind": "offer", "sdp": offer,
	}, nil), http.StatusNoContent, "signal through a")
	var sig clientapi.VoiceStreamSignal
	streamB.next("signal", "VoiceStreamSignal", &sig)
	if sig.From != opened.Session || sig.Kind != clientapi.VoiceSignalKindOffer || sig.Sdp == nil || *sig.Sdp != offer {
		t.Errorf("signal = %+v, want alice's offer", sig)
	}
	// bob, a listener, may answer only to receive.
	answer := strings.Replace(offer, "a=sendrecv", "a=recvonly", 1)
	ea.want(eb.do(http.MethodPost, path+"/signals", bob, map[string]any{
		"session": joined.Session, "to": opened.Session, "kind": "answer", "sdp": offer,
	}, nil), http.StatusUnprocessableEntity, "a sending answer of a listener")
	eb.want(eb.do(http.MethodPost, path+"/signals", bob, map[string]any{
		"session": joined.Session, "to": opened.Session, "kind": "answer", "sdp": answer,
	}, nil), http.StatusNoContent, "a receiving answer of a listener")
	streamA.next("signal", "VoiceStreamSignal", &sig)
	if sig.From != joined.Session || sig.Kind != clientapi.VoiceSignalKindAnswer {
		t.Errorf("signal = %+v, want bob's answer", sig)
	}

	// bob reacts through b; alice's stream in a receives it.
	eb.want(eb.do(http.MethodPost, path+"/reactions", bob, map[string]any{"emoji": "👏"}, nil), http.StatusNoContent, "react through b")
	var reaction clientapi.VoiceStreamReaction
	streamA.next("reaction", "VoiceStreamReaction", &reaction)
	if reaction.Participant != r.AccountID || reaction.Emoji != "👏" {
		t.Errorf("reaction = %+v, want bob's 👏", reaction)
	}

	// alice turns her microphone off through a; bob's stream in b hears of it.
	ea.want(ea.do(http.MethodPost, path+"/mic", alice.token, map[string]any{"session": opened.Session, "on": false}, nil), http.StatusNoContent, "mic through a")
	var mic clientapi.VoiceStreamMic
	streamB.next("mic", "VoiceStreamMic", &mic)
	if mic.Participant != alice.id || mic.On {
		t.Errorf("mic = %+v, want alice's off", mic)
	}

	// alice mutes bob through a, whose voice.apply tells bob's stream in b.
	ea.want(ea.do(http.MethodPost, path+"/participants/"+r.AccountID+"/mute", alice.token, nil, nil), http.StatusNoContent, "mute through a")
	if _, err := a.Runner.Drain(t.Context()); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	var muted clientapi.VoiceStreamMuted
	streamB.next("muted", "VoiceStreamMuted", &muted)
	if muted.By != alice.id {
		t.Errorf("muted = %+v, want by alice", muted)
	}

	// bob leaves through b; his stream ends with the reason.
	eb.want(eb.do(http.MethodDelete, path+"/participation", bob, nil, nil), http.StatusNoContent, "leave through b")
	var end clientapi.VoiceStreamEnd
	streamB.next("removed", "VoiceStreamEnd", &end)
	if end.Reason != "left" {
		t.Errorf("removed = %+v, want left", end)
	}
}
