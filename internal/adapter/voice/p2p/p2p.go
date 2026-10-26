// SPDX-License-Identifier: AGPL-3.0-only

// Package p2p is the voice.Transport of voice = p2p (ADR-0101 §2): the browsers connect to each
// other, and the server only relays their signaling (ADR-0101 §4). It holds no media, so Open,
// SetSpeaking, Mute, Unmute, Remove, and Close do nothing; Grant returns the ICE servers, and
// Sessions the sessions whose heartbeats are alive.
package p2p

import (
	"context"
	"slices"
	"time"

	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/voice"
)

// Kind is the Grant.Kind of the p2p transport.
const Kind = "p2p"

// GrantTTL is how long a grant lasts: the STUN servers need no credentials, so the client asks
// again rarely.
const GrantTTL = 6 * time.Hour

// Options configure the Transport.
type Options struct {
	// STUNURLs are the STUN servers (stun: / stuns:) of the grants.
	STUNURLs []string
	// RelayOnly makes the clients connect through TURN only (voice_ice_transport_policy = relay).
	RelayOnly bool
	// Sessions are the live sessions of the rooms: the heartbeats.
	Sessions voice.LiveSessionSource
	Clock    kernel.Clock
}

// Transport is the p2p transport.
type Transport struct{ o Options }

var _ voice.Transport = (*Transport)(nil)

// New returns the Transport of o.
func New(o Options) *Transport { return &Transport{o: o} }

// Capabilities implements voice.Transport: nothing is enforced by a media server.
func (*Transport) Capabilities() voice.Capabilities { return voice.Capabilities{} }

// Open implements voice.Transport; there is no media path to prepare.
func (*Transport) Open(context.Context, voice.RoomRef) error { return nil }

// Grant implements voice.Transport: the ICE servers of the clients.
func (t *Transport) Grant(context.Context, voice.RoomRef, voice.Session, bool) (voice.Grant, error) {
	g := voice.Grant{Kind: Kind, RelayOnly: t.o.RelayOnly, ExpiresAt: t.o.Clock.Now().Add(GrantTTL)}
	if len(t.o.STUNURLs) > 0 {
		g.ICE = []voice.ICEServer{{URLs: slices.Clone(t.o.STUNURLs)}}
	}
	return g, nil
}

// SetSpeaking implements voice.Transport; the relay of the signaling enforces it.
func (*Transport) SetSpeaking(context.Context, voice.RoomRef, voice.Session, bool) (bool, error) {
	return false, nil
}

// Mute implements voice.Transport; the stream of the room tells the clients.
func (*Transport) Mute(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Unmute implements voice.Transport.
func (*Transport) Unmute(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Remove implements voice.Transport; the relay of the signaling stops the session's peers.
func (*Transport) Remove(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Close implements voice.Transport.
func (*Transport) Close(context.Context, voice.RoomRef) error { return nil }

// Sessions implements voice.Transport: the sessions whose heartbeats are alive.
func (t *Transport) Sessions(ctx context.Context, room voice.RoomRef) ([]voice.SessionRef, error) {
	return t.o.Sessions.LiveSessions(ctx, room.ID)
}
