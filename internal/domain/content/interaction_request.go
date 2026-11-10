// SPDX-License-Identifier: AGPL-3.0-only

package content

import "time"

// This file holds the approvals of interactions (ADR-0155 §6–§8): the request of a reply, a
// boost, or a quote that its target's author approves, the answers to it, its current state, and
// the approvals that this server gives.

// InteractionDirection is where the two sides of an interaction request are
// (interaction_requests.direction).
type InteractionDirection string

// Interaction directions.
const (
	// DirectionInbound is a remote account's interaction with a local status.
	DirectionInbound InteractionDirection = "inbound"
	// DirectionOutbound is a local account's interaction with a remote status.
	DirectionOutbound InteractionDirection = "outbound"
	// DirectionLocal is a local account's interaction with a local status.
	DirectionLocal InteractionDirection = "local"
	// DirectionRemote is a remote account's interaction with a remote status, kept for the
	// approval that this server verified.
	DirectionRemote InteractionDirection = "remote"
)

// DirectionOf returns the direction of an interaction by an account that is local or not with a
// status that is local or not.
func DirectionOf(targetIsLocal, interactorIsLocal bool) InteractionDirection {
	switch {
	case targetIsLocal && interactorIsLocal:
		return DirectionLocal
	case targetIsLocal:
		return DirectionInbound
	case interactorIsLocal:
		return DirectionOutbound
	}
	return DirectionRemote
}

// Valid reports whether d is a known direction.
func (d InteractionDirection) Valid() bool {
	switch d {
	case DirectionInbound, DirectionOutbound, DirectionLocal, DirectionRemote:
		return true
	}
	return false
}

// InteractionRequest is an interaction that its target's author approves (interaction_requests,
// a resource): a reply, a boost, or a quote of the status TargetURI by InteractingAccountID. One
// interaction has one request per target.
type InteractionRequest struct {
	ID        string
	Kind      InteractionKind
	Direction InteractionDirection
	// TargetURI is the status interacted with; TargetAccountID its author, "" while the status
	// is not stored.
	TargetURI       string
	TargetAccountID string
	// InteractingURI is the reply, the Announce, or the quoting status; InteractingAccountID its
	// author.
	InteractingURI       string
	InteractingAccountID string
	// ActivityURI is the ReplyRequest, AnnounceRequest, or QuoteRequest that asked, "" for an
	// interaction that came without one.
	ActivityURI string
	// Visibility is the boost's, for a boost that is recorded only once it is approved; "" for
	// the others.
	Visibility Visibility
	CreatedAt  time.Time
}

// InteractionEventKind is an answer to an interaction request.
type InteractionEventKind string

// Interaction event kinds (interaction_request_events.kind).
const (
	// InteractionEventAccepted approves the interaction: by an authorization verified or given,
	// or as an interaction with the author's own status.
	InteractionEventAccepted InteractionEventKind = "accepted"
	// InteractionEventRejected refuses it.
	InteractionEventRejected InteractionEventKind = "rejected"
	// InteractionEventRevoked takes an approval back (only of quotes, ADR-0155 §8).
	InteractionEventRevoked InteractionEventKind = "revoked"
)

// Valid reports whether k is a known kind.
func (k InteractionEventKind) Valid() bool {
	switch k {
	case InteractionEventAccepted, InteractionEventRejected, InteractionEventRevoked:
		return true
	}
	return false
}

// InteractionEvent is one answer to an interaction request (interaction_request_events).
type InteractionEvent struct {
	ID        string
	RequestID string
	Kind      InteractionEventKind
	// AuthorizationURI is the authorization that an accepted event verified or gave, or that a
	// revoked event deleted; "" for a rejection and for the approval of an interaction with the
	// author's own status, which needs none.
	AuthorizationURI string
	OccurredAt       time.Time
}

// InteractionRequestState is the state of an interaction request.
type InteractionRequestState string

// Interaction request states (interaction_request_states.state).
const (
	InteractionRequestPending  InteractionRequestState = "pending"
	InteractionRequestAccepted InteractionRequestState = "accepted"
	InteractionRequestRejected InteractionRequestState = "rejected"
	InteractionRequestRevoked  InteractionRequestState = "revoked"
)

// InteractionState is the current state of an interaction request (the
// interaction_request_states projection).
type InteractionState struct {
	RequestID       string
	Kind            InteractionKind
	TargetAccountID string
	State           InteractionRequestState
	// AuthorizationURI is the authorization of an accepted request, "" when it needs none.
	AuthorizationURI string
}

// InitialInteractionState returns the state of r before any answer: pending.
func InitialInteractionState(r InteractionRequest) InteractionState {
	return InteractionState{RequestID: r.ID, Kind: r.Kind, TargetAccountID: r.TargetAccountID, State: InteractionRequestPending}
}

