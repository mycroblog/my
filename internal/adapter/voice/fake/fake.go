// SPDX-License-Identifier: AGPL-3.0-only

// Package fake is an in-memory voice.Transport for tests (ADR-0101 §2): a media path that
// enforces the server side as an SFU does. A session is on the media path once Connect saw it,
// until Remove or Close. It records every call, passes testkit.VoiceTransportContract, and is
// safe for concurrent use.
package fake

import (
	"context"
	"sync"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/voice"
)

// Kind is the Grant.Kind of the fake transport.
const Kind = "fake"

// GrantTTL is how long a grant of the fake transport lasts.
const GrantTTL = time.Minute

// Call is one call of the Transport.
type Call struct {
	Op       string // "open", "grant", "set_speaking", "mute", "unmute", "remove", "close", "sessions"
	Room     string
	Session  string
	CanSpeak bool
}

// Transport is the fake transport.
type Transport struct {
	clock kernel.Clock

	mu    sync.Mutex
	open  map[string]bool
	media map[string]map[string]voice.SessionRef // room -> session ID -> session
	muted map[string]bool                        // session ID
	calls []Call
}

var _ voice.Transport = (*Transport)(nil)

// New returns a Transport whose grants expire by clock.
func New(clock kernel.Clock) *Transport {
	return &Transport{clock: clock, open: map[string]bool{}, media: map[string]map[string]voice.SessionRef{}, muted: map[string]bool{}}
}

func (t *Transport) record(c Call) {
	t.calls = append(t.calls, c)
}

// Calls returns the calls so far, in order.
func (t *Transport) Calls() []Call {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Call(nil), t.calls...)
}

// Connect puts a session on the media path of a room, as a client that connected with its
// grant.
func (t *Transport) Connect(room voice.RoomRef, s voice.Session) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.media[room.ID] == nil {
		t.media[room.ID] = map[string]voice.SessionRef{}
	}
	t.media[room.ID][s.ID] = voice.SessionRef(s)
}

// IsOpen reports whether the media path of a room is open.
func (t *Transport) IsOpen(room string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open[room]
}

// Muted reports whether a session is muted.
func (t *Transport) Muted(session string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.muted[session]
}

// Capabilities implements voice.Transport: the server enforces speaking.
func (t *Transport) Capabilities() voice.Capabilities {
	return voice.Capabilities{ServerEnforced: true}
}

// Open implements voice.Transport.
func (t *Transport) Open(_ context.Context, room voice.RoomRef) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "open", Room: room.ID})
	t.open[room.ID] = true
	return nil
}

// Grant implements voice.Transport.
func (t *Transport) Grant(_ context.Context, room voice.RoomRef, s voice.Session, canSpeak bool) (voice.Grant, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "grant", Room: room.ID, Session: s.ID, CanSpeak: canSpeak})
	speak := "false"
	if canSpeak {
		speak = "true"
	}
	return voice.Grant{
		Kind: Kind, URL: "wss://media.invalid/" + room.ID, Token: s.ID, Options: map[string]string{"can_speak": speak},
		ExpiresAt: t.clock.Now().Add(GrantTTL),
	}, nil
}

// SetSpeaking implements voice.Transport; the change applies without reconnecting.
func (t *Transport) SetSpeaking(_ context.Context, room voice.RoomRef, s voice.Session, canSpeak bool) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "set_speaking", Room: room.ID, Session: s.ID, CanSpeak: canSpeak})
	return false, nil
}

// Mute implements voice.Transport.
func (t *Transport) Mute(_ context.Context, room voice.RoomRef, s voice.Session) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "mute", Room: room.ID, Session: s.ID})
	t.muted[s.ID] = true
	return nil
}

// Unmute implements voice.Transport.
func (t *Transport) Unmute(_ context.Context, room voice.RoomRef, s voice.Session) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "unmute", Room: room.ID, Session: s.ID})
	delete(t.muted, s.ID)
	return nil
}

// Remove implements voice.Transport.
func (t *Transport) Remove(_ context.Context, room voice.RoomRef, s voice.Session) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "remove", Room: room.ID, Session: s.ID})
	delete(t.media[room.ID], s.ID)
	return nil
}

// Close implements voice.Transport.
func (t *Transport) Close(_ context.Context, room voice.RoomRef) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "close", Room: room.ID})
	delete(t.open, room.ID)
	delete(t.media, room.ID)
	return nil
}

// Sessions implements voice.Transport.
func (t *Transport) Sessions(_ context.Context, room voice.RoomRef) ([]voice.SessionRef, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.record(Call{Op: "sessions", Room: room.ID})
	out := make([]voice.SessionRef, 0, len(t.media[room.ID]))
	for _, s := range t.media[room.ID] {
		out = append(out, s)
	}
	return out, nil
}
