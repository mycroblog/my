// SPDX-License-Identifier: AGPL-3.0-only

package testkit

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/stream"
	appvoice "github.com/mycroblog/my/internal/app/voice"
	"github.com/mycroblog/my/internal/domain/voice"
)

// The cases of VoiceRoomsContract for the room streams, the P2P signaling relay, the
// microphone, the reactions, and the grants (ADR-0100 §10, ADR-0101 §3, §4, §6).

// roomSignals subscribes to the signals of a room on the Hub of e.
func (e *roomsEnv) roomSignals(t *testing.T, room string) stream.Subscription {
	t.Helper()
	sub, err := e.hub.SubscribeRoom(t.Context(), room)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Close)
	return sub
}

// nextSignal returns the next signal of kind on sub, skipping the others.
func nextSignal(t *testing.T, sub stream.Subscription, kind stream.Kind) stream.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e := <-sub.Events():
			if e.Kind == kind {
				return e
			}
		case <-timeout:
			t.Fatalf("no %s signal", kind)
			return stream.Event{}
		}
	}
}

// openStream opens the room stream of a session.
func (e *roomsEnv) openStream(t *testing.T, account, room, session string) appvoice.RoomStream {
	t.Helper()
	st, err := e.svc.OpenStream(t.Context(), account, room, session)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	return st
}

// streamState reads the state of a room stream.
func (e *roomsEnv) streamState(t *testing.T, st appvoice.RoomStream) appvoice.StreamState {
	t.Helper()
	s, err := e.svc.StreamState(t.Context(), st, time.Now())
	if err != nil {
		t.Fatalf("StreamState: %v", err)
	}
	return s
}

func roomsStream(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, bob, carol := e.account(true), e.account(false), e.account(false)
	opened := e.open(t, host, appvoice.SettingsInput{})
	r := opened.Room.ID
	sub := e.roomSignals(t, r)
	bobs := e.join(t, bob, r).Session
	if got := nextSignal(t, sub, stream.KindVoiceRoom); got.RoomID != r {
		t.Errorf("the join signals %+v, want voice_room of %s", got, r)
	}
	carols := e.join(t, carol, r).Session

	_, err := e.svc.OpenStream(ctx, bob, r, carols)
	wantErr(t, "OpenStream of another's session", err, appvoice.ErrNotFound)
	_, err = e.svc.OpenStream(ctx, bob, r, "unknown")
	wantErr(t, "OpenStream of an unknown session", err, appvoice.ErrNotFound)
	st := e.openStream(t, bob, r, bobs)
	again := e.openStream(t, bob, r, bobs)
	if ok, err := e.svc.StreamCurrent(ctx, st); err != nil || ok {
		t.Errorf("StreamCurrent of the stream opened first = %v, %v; want false", ok, err)
	}
	if ok, err := e.svc.StreamCurrent(ctx, again); err != nil || !ok {
		t.Errorf("StreamCurrent of the stream opened again = %v, %v; want true", ok, err)
	}

	e.block(t, carol, bob)
	s := e.streamState(t, again)
	if s.End != "" || len(s.Room.Participants) != 3 || s.Sessions[bob] != bobs || s.Sessions[carol] != carols || s.Sessions[host] != opened.Session {
		t.Errorf("state = %+v, want the three participants with their sessions", s)
	}
	if !s.Blocked[carol] || s.Blocked[host] {
		t.Errorf("blocked = %v, want carol only", s.Blocked)
	}

	// bob joins again from another device: the stream of the previous session ends.
	e.join(t, bob, r)
	if s := e.streamState(t, again); s.End != appvoice.StreamReplaced || s.Reason != appvoice.ReasonReplaced {
		t.Errorf("state after joining again = %s %s, want replaced", s.End, s.Reason)
	}
	cst := e.openStream(t, carol, r, carols)
	if err := e.svc.Remove(ctx, host, r, carol); err != nil {
		t.Fatal(err)
	}
	if s := e.streamState(t, cst); s.End != appvoice.StreamRemoved || s.Reason != string(voice.RemovedFromRoom) {
		t.Errorf("state after the removal = %s %s, want removed", s.End, s.Reason)
	}
	hst := e.openStream(t, host, r, opened.Session)
	if err := e.svc.End(ctx, host, r); err != nil {
		t.Fatal(err)
	}
	if s := e.streamState(t, hst); s.End != appvoice.StreamEnded || s.Reason != string(voice.EndHost) {
		t.Errorf("state after the end = %s %s, want ended by the host", s.End, s.Reason)
	}
}

