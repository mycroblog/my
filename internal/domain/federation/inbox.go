// SPDX-License-Identifier: AGPL-3.0-only

package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// MaxEnvelopeIRIBytes bounds the id, actor, and object id that the inbox reads before verifying
// a request (federation.md §3.1): they become keys of received_activities and of the inbox job.
const MaxEnvelopeIRIBytes = 2048

// MaxEnvelopeTypeBytes bounds the type name stored with a received activity.
const MaxEnvelopeTypeBytes = 255

// DigestURIPrefix starts the activity URI synthesized for an activity without an id.
const DigestURIPrefix = "urn:my:digest:"

// Envelope is what the inbox reads from a received activity before verifying it (federation.md
// §3.1 step 7): the id, the first type name, the actor, and the id of the first object. The
// rest of the document is read later by Normalize.
type Envelope struct {
	// ID is the activity's id; "" when the document has none (see ActivityURI).
	ID IRI
	// Type is the first type name of the document.
	Type string
	// Actor is the activity's actor: an https IRI.
	Actor IRI
	// Object is the id of the first object; "" when it has none.
	Object IRI
}

// ReadEnvelope reads the Envelope of a received activity. It fails with ErrMalformed when raw is
// not a JSON object with a type and an https actor, when an id is present but is not a valid
// IRI, or when a value is over MaxEnvelopeIRIBytes or MaxEnvelopeTypeBytes.
func ReadEnvelope(raw []byte, host HostFunc) (Envelope, error) {
	v, err := decode(raw, MaxDepth)
	if err != nil {
		return Envelope{}, err
	}
	o, ok := v.(object)
	if !ok {
		return Envelope{}, fmt.Errorf("%w: not a JSON object", ErrMalformed)
	}
	n := normalizer{host: host}
	var e Envelope
	if types := strs(o.get("type")); len(types) > 0 {
		e.Type = types[0]
	}
	if e.Type == "" || len(e.Type) > MaxEnvelopeTypeBytes {
		return Envelope{}, fmt.Errorf("%w: no usable type", ErrMalformed)
	}
	if id := str(o.get("id")); id != "" {
		if e.ID = n.normalizeIRI(id); e.ID == "" || e.ID == PublicIRI {
			return Envelope{}, fmt.Errorf("%w: invalid id", ErrMalformed)
		}
	}
	e.Actor = n.firstRef(o.get("actor"))
	if !strings.HasPrefix(string(e.Actor), "https://") || e.Actor == PublicIRI {
		return Envelope{}, fmt.Errorf("%w: no https actor", ErrMalformed)
	}
	if objects := list(o.get("object")); len(objects) > 0 {
		switch v := objects[0].(type) {
		case string:
			e.Object = n.normalizeIRI(v)
		case object:
			e.Object = n.linkIRI(v)
		}
	}
	for _, i := range []IRI{e.ID, e.Actor, e.Object} {
		if len(i) > MaxEnvelopeIRIBytes {
			return Envelope{}, fmt.Errorf("%w: IRI over %d bytes", ErrMalformed, MaxEnvelopeIRIBytes)
		}
	}
	return e, nil
}

// ActivityURI returns the key that deduplicates the activity (data-model §2.2): its id, or for an
// activity without one, DigestURIPrefix and the hex SHA-256 of the actor, a line feed, and the
// body as received.
func (e Envelope) ActivityURI(body []byte) IRI {
	if e.ID != "" {
		return e.ID
	}
	h := sha256.New()
	h.Write([]byte(e.Actor))
	h.Write([]byte{'\n'})
	h.Write(body)
	return IRI(DigestURIPrefix + hex.EncodeToString(h.Sum(nil)))
}

// SelfRemoval reports whether the activity is the actor's Delete or Update of itself, which the
// inbox drops without fetching the key when the actor is unknown (ADR-0019 §7).
func (e Envelope) SelfRemoval() bool {
	return (e.Type == string(ActivityDelete) || e.Type == string(ActivityUpdate)) && e.Object == e.Actor
}

// Trust says whether a received activity may be processed as signed or must be fetched again
// from its origin (received_activities.trust).
type Trust string

