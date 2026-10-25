// SPDX-License-Identifier: AGPL-3.0-only

package federation

// This file holds the federation direction (FR-413, ADR-0076): the instance setting
// federation_mode, the flows of content between this server and the others, and which flows each
// mode lets through. Every entry point of federation asks Allows (and Refuses) about the flow it
// handles and never branches on the mode's value.

// Mode is the direction the instance federates in: the value of the instance setting
// federation_mode.
type Mode string

// Federation modes.
const (
	// ModeOff federates in neither direction.
	ModeOff Mode = "off"
	// ModeInbound follows and reads remote accounts, and sends nothing of the local posts,
	// profiles, and reactions.
	ModeInbound Mode = "inbound"
	// ModeOutbound takes remote followers and delivers the local posts to them, and takes in
	// nothing of the remote posts and reactions.
	ModeOutbound Mode = "outbound"
	// ModeBoth federates in both directions: the default.
	ModeBoth Mode = "both"
)

// DefaultMode is the mode of an instance that never set federation_mode.
const DefaultMode = ModeBoth

// Modes returns every mode, in order.
func Modes() []Mode { return []Mode{ModeOff, ModeInbound, ModeOutbound, ModeBoth} }

// Valid reports whether m is one of the modes.
func (m Mode) Valid() bool {
	switch m {
	case ModeOff, ModeInbound, ModeOutbound, ModeBoth:
		return true
	}
	return false
}

// Flow is a kind of content that moves between this server and the others (ADR-0076, 流れの分類).
type Flow string

// Flows.
const (
	// FlowPublish sends the local posts, profiles, and reactions out (delivery and GET).
	FlowPublish Flow = "publish"
	// FlowIngest takes the remote posts and reactions in (receiving and fetching).
	FlowIngest Flow = "ingest"
	// FlowFollowOut is the exchange of a local account following a remote one: Follow and Undo
	// Follow sent, Accept and Reject received.
	FlowFollowOut Flow = "follow_out"
	// FlowFollowIn is the exchange of a remote account following a local one: Follow and Undo
	// Follow received, Accept and Reject sent.
	FlowFollowIn Flow = "follow_in"
	// FlowRetract takes back what was sent out: the Delete of a post or an actor, Undo Like, Undo
	// Announce, Undo EmojiReact, and the Update of a suspended actor.
	FlowRetract Flow = "retract"
	// FlowProtect is the protective procedures: Block and Undo Block, Flag, and a remote actor's
	// Update or Delete of itself.
	FlowProtect Flow = "protect"
	// FlowIdentify fetches and serves the keys that signatures are verified with: fetching remote
	// actors, GET of the local actors and the instance actor, WebFinger, host-meta, and NodeInfo.
	FlowIdentify Flow = "identify"
)

// Flows returns every flow, in order.
func Flows() []Flow {
	return []Flow{FlowPublish, FlowIngest, FlowFollowOut, FlowFollowIn, FlowRetract, FlowProtect, FlowIdentify}
}

// allowed is the table of ADR-0076 (流れの分類): the modes each flow passes in.
var allowed = map[Flow][]Mode{
	FlowPublish:   {ModeOutbound, ModeBoth},
	FlowIngest:    {ModeInbound, ModeBoth},
	FlowFollowOut: {ModeInbound, ModeBoth},
	FlowFollowIn:  {ModeOutbound, ModeBoth},
	FlowRetract:   {ModeInbound, ModeOutbound, ModeBoth},
	FlowProtect:   {ModeInbound, ModeOutbound, ModeBoth},
	FlowIdentify:  {ModeInbound, ModeOutbound, ModeBoth},
}

// Allows reports whether mode lets flow through. An unknown mode or flow lets nothing through.
func Allows(mode Mode, flow Flow) bool {
	for _, m := range allowed[flow] {
		if m == mode {
			return true
		}
	}
	return false
}

// Refuses reports whether mode answers flow with a refusal instead of dropping it: inbound
// refuses the remote Follows of local accounts with a Reject (follow_in), so that they do not
// stay pending. The Reject itself goes out in that mode.
func Refuses(mode Mode, flow Flow) bool {
	return mode == ModeInbound && flow == FlowFollowIn
}

// OutboundFlows returns the flows of an outbound activity a (federation.md §7). The activity goes
// out when the mode allows one of them, or, for a Reject, refuses one of them (Sends). retracting
// tells the Update of an actor that carries its suspension (ADR-0036), which takes the actor's
// profile back, or its movedTo (ADR-0089), which only tells the followers where it went.
//
//   - The Create and Update of a status, a boost, a favourite or reaction, a vote, a
//     QuoteRequest, and the answers to remote QuoteRequests: publish. Their Delete or Undo, and
//     the Delete of a revoked QuoteAuthorization: retract.
//   - The Update of an actor: publish; of its suspension or with its movedTo, its Delete, and the
//     Move of the actor: retract (ADR-0089, so that inbound sends them too).
//   - Follow and Undo Follow: follow_out; Accept and Reject of a follow, and the Reject of a
//     refused Follow: follow_in.
//   - Block, Undo Block, and Flag: protect.
//   - The Follow and Undo of a relay's membership: ingest or publish (receiving or sending
//     through the relay); the Accept of a relay's Follow of the instance actor: publish; the
//     Reject of another Follow of the instance actor: follow_in.
func OutboundFlows(a OutboundActivity, retracting bool) []Flow {
	switch a.ObjectKind {
	case OutboundStatus, OutboundBoost, OutboundFavourite, OutboundPollVote, OutboundQuote:
		if a.Type == ActivityDelete || a.Type == ActivityUndo {
			return []Flow{FlowRetract}
		}
		return []Flow{FlowPublish}
	case OutboundAccount:
		if a.Type == ActivityDelete || retracting {
			return []Flow{FlowRetract}
		}
		return []Flow{FlowPublish}
	case OutboundMove:
		return []Flow{FlowRetract}
	case OutboundFollow:
		if a.Type == ActivityFollow || a.Type == ActivityUndo {
			return []Flow{FlowFollowOut}
		}
		return []Flow{FlowFollowIn}
	case OutboundRefusal:
		return []Flow{FlowFollowIn}
	case OutboundBlock, OutboundReport:
		return []Flow{FlowProtect}
	case OutboundRelay:
		switch a.Type {
		case ActivityAccept:
			return []Flow{FlowPublish}
		case ActivityReject:
			return []Flow{FlowFollowIn}
		}
		return []Flow{FlowIngest, FlowPublish}
	}
	return nil
}

// Sends reports whether mode lets the outbound activity a go out: one of its flows passes, or a
// is a Reject that answers a flow the mode refuses (Refuses).
func Sends(mode Mode, a OutboundActivity, retracting bool) bool {
	for _, f := range OutboundFlows(a, retracting) {
		if Allows(mode, f) || a.Type == ActivityReject && Refuses(mode, f) {
			return true
		}
	}
	return false
}

// AddsRecipients reports whether mode lets the outbound activity a reach recipients beyond the
// ones that what it takes back went to: a retraction in a mode that does not publish goes only
// to the inboxes of the original delivery and the followers at the time (ADR-0076), never to
// every known shared inbox.
func AddsRecipients(mode Mode, a OutboundActivity, retracting bool) bool {
	for _, f := range OutboundFlows(a, retracting) {
		if f != FlowRetract {
			return true
		}
	}
	return Allows(mode, FlowPublish)
}
