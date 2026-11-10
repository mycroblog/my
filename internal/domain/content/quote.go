// SPDX-License-Identifier: AGPL-3.0-only

package content

import "time"

// This file holds the quotes of statuses (FR-114, ADR-0069): the status that a status quotes,
// the approvals of the quote, and their current state.

// QuoteForm is how a status names the status it quotes (ADR-0069 §2).
type QuoteForm string

// Quote forms (status_quotes.form).
const (
	// QuoteFormFEP044f is FEP-044f's quote, whose author must approve it.
	QuoteFormFEP044f QuoteForm = "fep_044f"
	// QuoteFormLegacy is any other spelling (Misskey's _misskey_quote, quoteUrl, Fedibird's
	// quoteUri, an FEP-e232 Link), which carries no approval.
	QuoteFormLegacy QuoteForm = "legacy"
)

// Valid reports whether f is a known form.
func (f QuoteForm) Valid() bool { return f == QuoteFormFEP044f || f == QuoteFormLegacy }

// Quote is the status that a status quotes (status_quotes, ADR-0069 §1). It is fixed when the
// status is created; the author can only take it away (QuoteDetached).
type Quote struct {
	StatusID string
	// QuotedURI is the URI of the quoted status, which may not be stored yet.
	QuotedURI string
	Form      QuoteForm
}

// QuoteEventKind is what happened to the approval of a quote.
type QuoteEventKind string

// Quote event kinds: the answers to the quote's request (interaction_request_events.kind), and
// its detachment (status_quote_detachments).
const (
	// QuoteEventAccepted approves the quote: a QuoteAuthorization verified, or a quote of the
	// author's own status.
	QuoteEventAccepted QuoteEventKind = "accepted"
	// QuoteEventRejected refuses it: the quoted author's Reject of a QuoteRequest.
	QuoteEventRejected QuoteEventKind = "rejected"
	// QuoteEventRevoked withdraws an approval: the Delete of its QuoteAuthorization.
	QuoteEventRevoked QuoteEventKind = "revoked"
	// QuoteEventDetached takes the quote away from the quoting status: an edit without it.
	QuoteEventDetached QuoteEventKind = "detached"
)

// Valid reports whether k is a known kind.
func (k QuoteEventKind) Valid() bool {
	switch k {
	case QuoteEventAccepted, QuoteEventRejected, QuoteEventRevoked, QuoteEventDetached:
		return true
	}
	return false
}

// QuoteEvent is one change of the approval of a quote: an answer to its request, or its
// detachment.
type QuoteEvent struct {
	ID       string
	StatusID string
	Kind     QuoteEventKind
	// AuthorizationURI is the QuoteAuthorization that an accepted event verified or a revoked
	// event deleted; "" for the other kinds and for the approval of a quote of the author's own
	// status, which needs none.
	AuthorizationURI string
	OccurredAt       time.Time
}

// QuoteApproval is the state of the approval of a quote, with the values of Mastodon's
// Quote.state (ADR-0069 §1).
type QuoteApproval string

// Quote approvals.
const (
	QuoteApprovalPending  QuoteApproval = "pending"
	QuoteApprovalAccepted QuoteApproval = "accepted"
	QuoteApprovalRejected QuoteApproval = "rejected"
	QuoteApprovalRevoked  QuoteApproval = "revoked"
	// QuoteApprovalLegacy is a legacy quote, which no approval governs.
	QuoteApprovalLegacy QuoteApproval = "legacy"
	// QuoteApprovalDetached is a quote that its author took away: the status shows none.
	QuoteApprovalDetached QuoteApproval = "detached"
)

// Shown reports whether the quoted status may be embedded in the quoting one: the quote is
// approved or legacy. Who may see the quoted status is decided on top of it, per viewer.
func (a QuoteApproval) Shown() bool { return a == QuoteApprovalAccepted || a == QuoteApprovalLegacy }

// QuoteState is the current approval of a quote, derived from its form, its detachment, and the
// state of its request (QuoteStateFrom).
type QuoteState struct {
	StatusID string
	Approval QuoteApproval
	// AuthorizationURI is the QuoteAuthorization of an accepted quote, "" when it needs none.
	AuthorizationURI string
}

// InitialQuoteState returns the state of q before any event: legacy for a legacy quote, pending
// for an FEP-044f one.
func InitialQuoteState(q Quote) QuoteState {
	s := QuoteState{StatusID: q.StatusID, Approval: QuoteApprovalPending}
	if q.Form == QuoteFormLegacy {
		s.Approval = QuoteApprovalLegacy
	}
	return s
}

// Apply returns the state after e. A detached quote stays detached. A revocation withdraws only
// the approval of the QuoteAuthorization it deletes. The approval of a legacy quote changes
// only by being taken away.
func (s QuoteState) Apply(q Quote, e QuoteEvent) QuoteState {
	if s.Approval == QuoteApprovalDetached {
		return s
	}
	if e.Kind == QuoteEventDetached {
		return QuoteState{StatusID: s.StatusID, Approval: QuoteApprovalDetached}
	}
	if q.Form == QuoteFormLegacy {
		return s
	}
	switch e.Kind {
	case QuoteEventAccepted:
		return QuoteState{StatusID: s.StatusID, Approval: QuoteApprovalAccepted, AuthorizationURI: e.AuthorizationURI}
	case QuoteEventRejected:
		return QuoteState{StatusID: s.StatusID, Approval: QuoteApprovalRejected}
	case QuoteEventRevoked:
		if s.Approval == QuoteApprovalAccepted && s.AuthorizationURI != "" && s.AuthorizationURI == e.AuthorizationURI {
			return QuoteState{StatusID: s.StatusID, Approval: QuoteApprovalRevoked}
		}
	}
	return s
}