// sdpOf is a session description with one audio section of direction dir.
func sdpOf(dir string) string {
	return "v=0\r\no=- 1 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=" + dir + "\r\n"
}

func roomsSignals(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, bob, carol := e.account(true), e.account(false), e.account(false)
	opened := e.open(t, host, appvoice.SettingsInput{}) // speakers: manual
	r := opened.Room.ID
	bobs, carols := e.join(t, bob, r).Session, e.join(t, carol, r).Session
	sub := e.roomSignals(t, r)
	bst := e.openStream(t, bob, r, bobs)
	hst := e.openStream(t, host, r, opened.Session)

	offer := sdpOf("sendrecv")
	if err := e.svc.Signal(ctx, host, r, appvoice.SignalInput{Session: opened.Session, To: bobs, Kind: appvoice.SignalOffer, SDP: offer}); err != nil {
		t.Fatalf("Signal of the host: %v", err)
	}
	sig := nextSignal(t, sub, stream.KindVoiceSignal)
	if _, ok, err := e.svc.Message(ctx, hst, sig.MessageID); err != nil || ok {
		t.Errorf("the message for bob read by the host's stream = %v, %v", ok, err)
	}
	m, ok, err := e.svc.Message(ctx, bst, sig.MessageID)
	if err != nil || !ok || m.Signal == nil || m.Signal.From != opened.Session || m.Signal.SDP != offer || m.Signal.Kind != appvoice.SignalOffer {
		t.Fatalf("the message for bob = %+v, %v, %v; want the host's offer", m, ok, err)
	}

	field := func(what string, err error, want string) {
		t.Helper()
		var se *appvoice.SignalError
		if !errors.As(err, &se) || se.Field != want {
			t.Errorf("%s: err = %v, want a signal error of %s", what, err, want)
		}
	}
	// Two listeners do not connect (P1).
	field("Signal between listeners", e.svc.Signal(ctx, bob, r, appvoice.SignalInput{Session: bobs, To: carols, Kind: appvoice.SignalCandidate, Candidate: "c"}), "to")
	field("Signal to an unknown session", e.svc.Signal(ctx, bob, r, appvoice.SignalInput{Session: bobs, To: "unknown", Kind: appvoice.SignalRestart}), "to")
	field("Signal to oneself", e.svc.Signal(ctx, host, r, appvoice.SignalInput{Session: opened.Session, To: opened.Session, Kind: appvoice.SignalRestart}), "to")
	field("a sending answer of a listener", e.svc.Signal(ctx, bob, r, appvoice.SignalInput{Session: bobs, To: opened.Session, Kind: appvoice.SignalAnswer, SDP: offer}), "sdp")
	field("a video offer", e.svc.Signal(ctx, host, r, appvoice.SignalInput{
		Session: opened.Session, To: bobs, Kind: appvoice.SignalOffer, SDP: offer + "m=video 9 UDP/TLS/RTP/SAVPF 96\r\n",
	}), "sdp")
	field("an offer without SDP", e.svc.Signal(ctx, host, r, appvoice.SignalInput{Session: opened.Session, To: bobs, Kind: appvoice.SignalOffer}), "sdp")
	field("an unknown kind", e.svc.Signal(ctx, host, r, appvoice.SignalInput{Session: opened.Session, To: bobs, Kind: "hello"}), "kind")
	field("an SDP over 16 KiB", e.svc.Signal(ctx, host, r, appvoice.SignalInput{
		Session: opened.Session, To: bobs, Kind: appvoice.SignalOffer, SDP: offer + strings.Repeat("a=x\r\n", appvoice.MaxSignalSize/5),
	}), "sdp")
	err = e.svc.Signal(ctx, carol, r, appvoice.SignalInput{Session: bobs, To: opened.Session, Kind: appvoice.SignalRestart})
	wantErr(t, "Signal from another's session", err, appvoice.ErrNotFound)
	if err := e.svc.Signal(ctx, bob, r, appvoice.SignalInput{Session: bobs, To: opened.Session, Kind: appvoice.SignalAnswer, SDP: sdpOf("recvonly")}); err != nil {
		t.Errorf("a receiving answer of a listener: %v", err)
	}

	// A listener who may speak freely sends; a session sends at most SignalLimit.
	if _, err := e.svc.Update(ctx, host, r, appvoice.SettingsPatch{Speakers: ptr(voice.SpeakersEveryone)}); err != nil {
		t.Fatal(err)
	}
	var limited error
	for range appvoice.SignalLimit.Max + 1 {
		if limited = e.svc.Signal(ctx, bob, r, appvoice.SignalInput{Session: bobs, To: opened.Session, Kind: appvoice.SignalOffer, SDP: offer}); limited != nil {
			break
		}
	}
	wantErr(t, "Signal past the limit", limited, kernel.ErrRateLimited)
}

