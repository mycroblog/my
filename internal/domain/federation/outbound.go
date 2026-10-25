// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// This file holds the rules of outbound delivery (federation.md §7): the record of an activity
// a local actor sends, the addressing of its document, the inbox each recipient is reached at,
// and which remote servers are delivered to.

// OutboundObjectKind is the kind of local object an outbound activity is about
// (outbound_activities.object_type).
type OutboundObjectKind string

// Outbound object kinds.
const (
	OutboundStatus    OutboundObjectKind = "status"
	OutboundBoost     OutboundObjectKind = "boost"
	OutboundFavourite OutboundObjectKind = "favourite"
	OutboundFollow    OutboundObjectKind = "follow"
	OutboundBlock     OutboundObjectKind = "block"
	OutboundAccount   OutboundObjectKind = "account"
	OutboundReport    OutboundObjectKind = "report"
	// OutboundPollVote is a local account's vote in a remote poll: the Create of the Note that
	// carries one choice, about the poll's status (ObjectID).
	OutboundPollVote OutboundObjectKind = "poll_vote"
	// OutboundRelay is the membership of a relay (ADR-0061): the Follow and Undo that join and
	// leave it, and the Accept or Reject of a Follow of the instance actor. Its object is the
	// relay, or the remote actor whose Follow of the instance actor is rejected.
	OutboundRelay OutboundObjectKind = "relay"
	// OutboundQuote is an activity of the approval of a quote (FEP-044f, ADR-0069 §3): the
	// QuoteRequest of a local status's quote of a remote one, the Accept or Reject of a remote
	// QuoteRequest, and the Delete of a revoked QuoteAuthorization. Its document is stored when
	// it is recorded, and it goes to one inbox (the payload's).
	OutboundQuote OutboundObjectKind = "quote"
	// OutboundRefusal is the Reject of a remote Follow of a local actor that the federation mode
	// refuses (inbound, ADR-0076), recorded without a follow. Its object is the remote actor; its
	// document is stored when it is recorded, and it goes to one inbox (the payload's).
	OutboundRefusal OutboundObjectKind = "refusal"
	// OutboundMove is the Move of a local account to another (ADR-0089): its object is the move
	// (account_moves), and it goes to the actor's followers.
	OutboundMove OutboundObjectKind = "move"
)

// Valid reports whether k is one of the outbound object kinds.
func (k OutboundObjectKind) Valid() bool {
	switch k {
	case OutboundStatus, OutboundBoost, OutboundFavourite, OutboundFollow, OutboundBlock, OutboundAccount, OutboundReport, OutboundPollVote, OutboundRelay,
		OutboundQuote, OutboundRefusal, OutboundMove:
		return true
	}
	return false
}

// OutboundActivity is an activity a local actor sends about one of its objects, recorded in the
// transaction of the local command (federation.md §7.1). Its document is built later, once, by
// the fan-out.
type OutboundActivity struct {
	ID             string
	ActivityURI    IRI
	Type           ActivityType
	ActorAccountID string
	ObjectKind     OutboundObjectKind
	ObjectID       string
	CreatedAt      time.Time
}

// ErrInvalidOutbound reports an outbound activity with a missing or unknown field.
var ErrInvalidOutbound = errors.New("federation: invalid outbound activity")

// Validate checks that every field of a is set and its kind is known.
func (a OutboundActivity) Validate() error {
	switch {
	case a.ID == "" || a.ActivityURI == "" || a.Type == "" || a.ActorAccountID == "" || a.ObjectID == "" || a.CreatedAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidOutbound)
	case !a.ObjectKind.Valid():
		return fmt.Errorf("%w: object kind %q", ErrInvalidOutbound, a.ObjectKind)
	}
	return nil
}

// Delivery is one inbox an outbound activity goes to, shared inboxes merged; unique per
// (OutboundActivityID, InboxURL). The deliveries of a Create are also the record of where a
// status went, for its later Update and Delete.
type Delivery struct {
	ID                 string
	OutboundActivityID string
	InboxURL           IRI
	Domain             string // the inbox's host
	CreatedAt          time.Time
}