// QuoteStateOf derives the state of q from its events in the order they occurred.
func QuoteStateOf(q Quote, events []QuoteEvent) QuoteState {
	s := InitialQuoteState(q)
	for _, e := range events {
		s = s.Apply(q, e)
	}
	return s
}

// QuoteStateFrom returns the state of q from whether its author took it away and the state of
// its request, nil while it has none: as QuoteStateOf derives it from the same events.
func QuoteStateFrom(q Quote, detached bool, req *InteractionState) QuoteState {
	s := InitialQuoteState(q)
	switch {
	case detached:
		return QuoteState{StatusID: q.StatusID, Approval: QuoteApprovalDetached}
	case q.Form == QuoteFormLegacy || req == nil:
		return s
	}
	switch req.State {
	case InteractionRequestAccepted:
		s.Approval, s.AuthorizationURI = QuoteApprovalAccepted, req.AuthorizationURI
	case InteractionRequestRejected:
		s.Approval = QuoteApprovalRejected
	case InteractionRequestRevoked:
		s.Approval = QuoteApprovalRevoked
	}
	return s
}

// QuoteAllowance is whether an account may quote a status (MayQuote).
type QuoteAllowance int

// Quote allowances.
const (
	// QuoteDenied: the account may not quote the status.
	QuoteDenied QuoteAllowance = iota
	// QuoteApproved: the quote is approved at once: a quote of a local status, or of the
	// account's own.
	QuoteApproved
	// QuoteRequested: the quote of a remote status must be approved by its server, which a
	// QuoteRequest asks.
	QuoteRequested
	// QuoteLegacy: a remote status that declares no policy, whose server knows no approvals.
	QuoteLegacy
	// QuotePending: the quote of a local status waits for its author's approval: a manual rule
	// of its policy includes the quoter (ADR-0155 §6).
	QuotePending
)

// QuoteFacts are what MayQuote decides by.
type QuoteFacts struct {
	// QuoterAccountID is the account that quotes, QuoterURI its actor IRI (needed only for a
	// policy that names actors), QuoterIsLocal whether it is local.
	QuoterAccountID string
	QuoterURI       string
	QuoterIsLocal   bool
	// QuoterVisible is whether the quoter may see the quoted status (CanView).
	QuoterVisible bool
	// QuoterFollows is whether the quoter actively follows the quoted author, QuoterFollowed
	// whether the quoted author actively follows the quoter, and QuoterMentioned whether the
	// quoted status mentions the quoter.
	QuoterFollows   bool
	QuoterFollowed  bool
	QuoterMentioned bool
	// QuotingVisibility is the visibility of the quoting status, "" when it is not known yet
	// (whether a status may be quoted at all).
	QuotingVisibility Visibility
	// Quoted is the quoted status; Policy its interaction policy, whose quote rules decide.
	Quoted Status
	Policy InteractionPolicy
}

// QuotesRemotePosts reports whether a local status of visibility v may quote a remote status
// (ADR-0018 §15): a quote names the quoting status's IRI to the quoted author's server, which is
// outside the audience of a local, private, or direct status.
func QuotesRemotePosts(v Visibility) bool {
	return v == VisibilityPublic || v == VisibilityUnlisted
}

// MayQuote decides whether an account may quote a status (ADR-0069 §3, §4, ADR-0155 §3) by
// MayInteract: a refused quote is denied; the quote of a local status is approved at once, or
// waits for its author when a manual rule includes the quoter; the author's quote of their own
// status is approved; and the quote of a remote status is requested from its server, or legacy
// when the status declares no quote rules.
func MayQuote(f QuoteFacts) QuoteAllowance {
	q := f.Quoted
	decision := MayInteract(InteractionFacts{
		Kind: InteractionQuote, ActorAccountID: f.QuoterAccountID, ActorURI: f.QuoterURI, ActorIsLocal: f.QuoterIsLocal,
		ActorVisible: f.QuoterVisible, Follows: f.QuoterFollows, FollowedBy: f.QuoterFollowed, Mentioned: f.QuoterMentioned,
		InteractingVisibility: f.QuotingVisibility, Status: q, Policy: f.Policy,
	})
	self := f.QuoterAccountID != "" && f.QuoterAccountID == q.AuthorAccountID
	switch {
	case decision == InteractionDenied:
		return QuoteDenied
	case self:
		return QuoteApproved
	case q.IsLocal && decision == InteractionPending:
		return QuotePending
	case q.IsLocal:
		return QuoteApproved
	case !f.Policy.Declares(InteractionQuote):
		return QuoteLegacy
	}
	return QuoteRequested
}