func roomsMicReactions(t *testing.T, e *roomsEnv) {
	ctx := t.Context()
	host, bob := e.account(true), e.account(false)
	opened := e.open(t, host, appvoice.SettingsInput{})
	r := opened.Room.ID
	bobs := e.join(t, bob, r).Session
	sub := e.roomSignals(t, r)

	wantErr(t, "Mic on of a listener", e.svc.Mic(ctx, bob, r, bobs, true), appvoice.ErrCannotSpeak)
	wantErr(t, "Mic of another's session", e.svc.Mic(ctx, host, r, bobs, false), appvoice.ErrNotFound)
	if err := e.svc.Mic(ctx, bob, r, bobs, false); err != nil {
		t.Fatal(err)
	}
	if got := nextSignal(t, sub, stream.KindVoiceMic); got.ParticipantID != bob || got.On {
		t.Errorf("mic = %+v, want bob's off", got)
	}

	// bob speaks, the host mutes him, and his microphone turned on again lifts the mute.
	if _, err := e.svc.Grant(ctx, host, r, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Speak(ctx, bob, r); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Mute(ctx, host, r, bob); err != nil {
		t.Fatal(err)
	}
	if got := nextSignal(t, sub, stream.KindVoiceMute); got.SessionID != bobs || got.By != host {
		t.Errorf("mute = %+v, want bob's session by the host", got)
	}
	if err := e.svc.Mic(ctx, bob, r, bobs, true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Mic(ctx, bob, r, bobs, true); err != nil {
		t.Fatal(err)
	}
	if n := e.transport.count("unmute:" + bobs); n != 1 {
		t.Errorf("unmute calls = %d, want 1 after the mute", n)
	}

	wantErr(t, "React with a word", e.svc.React(ctx, bob, r, "hello"), appvoice.ErrInvalidEmoji)
	wantErr(t, "React out of the room", e.svc.React(ctx, e.account(false), r, "👏"), appvoice.ErrNotFound)
	if err := e.svc.React(ctx, bob, r, "👏"); err != nil {
		t.Fatal(err)
	}
	if got := nextSignal(t, sub, stream.KindVoiceReaction); got.ParticipantID != bob || got.Emoji != "👏" {
		t.Errorf("reaction = %+v, want bob's 👏", got)
	}
	var limited error
	for range appvoice.ReactionLimit.Max {
		if limited = e.svc.React(ctx, bob, r, "👏"); limited != nil {
			break
		}
	}
	wantErr(t, "React past the limit", limited, kernel.ErrRateLimited)

	g, err := e.svc.RefreshGrant(ctx, bob, r, bobs)
	if err != nil || g.Token != bobs {
		t.Errorf("RefreshGrant = %+v, %v", g, err)
	}

	// A transport that has the client connect again sends a grant to the session's stream.
	e.transport.mu.Lock()
	e.transport.reconnect = true
	e.transport.mu.Unlock()
	bst := e.openStream(t, bob, r, bobs)
	if _, err := e.svc.Listen(ctx, bob, r); err != nil {
		t.Fatal(err)
	}
	for {
		sig := nextSignal(t, sub, stream.KindVoiceSignal)
		m, ok, err := e.svc.Message(ctx, bst, sig.MessageID)
		if err != nil {
			t.Fatal(err)
		}
		if ok && m.Grant != nil {
			if !m.Grant.Reconnect || m.Grant.Grant.Token != bobs {
				t.Errorf("grant = %+v, want a new grant to reconnect", m.Grant)
			}
			break
		}
	}
}