// FollowersOf returns the followers collection of a local actor (ADR-0018: {actor}/followers).
func FollowersOf(actor IRI) IRI { return actor + "/followers" }

// Address returns the to and cc of an outbound object of visibility v and of the activity that
// carries it (federation.md §7.4). followers is the author's followers collection; mentioned are
// the actors the object addresses by name, in order (the mentioned accounts of a status, the
// author of a boosted status).
//
//   - public: to Public; cc followers and mentioned.
//   - unlisted: to followers; cc Public and mentioned.
//   - private: to followers; cc mentioned.
//   - local: to followers only. Public is never added, and a local object reaches no one but the
//     author's followers (ADR-0017), so mentioned are left out.
//   - direct: to mentioned.
//
// An unknown visibility addresses no one, so that callers fail closed. The returned slices are
// never nil and hold no duplicates.
func Address(v Visibility, followers IRI, mentioned []IRI) Audience {
	var to, cc []IRI
	switch v {
	case VisibilityPublic:
		to, cc = []IRI{PublicIRI}, append([]IRI{followers}, mentioned...)
	case VisibilityUnlisted:
		to, cc = []IRI{followers}, append([]IRI{PublicIRI}, mentioned...)
	case VisibilityPrivate:
		to, cc = []IRI{followers}, mentioned
	case VisibilityLocal:
		to = []IRI{followers}
	case VisibilityDirect:
		to = mentioned
	}
	to = uniqueIRIs(to, nil)
	return Audience{To: to, Cc: uniqueIRIs(cc, to)}
}

// uniqueIRIs returns the non-empty IRIs of in that are not in skip, without duplicates, in
// order, as a new non-nil slice.
func uniqueIRIs(in, skip []IRI) []IRI {
	out := []IRI{}
	for _, i := range in {
		if i == "" || slices.Contains(skip, i) || slices.Contains(out, i) {
			continue
		}
		out = append(out, i)
	}
	return out
}

// InboxFor returns the inbox a recipient is delivered to: its shared inbox when it has one, so
// that recipients on one server share one delivery (NFR-COST-5, ActivityPub §7.1.3), its own
// inbox otherwise.
func InboxFor(inbox, sharedInbox IRI) IRI {
	if sharedInbox != "" {
		return sharedInbox
	}
	return inbox
}

// InstanceState is whether a remote server is a delivery target (instance_health.state).
type InstanceState string

// Instance states. A server without an instance_health row is available.
const (
	InstanceAvailable   InstanceState = "available"
	InstanceUnavailable InstanceState = "unavailable"
)

// InstanceHealth is the long-term health of a remote server (the instance_health projection,
// federation.md §7.3).
type InstanceHealth struct {
	Domain string
	State  InstanceState
	Since  time.Time
	// FailureDays is the number of distinct UTC days of the latest run of failures.
	FailureDays int
	// LastFailureDay is the latest of them as YYYYMMDD, "" without failures.
	LastFailureDay string
}

// DomainStatus is what decides whether an activity is delivered to a remote server.
type DomainStatus struct {
	// Suspended is true when a domain block with the federation restriction (suspend) covers
	// the server (federation.md §11).
	Suspended bool
	// State is the server's long-term health (federation.md §7.3).
	State InstanceState
}

// Deliverable reports whether an activity of type t is delivered to a server in status s
// (federation.md §7.2 step 4). A suspended server gets nothing. An unavailable server gets only
// Follow, Accept, and Reject, so that a relationship can still be set up with a server that comes
// back; everything else waits until it is available again.
func Deliverable(t ActivityType, s DomainStatus) bool {
	if s.Suspended {
		return false
	}
	if s.State != InstanceUnavailable {
		return true
	}
	switch t {
	case ActivityFollow, ActivityAccept, ActivityReject:
		return true
	}
	return false
}