// Apply returns the state after e. A revocation takes back only the approval of the
// authorization it deletes.
func (s InteractionState) Apply(e InteractionEvent) InteractionState {
	out := s
	switch e.Kind {
	case InteractionEventAccepted:
		out.State, out.AuthorizationURI = InteractionRequestAccepted, e.AuthorizationURI
	case InteractionEventRejected:
		out.State, out.AuthorizationURI = InteractionRequestRejected, ""
	case InteractionEventRevoked:
		if s.State == InteractionRequestAccepted && s.AuthorizationURI != "" && s.AuthorizationURI == e.AuthorizationURI {
			out.State, out.AuthorizationURI = InteractionRequestRevoked, ""
		}
	}
	return out
}

// InteractionStateOf derives the state of r from its answers in the order they occurred.
func InteractionStateOf(r InteractionRequest, events []InteractionEvent) InteractionState {
	s := InitialInteractionState(r)
	for _, e := range events {
		s = s.Apply(e)
	}
	return s
}

// InteractionAuthorization is this server's approval of an interaction with a local status
// (interaction_authorizations): GoToSocial's ReplyAuthorization and AnnounceAuthorization, and
// FEP-044f's QuoteAuthorization. An interaction has at most one per target status; a revocation
// takes it back for good.
type InteractionAuthorization struct {
	ID   string
	Kind InteractionKind
	// InteractingURI is the reply, the Announce, or the quoting status, local or remote.
	InteractingURI string
	// InteractingAccountID is its author.
	InteractingAccountID string
	// TargetStatusID is the local status interacted with.
	TargetStatusID string
	CreatedAt      time.Time
	// RevokedAt is when the target's author revoked it, nil while it stands
	// (interaction_authorization_revocations).
	RevokedAt *time.Time
}

// QuoteAuthorization is the approval of a quote.
type QuoteAuthorization = InteractionAuthorization

// authorizationSegments are the path segments of the IRIs of the authorizations of each kind.
var authorizationSegments = map[InteractionKind]string{
	InteractionReply:    "reply_authorizations",
	InteractionAnnounce: "announce_authorizations",
	InteractionQuote:    "quote_authorizations",
}

// AuthorizationSegment returns the path segment of the IRIs of the authorizations of kind:
// /ap/{segment}/{id}.
func AuthorizationSegment(kind InteractionKind) string { return authorizationSegments[kind] }

// InteractionAuthorizationURI returns the ActivityPub id of an authorization of kind of host;
// it is derived from the authorization's ID and never stored.
func InteractionAuthorizationURI(host string, kind InteractionKind, authorizationID string) string {
	return "https://" + host + "/ap/" + AuthorizationSegment(kind) + "/" + authorizationID
}

// QuoteAuthorizationURI returns the ActivityPub id of a QuoteAuthorization of host.
func QuoteAuthorizationURI(host, authorizationID string) string {
	return InteractionAuthorizationURI(host, InteractionQuote, authorizationID)
}

// InteractionRequestURI returns the ActivityPub id of the ReplyRequest or AnnounceRequest that
// this server sends for the request id; it is derived from the request's ID and never stored.
func InteractionRequestURI(host, requestID string) string {
	return "https://" + host + "/ap/interaction_requests/" + requestID
}

// AdmitsEveryone reports whether the rules of kind of p admit anyone at once (as:Public in
// automaticApproval), or whether p declares none of kind for a remote status: an interaction
// that needs no request (ADR-0155 §6, §7).
func AdmitsEveryone(p InteractionPolicy, kind InteractionKind, isLocal bool) bool {
	rules := p.Of(kind)
	if len(rules) == 0 {
		if !isLocal {
			return true
		}
		rules = DefaultInteractionSettings.Policy().Of(kind)
	}
	for _, r := range rules {
		if r.Approval == ApprovalAutomatic && r.Audience == AudienceEveryone {
			return true
		}
	}
	return false
}

// NeedsAuthorization reports whether an interaction that MayInteract decided d of needs the
// approval of its target's author to be shown to others (ADR-0155 §6, §7): one that waits for it,
// and one that the rules admit at once only by who the actor is (a follower, a followed account,
// a named actor), which a third server cannot check. The author's own interactions, the replies
// of the accounts that the status mentions or replies to, and those that the rules admit from
// anyone need none.
func NeedsAuthorization(f InteractionFacts, d InteractionDecision) bool {
	switch d {
	case InteractionPending:
		return true
	case InteractionDenied:
		return false
	}
	if f.ActorAccountID != "" && f.ActorAccountID == f.Status.AuthorAccountID {
		return false
	}
	if f.Kind == InteractionReply && (f.Mentioned || f.RepliedTo) {
		return false
	}
	return !AdmitsEveryone(f.Policy, f.Kind, f.Status.IsLocal)
}
