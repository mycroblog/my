// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// This file holds the rules of the relays the instance joins (ADR-0006, federation.md §13): the
// registration, the events of its membership and the state they lead to, and the activities the
// instance actor sends to a relay.

// RelayProtocol is how a relay is joined (relays.protocol).
type RelayProtocol string

// Relay protocols.
const (
	// RelayMastodon is a relay joined at its inbox: the instance actor follows the Public
	// collection (pub-relay, Activity-Relay's subscribers).
	RelayMastodon RelayProtocol = "mastodon"
	// RelayLitePub is a relay joined at its actor: the instance actor follows the relay actor
	// (Pleroma and Akkoma's relays, Activity-Relay's followers).
	RelayLitePub RelayProtocol = "litepub"
)

// Valid reports whether p is one of the relay protocols.
func (p RelayProtocol) Valid() bool { return p == RelayMastodon || p == RelayLitePub }

// RelayState is where the membership of a relay stands (relay_states.state).
type RelayState string

// Relay states.
const (
	// RelayPending is a relay whose Follow is sent and not answered yet.
	RelayPending RelayState = "pending"
	// RelayAccepted is a relay that accepted the Follow: posts go to it and come from it.
	RelayAccepted RelayState = "accepted"
	// RelayRejected is a relay that rejected the Follow, or ended the membership.
	RelayRejected RelayState = "rejected"
	// RelayLeft is a relay that the instance left with an Undo of its Follow.
	RelayLeft RelayState = "left"
)

// Valid reports whether s is one of the relay states.
func (s RelayState) Valid() bool {
	switch s {
	case RelayPending, RelayAccepted, RelayRejected, RelayLeft:
		return true
	}
	return false
}

// Joinable reports whether a relay in state s may be followed again: one that rejected the
// Follow or that the instance left. A relay without a state is joined by its first Follow.
func (s RelayState) Joinable() bool { return s == RelayRejected || s == RelayLeft }

// Removable reports whether the registration of a relay in state s may be deleted: only one that
// the instance is no member of.
func (s RelayState) Removable() bool { return s == RelayRejected || s == RelayLeft }

// RelayEventKind is a change of a relay's membership (relay_events.kind).
type RelayEventKind string

// Relay event kinds.
const (
	// RelayFollowSent: the instance actor sent a Follow to the relay.
	RelayFollowSent RelayEventKind = "follow_sent"
	// RelayAcceptReceived: the relay accepted the Follow.
	RelayAcceptReceived RelayEventKind = "accepted"
	// RelayRejectReceived: the relay rejected the Follow, or ended the membership.
	RelayRejectReceived RelayEventKind = "rejected"
	// RelayLeftSent: the instance actor sent an Undo of the Follow.
	RelayLeftSent RelayEventKind = "left"
)

// Valid reports whether k is one of the relay event kinds.
func (k RelayEventKind) Valid() bool {
	switch k {
	case RelayFollowSent, RelayAcceptReceived, RelayRejectReceived, RelayLeftSent:
		return true
	}
	return false
}

// State returns the state that an event of kind k leads to.
func (k RelayEventKind) State() RelayState {
	switch k {
	case RelayFollowSent:
		return RelayPending
	case RelayAcceptReceived:
		return RelayAccepted
	case RelayRejectReceived:
		return RelayRejected
	case RelayLeftSent:
		return RelayLeft
	}
	return ""
}

// MaxRelayURL is the longest URL a relay is registered with, in bytes.
const MaxRelayURL = 2048

// Relay is a relay the operator registered (relays, a resource). URL is what the operator
// entered: the relay's inbox for RelayMastodon, its actor for RelayLitePub. InboxURL is where
// the instance delivers to: URL itself for RelayMastodon, the relay actor's shared inbox, or its
// inbox without one, for RelayLitePub.
type Relay struct {
	ID       string
	Protocol RelayProtocol
	URL      IRI
	InboxURL IRI
	// Receive says whether the posts that the relay sends are taken in; false only sends.
	Receive bool
	// CreatedByUserID is the administrator who registered it, "" for the CLI.
	CreatedByUserID string
	CreatedAt       time.Time
}

