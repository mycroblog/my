// SPDX-License-Identifier: AGPL-3.0-only

// Package p2p is the voice.Transport of voice = p2p (ADR-0101 §2): the browsers connect to each
// other, and the server only relays their signaling (ADR-0101 §4). It holds no media, so Open,
// SetSpeaking, Mute, Unmute, and Close do nothing; Grant returns the ICE servers with the
// short-lived credentials of the TURN source (ADR-0101 §5), Remove revokes those credentials,
// and Sessions returns the sessions whose heartbeats are alive.
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

// GrantTTL is how long a grant lasts when Options.TTL is not set: the default of
// voice_turn_credential_ttl.
const GrantTTL = 6 * time.Hour

// TURNCredentials counts the TURN credentials that the grants asked for (ADR-0101 §9), by the
// TURN source and by whether they were issued, reused from the cache of the session, or failed.
var TURNCredentials = kernel.Instrument{
	Name: "my.voice.turn.credentials", Unit: "{credential}",
	Description: "TURN credentials of the voice chat grants, by TURN source and outcome.",
	LabelKeys:   []string{"my.voice.turn", "my.outcome"},
}

// The outcomes of TURNCredentials.
const (
	OutcomeIssued = "issued"
	OutcomeReused = "reused"
	OutcomeFailed = "failed"
)

// TURN issues the short-lived TURN credentials of the sessions (voice_turn, ADR-0101 §5).
type TURN interface {
	// Name is the voice_turn value of the source: "coturn" or "cloudflare".
	Name() string
	// Credentials returns the TURN servers of a session with credentials that last at most
	// ttl from now.
	Credentials(ctx context.Context, session voice.Session, ttl time.Duration) (Credentials, error)
	// Revoke keeps the credentials of a session from being used again, after the session
	// left or was removed. Revoking a session without credentials is not an error.
	Revoke(ctx context.Context, session voice.Session) error
}

// Credentials are the TURN servers of a session.
type Credentials struct {
	Servers   []voice.ICEServer
	ExpiresAt time.Time
	// Reused reports credentials issued earlier for the same session.
	Reused bool
}

// Options configure the Transport.
type Options struct {
	// STUNURLs are the STUN servers (stun: / stuns:) of the grants.
	STUNURLs []string
	// TURN issues the TURN credentials of the grants; nil for voice_turn = none.
	TURN TURN
	// TTL is how long a grant and its TURN credentials last (voice_turn_credential_ttl);
	// GrantTTL when 0.
	TTL time.Duration
	// RelayOnly makes the clients connect through TURN only (voice_ice_transport_policy = relay).
	RelayOnly bool
	// Sessions are the live sessions of the rooms: the heartbeats.
	Sessions voice.LiveSessionSource
	Clock    kernel.Clock
	// Meter records TURNCredentials; nil records nothing.
	Meter kernel.Meter
}

// Transport is the p2p transport.
type Transport struct {
	o           Options
	credentials kernel.Counter
}

var _ voice.Transport = (*Transport)(nil)

// New returns the Transport of o.
func New(o Options) *Transport {
	if o.TTL <= 0 {
		o.TTL = GrantTTL
	}
	t := &Transport{o: o}
	if o.TURN != nil {
		t.credentials = kernel.MeterOrNop(o.Meter).Counter(TURNCredentials)
	}
	return t
}

// Capabilities implements voice.Transport: nothing is enforced by a media server.
func (*Transport) Capabilities() voice.Capabilities { return voice.Capabilities{} }

// Open implements voice.Transport; there is no media path to prepare.
func (*Transport) Open(context.Context, voice.RoomRef) error { return nil }

// Grant implements voice.Transport: the STUN servers and the TURN servers with the session's
// credentials. A grant expires with its TURN credentials, so that the client asks for new
// ones before they stop working.
func (t *Transport) Grant(ctx context.Context, _ voice.RoomRef, session voice.Session, _ bool) (voice.Grant, error) {
	g := voice.Grant{Kind: Kind, RelayOnly: t.o.RelayOnly, ExpiresAt: t.o.Clock.Now().Add(t.o.TTL)}
	if len(t.o.STUNURLs) > 0 {
		g.ICE = []voice.ICEServer{{URLs: slices.Clone(t.o.STUNURLs)}}
	}
	if t.o.TURN == nil {
		return g, nil
	}
	c, err := t.o.TURN.Credentials(ctx, session, t.o.TTL)
	if err != nil {
		t.count(ctx, OutcomeFailed)
		return voice.Grant{}, err
	}
	if c.Reused {
		t.count(ctx, OutcomeReused)
	} else {
		t.count(ctx, OutcomeIssued)
	}
	g.ICE = append(g.ICE, c.Servers...)
	if c.ExpiresAt.Before(g.ExpiresAt) {
		g.ExpiresAt = c.ExpiresAt
	}
	return g, nil
}

func (t *Transport) count(ctx context.Context, outcome string) {
	t.credentials.Add(ctx, 1, kernel.NewLabels("my.voice.turn", t.o.TURN.Name(), "my.outcome", outcome))
}

// SetSpeaking implements voice.Transport; the relay of the signaling enforces it.
func (*Transport) SetSpeaking(context.Context, voice.RoomRef, voice.Session, bool) (bool, error) {
	return false, nil
}

// Mute implements voice.Transport; the stream of the room tells the clients.
func (*Transport) Mute(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Unmute implements voice.Transport.
func (*Transport) Unmute(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Remove implements voice.Transport: the relay of the signaling stops the session's peers, and
// the TURN credentials of the session are revoked so that they are not used again (ADR-0101 §5).
// Revoking does not cut a connection already made; leaving the signaling does.
func (t *Transport) Remove(ctx context.Context, _ voice.RoomRef, session voice.Session) error {
	if t.o.TURN == nil {
		return nil
	}
	return t.o.TURN.Revoke(ctx, session)
}

// Close implements voice.Transport.
func (*Transport) Close(context.Context, voice.RoomRef) error { return nil }

// Sessions implements voice.Transport: the sessions whose heartbeats are alive.
func (t *Transport) Sessions(ctx context.Context, room voice.RoomRef) ([]voice.SessionRef, error) {
	return t.o.Sessions.LiveSessions(ctx, room.ID)
}