// Trust values.
const (
	// TrustSigned is an activity whose signer is its actor, on the actor's host.
	TrustSigned Trust = "signed"
	// TrustRefetch is an activity relayed or forwarded by someone else, or whose id is on
	// another host than its actor: the worker fetches its object from the origin.
	TrustRefetch Trust = "refetch"
)

// Valid reports whether t is one of the Trust values.
func (t Trust) Valid() bool { return t == TrustSigned || t == TrustRefetch }

// TrustFor returns the trust of the activity signed by signer (the canonical owner of the key,
// "" when it is not usable): TrustSigned when the signer is the actor and the id, when the
// activity has one, is on the actor's host; TrustRefetch otherwise (federation.md §3.1 step 12).
func (e Envelope) TrustFor(signer IRI) Trust {
	if signer == "" || signer != e.Actor {
		return TrustRefetch
	}
	if e.ID != "" && e.ID.Host() != e.Actor.Host() {
		return TrustRefetch
	}
	return TrustSigned
}

// ReceivedActivity is an activity accepted at an inbox (received_activities).
type ReceivedActivity struct {
	ActivityURI IRI
	ActorURI    IRI
	// Type is the first type name of the document as received.
	Type string
	// Scheme is the signature scheme that verified it: "rfc9421" or "cavage".
	Scheme     string
	Trust      Trust
	ReceivedAt time.Time
}

// RemoteObjectTombstone records a Delete of an unknown object, or an Undo of an unknown
// activity, so that the object or activity arriving later is dropped (remote_object_tombstones,
// federation.md §3.3).
type RemoteObjectTombstone struct {
	// URI is the deleted object or the undone activity: on the origin of ActorURI.
	URI         IRI
	ActorURI    IRI
	ActivityURI IRI
	ReceivedAt  time.Time
}

// TombstoneFor returns the tombstone that the Delete or Undo act, received as activityURI, leaves
// for its object when the object is unknown, and whether it leaves one: only an object on the
// actor's origin other than the actor itself can be tombstoned, so that nobody blocks the
// objects of others.
func TombstoneFor(act *Activity, activityURI IRI, receivedAt time.Time) (RemoteObjectTombstone, bool) {
	if act == nil || (act.Type != ActivityDelete && act.Type != ActivityUndo) {
		return RemoteObjectTombstone{}, false
	}
	obj := act.Object().ID
	if obj == "" || obj == act.Actor || !SameOrigin(obj, act.Actor) {
		return RemoteObjectTombstone{}, false
	}
	return RemoteObjectTombstone{URI: obj, ActorURI: act.Actor, ActivityURI: activityURI, ReceivedAt: receivedAt}, true
}

// RemoteFollowTombstone records an Undo of an unknown Follow by the pair that the embedded
// Follow names, so that the first Follow of the pair arriving later is dropped
// (remote_follow_tombstones, federation.md §3.3). Misskey's Undo names its Follow by an id other
// than the one the Follow was sent with, so the URI alone would not stop it.
type RemoteFollowTombstone struct {
	// ActivityURI is the Undo.
	ActivityURI IRI
	// ActorURI is the follower, the actor of the Undo; ObjectURI is the followee.
	ActorURI   IRI
	ObjectURI  IRI
	ReceivedAt time.Time
}

// FollowTombstoneFor returns the tombstone that the Undo act, received as activityURI, leaves
// for the pair of an embedded Follow when that Follow is unknown, and whether it leaves one: the
// Follow must be the actor's own and name an object other than the actor. Such a tombstone only
// ever drops the actor's own Follows.
func FollowTombstoneFor(act *Activity, activityURI IRI, receivedAt time.Time) (RemoteFollowTombstone, bool) {
	if act == nil || act.Type != ActivityUndo {
		return RemoteFollowTombstone{}, false
	}
	doc := act.Object().Untrusted()
	if doc == nil || doc.Activity == nil || doc.Activity.Type != ActivityFollow || doc.Activity.Actor != act.Actor {
		return RemoteFollowTombstone{}, false
	}
	obj := doc.Activity.Object().ID
	if obj == "" || obj == act.Actor {
		return RemoteFollowTombstone{}, false
	}
	return RemoteFollowTombstone{ActivityURI: activityURI, ActorURI: act.Actor, ObjectURI: obj, ReceivedAt: receivedAt}, true
}