// ErrInvalidRelay reports a relay or an event of one with a missing or invalid field.
var ErrInvalidRelay = errors.New("federation: invalid relay")

// Validate checks the invariants of a new registration: a known protocol and https URLs within
// MaxRelayURL bytes.
func (r Relay) Validate() error {
	switch {
	case r.ID == "" || r.CreatedAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidRelay)
	case !r.Protocol.Valid():
		return fmt.Errorf("%w: protocol %q", ErrInvalidRelay, r.Protocol)
	case !validRelayURL(r.URL) || !validRelayURL(r.InboxURL):
		return fmt.Errorf("%w: the URLs must be https and at most %d bytes", ErrInvalidRelay, MaxRelayURL)
	}
	return nil
}

func validRelayURL(u IRI) bool {
	return len(u) <= MaxRelayURL && strings.HasPrefix(string(u), "https://") && u.Host() != ""
}

// RelayEvent is one change of a relay's membership (relay_events, an event).
type RelayEvent struct {
	ID      string
	RelayID string
	Kind    RelayEventKind
	// FollowActivityURI is the Follow that a follow_sent sent, and the Follow that the other
	// kinds are about.
	FollowActivityURI IRI
	// RelayActorURI is the actor that accepted the Follow, for an accepted event; "" otherwise.
	RelayActorURI IRI
	// ActorUserID is the administrator who sent the Follow or the Undo, "" for the CLI and for
	// what the relay sent.
	ActorUserID string
	OccurredAt  time.Time
}

// Validate checks the invariants of an event: a known kind, its Follow, and the relay actor
// exactly for an accepted event.
func (e RelayEvent) Validate() error {
	switch {
	case e.ID == "" || e.RelayID == "" || e.FollowActivityURI == "" || e.OccurredAt.IsZero():
		return fmt.Errorf("%w: a field of the event is empty", ErrInvalidRelay)
	case !e.Kind.Valid():
		return fmt.Errorf("%w: event kind %q", ErrInvalidRelay, e.Kind)
	case (e.Kind == RelayAcceptReceived) != (e.RelayActorURI != ""):
		return fmt.Errorf("%w: only an accepted event names the relay actor", ErrInvalidRelay)
	}
	return nil
}

// RelayStatus is the current state of a relay's membership (relay_states, a projection of
// relay_events).
type RelayStatus struct {
	RelayID string
	State   RelayState
	// FollowActivityURI is the Follow of the current membership.
	FollowActivityURI IRI
	// RelayActorURI is the actor that accepted it, "" unless the relay is accepted.
	RelayActorURI IRI
	Since         time.Time
}

// Errors of the transitions of a relay's membership.
var (
	// ErrRelayNotJoinable reports a Follow of a relay that is pending or accepted.
	ErrRelayNotJoinable = errors.New("federation: the relay is joined or being joined")
	// ErrRelayNotJoined reports an Undo of a relay that is neither pending nor accepted.
	ErrRelayNotJoined = errors.New("federation: the relay is not joined")
	// ErrRelayNotAnswerable reports an Accept or Reject that is not about the relay's current
	// Follow, or that comes when the relay is not waiting for it.
	ErrRelayNotAnswerable = errors.New("federation: the answer is not about the relay's Follow")
)

