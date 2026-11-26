// SPDX-License-Identifier: AGPL-3.0-only

// Package sfu is the voice.Transport of voice = sfu (ADR-0101 §13) and the registry of its media
// nodes (ADR-0101 §14). The transport places the rooms on the nodes and gives the grants: the
// URL of the room's node and a single-use ticket for it, with the TURN servers of voice_turn
// (ADR-0101 §5, §15). The nodes themselves (package node) follow the room state, so SetSpeaking,
// Mute, Unmute and Remove change nothing on them. Nothing here talks to a node: the registry in
// the KeyValueStore is all that the web processes and the nodes share.
package sfu

import (
	"context"
	"time"

	"github.com/mycroblog/my/internal/adapter/voice/p2p"
	"github.com/mycroblog/my/internal/app/kernel"
	"github.com/mycroblog/my/internal/app/voice"
)

// Kind is the Grant.Kind of the sfu transport.
const Kind = "sfu"

// Options configure the Transport.
type Options struct {
	Registry *Registry
	// TURN issues the TURN credentials of the grants; nil for voice_turn = none.
	TURN p2p.TURN
	// TTL is how long a grant and its TURN credentials last (voice_turn_credential_ttl);
	// p2p.GrantTTL when 0.
	TTL time.Duration
	// MaxParticipants is what a room reserves when the caller does not know its maximum
	// (voice_max_participants).
	MaxParticipants int
	Clock           kernel.Clock
	// Meter records p2p.TURNCredentials; nil records nothing.
	Meter kernel.Meter
}

// Transport is the sfu transport.
type Transport struct {
	o           Options
	credentials kernel.Counter
}

var (
	_ voice.Transport       = (*Transport)(nil)
	_ voice.MaximumReserver = (*Transport)(nil)
)

// New returns the Transport of o.
func New(o Options) *Transport {
	if o.TTL <= 0 {
		o.TTL = p2p.GrantTTL
	}
	t := &Transport{o: o}
	if o.TURN != nil {
		t.credentials = kernel.MeterOrNop(o.Meter).Counter(p2p.TURNCredentials)
	}
	return t
}

// Capabilities implements voice.Transport: the nodes enforce who may speak.
func (*Transport) Capabilities() voice.Capabilities {
	return voice.Capabilities{ServerEnforced: true}
}

// max returns what a room reserves.
func (t *Transport) max(room voice.RoomRef) int {
	if room.MaxParticipants > 0 {
		return room.MaxParticipants
	}
	return max(t.o.MaxParticipants, 1)
}

// Open implements voice.Transport: it places the room on a node (Registry.Place), and fails
// with ErrNoCapacity when no node has room for it.
func (t *Transport) Open(ctx context.Context, room voice.RoomRef) error {
	_, err := t.o.Registry.Place(ctx, room.ID, t.max(room), "")
	return err
}

// Grant implements voice.Transport: the URL of the room's node, placed again when its node is
// gone or draining, and a ticket of the session for that node, with the TURN servers of the
// session. It fails with ErrNoCapacity when no node has room for the room.
func (t *Transport) Grant(ctx context.Context, room voice.RoomRef, session voice.Session, _ bool) (voice.Grant, error) {
	n, err := t.o.Registry.Place(ctx, room.ID, t.max(room), "")
	if err != nil {
		return voice.Grant{}, err
	}
	token, err := t.o.Registry.Issue(ctx, Ticket{Room: room.ID, Session: session.ID, Account: session.AccountID, Node: n.ID})
	if err != nil {
		return voice.Grant{}, err
	}
	g := voice.Grant{Kind: Kind, URL: n.URL, Token: token, ExpiresAt: t.o.Clock.Now().Add(t.o.TTL)}
	if t.o.TURN == nil {
		return g, nil
	}
	c, err := t.o.TURN.Credentials(ctx, session, t.o.TTL)
	if err != nil {
		t.count(ctx, p2p.OutcomeFailed)
		return voice.Grant{}, err
	}
	if c.Reused {
		t.count(ctx, p2p.OutcomeReused)
	} else {
		t.count(ctx, p2p.OutcomeIssued)
	}
	g.ICE = c.Servers
	if c.ExpiresAt.Before(g.ExpiresAt) {
		g.ExpiresAt = c.ExpiresAt
	}
	return g, nil
}

func (t *Transport) count(ctx context.Context, outcome string) {
	t.credentials.Add(ctx, 1, kernel.NewLabels("my.voice.turn", t.o.TURN.Name(), "my.outcome", outcome))
}

// ReserveMaximum implements voice.MaximumReserver: the room's node reserves its larger maximum, or
// the call fails with ErrNoCapacity and the maximum does not change (Registry.Resize).
func (t *Transport) ReserveMaximum(ctx context.Context, room voice.RoomRef) error {
	return t.o.Registry.Resize(ctx, room.ID, t.max(room))
}

// SetSpeaking implements voice.Transport; the node reads the room again and applies it.
func (*Transport) SetSpeaking(context.Context, voice.RoomRef, voice.Session, bool) error {
	return nil
}

// Mute implements voice.Transport; the node reads the room again and applies it.
func (*Transport) Mute(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Unmute implements voice.Transport; the node reads the room again and applies it.
func (*Transport) Unmute(context.Context, voice.RoomRef, voice.Session) error { return nil }

// Remove implements voice.Transport: the node reads the room again and closes the session's
// connection; the TURN credentials of the session are revoked so that they are not used again
// (ADR-0101 §5).
func (t *Transport) Remove(ctx context.Context, _ voice.RoomRef, session voice.Session) error {
	if t.o.TURN == nil {
		return nil
	}
	return t.o.TURN.Revoke(ctx, session)
}

// Close implements voice.Transport: the room leaves its node (Registry.Release).
func (t *Transport) Close(ctx context.Context, room voice.RoomRef) error {
	return t.o.Registry.Release(ctx, room.ID)
}

// Sessions implements voice.Transport: the sessions that the room's node reported connected, none
// when the node is gone.
func (t *Transport) Sessions(ctx context.Context, room voice.RoomRef) ([]voice.SessionRef, error) {
	a, ok, err := t.o.Registry.Assignment(ctx, room.ID)
	if err != nil || !ok {
		return nil, err
	}
	if _, live, err := t.o.Registry.Lease(ctx, a.Node); err != nil || !live {
		return nil, err
	}
	out := make([]voice.SessionRef, 0, len(a.Sessions))
	for _, s := range a.Sessions {
		out = append(out, voice.SessionRef{ID: s.ID, AccountID: s.AccountID})
	}
	return out, nil
}