// Next returns the status that e leads s to (the projection's rule). zero is a relay without a
// status yet, which only a follow_sent starts.
//
//   - follow_sent: from no status, rejected, or left, to pending with its Follow.
//   - accepted: from pending, with the same Follow, to accepted with its actor.
//   - rejected: from pending or accepted, with the same Follow, to rejected.
//   - left: from pending or accepted, with the same Follow, to left.
func (s RelayStatus) Next(e RelayEvent) (RelayStatus, error) {
	if err := e.Validate(); err != nil {
		return RelayStatus{}, err
	}
	joined := s.State == RelayPending || s.State == RelayAccepted
	switch e.Kind {
	case RelayFollowSent:
		if s.State != "" && !s.State.Joinable() {
			return RelayStatus{}, ErrRelayNotJoinable
		}
	case RelayAcceptReceived:
		if s.State != RelayPending || s.FollowActivityURI != e.FollowActivityURI {
			return RelayStatus{}, ErrRelayNotAnswerable
		}
	case RelayRejectReceived:
		if !joined || s.FollowActivityURI != e.FollowActivityURI {
			return RelayStatus{}, ErrRelayNotAnswerable
		}
	case RelayLeftSent:
		if !joined || s.FollowActivityURI != e.FollowActivityURI {
			return RelayStatus{}, ErrRelayNotJoined
		}
	}
	return RelayStatus{
		RelayID: e.RelayID, State: e.Kind.State(), FollowActivityURI: e.FollowActivityURI,
		RelayActorURI: e.RelayActorURI, Since: e.OccurredAt,
	}, nil
}

// RelayFollowObject returns the object of the Follow that joins a relay: the Public collection
// for RelayMastodon, as Mastodon and Misskey follow a relay, and the relay actor (the registered
// URL) for RelayLitePub.
func RelayFollowObject(r Relay) IRI {
	if r.Protocol == RelayLitePub {
		return r.URL
	}
	return PublicIRI
}

// RelayFollowDocument returns the Follow of the instance actor that joins relay r, with the id
// follow. The Follow of a Mastodon-style relay is addressed to no one, as Mastodon sends it:
// Activity-Relay takes an activity addressed to the Public collection for one to relay, and
// ignores a Follow among them. The Follow of a LitePub relay is addressed to its actor.
func RelayFollowDocument(follow, instanceActor IRI, r Relay, at time.Time) ActivityDocument {
	object := RelayFollowObject(r)
	var aud Audience
	if r.Protocol == RelayLitePub {
		aud.To = []IRI{object}
	}
	return NewActivityDocument(follow, ActivityFollow, instanceActor, at, aud, object)
}

// RelayUndoDocument returns the Undo of the instance actor with the id undo that leaves relay r,
// embedding the Follow follow that joined it.
func RelayUndoDocument(undo, follow, instanceActor IRI, r Relay, followedAt, at time.Time) ActivityDocument {
	f := RelayFollowDocument(follow, instanceActor, r, followedAt)
	return NewActivityDocument(undo, ActivityUndo, instanceActor, at, Audience{To: f.To}, f.Embedded())
}

// FollowAnswerDocument returns the Accept or Reject (t) of the instance actor with the id id
// that answers the Follow follow of the remote actor follower: a LitePub relay follows the
// instance actor back once it accepts the instance's Follow (ADR-0006).
func FollowAnswerDocument(id IRI, t ActivityType, instanceActor, follower, follow IRI, at time.Time) ActivityDocument {
	embedded := NewActivityDocument(follow, ActivityFollow, follower, time.Time{}, Audience{}, instanceActor).Embedded()
	return NewActivityDocument(id, t, instanceActor, at, Audience{To: []IRI{follower}}, embedded)
}

// SignsLinkedData reports whether the document of outbound activity a carries a linked data
// signature (RsaSignature2017, ADR-0006): the Create, Update, and Delete of a status addressed
// to the Public collection in to, that is, of a public status, which relays forward as they are.
func SignsLinkedData(a OutboundActivity, aud Audience) bool {
	if a.ObjectKind != OutboundStatus {
		return false
	}
	switch a.Type {
	case ActivityCreate, ActivityUpdate, ActivityDelete:
	default:
		return false
	}
	for _, to := range aud.To {
		if to == PublicIRI {
			return true
		}
	}
	return false
}
