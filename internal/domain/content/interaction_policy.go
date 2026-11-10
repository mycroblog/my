// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// This file holds who may reply to, boost, and quote a status (ADR-0155 §1–§5): the rules of a
// status, one per kind, approval, and audience (status_interaction_policies), what a local
// author chooses for each kind, and the decision of whether an account may interact.

// ErrInvalidInteractionPolicy reports an interaction policy that a local author may not choose:
// an unknown kind or audience, or a manual audience that is not wider than the automatic one.
var ErrInvalidInteractionPolicy = errors.New("invalid interaction policy")

// InteractionKind is an interaction that a policy governs (ADR-0155 §1).
type InteractionKind string

// Interaction kinds (status_interaction_policies.kind).
const (
	InteractionReply    InteractionKind = "reply"
	InteractionAnnounce InteractionKind = "announce"
	InteractionQuote    InteractionKind = "quote"
)

// InteractionKinds are the kinds in the order they are presented and stored.
var InteractionKinds = []InteractionKind{InteractionReply, InteractionAnnounce, InteractionQuote}

// Valid reports whether k is a known kind.
func (k InteractionKind) Valid() bool { return slices.Contains(InteractionKinds, k) }

// InteractionApproval is how a rule admits the accounts of its audience (ADR-0155 §2): at once,
// or after the author approves (GoToSocial's automaticApproval and manualApproval).
type InteractionApproval string

// Interaction approvals (status_interaction_policies.approval).
const (
	ApprovalAutomatic InteractionApproval = "automatic"
	ApprovalManual    InteractionApproval = "manual"
)

// Valid reports whether a is a known approval.
func (a InteractionApproval) Valid() bool { return a == ApprovalAutomatic || a == ApprovalManual }

// InteractionAudience is whom a rule admits (ADR-0155 §2). The author always may interact.
type InteractionAudience string

// Interaction audiences (status_interaction_policies.audience). A local author chooses among
// the first four, which widen in turn; the others come only from the interactionPolicy of a
// remote status.
const (
	// AudienceNobody admits no one but the author.
	AudienceNobody InteractionAudience = "nobody"
	// AudienceMentioned admits the accounts that the status mentions.
	AudienceMentioned InteractionAudience = "mentioned"
	// AudienceFollowers admits the author's followers, and, on a local status, the accounts
	// it mentions.
	AudienceFollowers InteractionAudience = "followers"
	// AudienceEveryone admits anyone (as:Public).
	AudienceEveryone InteractionAudience = "everyone"
	// AudienceFollowing admits the accounts that the remote author follows (the author's
	// following collection).
	AudienceFollowing InteractionAudience = "following"
	// AudienceAuthor is the remote author's own IRI, which admits no one else.
	AudienceAuthor InteractionAudience = "author"
	// AudienceAccount admits one actor, the rule's ActorURI.
	AudienceAccount InteractionAudience = "account"
)

// localAudiences are the audiences that a local author chooses, narrowest first.
var localAudiences = []InteractionAudience{AudienceNobody, AudienceMentioned, AudienceFollowers, AudienceEveryone}

// Valid reports whether a is a known audience.
func (a InteractionAudience) Valid() bool {
	return slices.Contains(localAudiences, a) || a == AudienceFollowing || a == AudienceAuthor || a == AudienceAccount
}

// Choosable reports whether a local author may choose a.
func (a InteractionAudience) Choosable() bool { return slices.Contains(localAudiences, a) }

// InteractionRule is one row of a status's interaction policy: the accounts of Audience may
// interact by Kind with the status, approved as Approval says.
type InteractionRule struct {
	Kind     InteractionKind
	Approval InteractionApproval
	Audience InteractionAudience
	// ActorURI is the one actor of AudienceAccount, "" for the other audiences.
	ActorURI string
}

// InteractionPolicy is the rules of a status (status_interaction_policies, a current value). A
// local status has rules of every kind; a remote status has those of the kinds its
// interactionPolicy declares, so a kind without rules is not declared.
type InteractionPolicy []InteractionRule

// Declares reports whether p has rules of kind.
func (p InteractionPolicy) Declares(kind InteractionKind) bool {
	return slices.ContainsFunc(p, func(r InteractionRule) bool { return r.Kind == kind })
}

// Of returns the rules of p of kind.
func (p InteractionPolicy) Of(kind InteractionKind) InteractionPolicy {
	var out InteractionPolicy
	for _, r := range p {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// Without returns the rules of p of every kind but those of kinds.
func (p InteractionPolicy) Without(kinds ...InteractionKind) InteractionPolicy {
	var out InteractionPolicy
	for _, r := range p {
		if !slices.Contains(kinds, r.Kind) {
			out = append(out, r)
		}
	}
	return out
}

// InteractionSetting is what a local author chooses for one kind (ADR-0155 §2): whom to admit
// at once, and whom to admit after approving them, a wider audience or none ("").
type InteractionSetting struct {
	Automatic InteractionAudience
	Manual    InteractionAudience
}

// Validate reports ErrInvalidInteractionPolicy unless s is a choice of a local author.
func (s InteractionSetting) Validate() error {
	if !s.Automatic.Choosable() {
		return fmt.Errorf("%w: automatic %q", ErrInvalidInteractionPolicy, s.Automatic)
	}
	if s.Manual != "" && (!s.Manual.Choosable() || slices.Index(localAudiences, s.Manual) <= slices.Index(localAudiences, s.Automatic)) {
		return fmt.Errorf("%w: manual %q with automatic %q", ErrInvalidInteractionPolicy, s.Manual, s.Automatic)
	}
	return nil
}

// InteractionSettings are what a local author chooses for every kind.
type InteractionSettings struct {
	Reply, Announce, Quote InteractionSetting
}

// DefaultInteractionSettings are the settings of a user who never chose any (ADR-0155 §5):
// everyone at once, no manual approval.
var DefaultInteractionSettings = InteractionSettings{
	Reply:    InteractionSetting{Automatic: AudienceEveryone},
	Announce: InteractionSetting{Automatic: AudienceEveryone},
	Quote:    InteractionSetting{Automatic: AudienceEveryone},
}

// Of returns the setting of kind.
func (s InteractionSettings) Of(kind InteractionKind) InteractionSetting {
	switch kind {
	case InteractionReply:
		return s.Reply
	case InteractionAnnounce:
		return s.Announce
	}
	return s.Quote
}

// With returns s with the setting of kind replaced.
func (s InteractionSettings) With(kind InteractionKind, setting InteractionSetting) InteractionSettings {
	switch kind {
	case InteractionReply:
		s.Reply = setting
	case InteractionAnnounce:
		s.Announce = setting
	case InteractionQuote:
		s.Quote = setting
	}
	return s
}

// Validate reports ErrInvalidInteractionPolicy unless the setting of every kind is valid.
func (s InteractionSettings) Validate() error {
	for _, k := range InteractionKinds {
		if err := s.Of(k).Validate(); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	}
	return nil
}

// Policy returns the rules of a local status with s: one automatic rule of each kind, and a
// manual one where s has it.
func (s InteractionSettings) Policy() InteractionPolicy {
	out := make(InteractionPolicy, 0, 2*len(InteractionKinds))
	for _, k := range InteractionKinds {
		set := s.Of(k)
		out = append(out, InteractionRule{Kind: k, Approval: ApprovalAutomatic, Audience: set.Automatic})
		if set.Manual != "" {
			out = append(out, InteractionRule{Kind: k, Approval: ApprovalManual, Audience: set.Manual})
		}
	}
	return out
}

// SettingsOf returns the settings that the rules of a local status record; a kind without a
// rule a local author may choose takes the default.
func SettingsOf(p InteractionPolicy) InteractionSettings {
	out := DefaultInteractionSettings
	for _, k := range InteractionKinds {
		var set InteractionSetting
		for _, r := range p.Of(k) {
			if !r.Audience.Choosable() {
				continue
			}
			switch r.Approval {
			case ApprovalAutomatic:
				set.Automatic = r.Audience
			case ApprovalManual:
				set.Manual = r.Audience
			}
		}
		if set.Automatic != "" {
			out = out.With(k, set)
		}
	}
	return out
}

// InteractionDecision is whether an account may interact with a status (ADR-0155 §3).
type InteractionDecision int

// Interaction decisions.
const (
	// InteractionDenied refuses the interaction.
	InteractionDenied InteractionDecision = iota
	// InteractionAllowed admits it at once.
	InteractionAllowed
	// InteractionPending admits it once the author approves it.
	InteractionPending
)

// InteractionFacts are what MayInteract decides by.
type InteractionFacts struct {
	Kind InteractionKind
	// ActorAccountID is the account that interacts, ActorURI its actor IRI, ActorIsLocal
	// whether it is local.
	ActorAccountID string
	ActorURI       string
	ActorIsLocal   bool
	// ActorVisible is whether the actor may see the status (CanView) and neither blocks the
	// other, nor is under the interaction restriction (FR-708).
	ActorVisible bool
	// Follows is whether the actor actively follows the author, FollowedBy whether the author
	// actively follows the actor.
	Follows    bool
	FollowedBy bool
	// Mentioned is whether the status mentions the actor, RepliedTo whether it replies to a
	// status of the actor.
	Mentioned bool
	RepliedTo bool
	// InteractingVisibility is the visibility of the actor's quoting status, "" when it is not
	// known yet (whether a status may be quoted at all).
	InteractingVisibility Visibility
	// Status is the status interacted with; Policy its rules.
	Status Status
	Policy InteractionPolicy
}

// MayInteract decides whether an account may interact with a status (ADR-0155 §3), in turn:
//
//  1. a status the actor may not see refuses every interaction;
//  2. the visibility limits the kind: a direct status is neither boosted nor quoted, a
//     followers-only one is boosted only by its author and quoted only by its author in a
//     followers-only or direct status (ADR-0069 §4), and a local-only one is neither boosted
//     nor quoted by a remote account (ADR-0017);
//  3. the author always may;
//  4. the rules of the kind decide: an automatic rule that includes the actor allows, a manual
//     one makes it pending, and else it is refused. A reply that the status mentions or replies
//     to is allowed whatever the rules. A local status without rules has the default ones; a
//     remote status that declares no rules of the kind allows it (GoToSocial's convention).
func MayInteract(f InteractionFacts) InteractionDecision {
	st := f.Status
	self := f.ActorAccountID != "" && f.ActorAccountID == st.AuthorAccountID
	if !f.ActorVisible {
		return InteractionDenied
	}
	switch f.Kind {
	case InteractionAnnounce:
		if !st.Visibility.Boostable(self) || (st.Visibility == VisibilityLocal && !f.ActorIsLocal) {
			return InteractionDenied
		}
	case InteractionQuote:
		switch {
		case st.Visibility == VisibilityDirect:
			return InteractionDenied
		case st.Visibility == VisibilityPrivate:
			v := f.InteractingVisibility
			if !self || (v != "" && v != VisibilityPrivate && v != VisibilityDirect) {
				return InteractionDenied
			}
		case st.Visibility == VisibilityLocal && !f.ActorIsLocal:
			return InteractionDenied
		}
	}
	if self || (f.Kind == InteractionReply && (f.Mentioned || f.RepliedTo)) {
		return InteractionAllowed
	}
	rules := f.Policy.Of(f.Kind)
	if len(rules) == 0 {
		if !st.IsLocal {
			return InteractionAllowed
		}
		rules = DefaultInteractionSettings.Policy().Of(f.Kind)
	}
	decision := InteractionDenied
	for _, r := range rules {
		if !r.includes(f) {
			continue
		}
		if r.Approval == ApprovalAutomatic {
			return InteractionAllowed
		}
		decision = InteractionPending
	}
	return decision
}

// includes reports whether r's audience includes the actor of f.
func (r InteractionRule) includes(f InteractionFacts) bool {
	switch r.Audience {
	case AudienceEveryone:
		return true
	case AudienceFollowers:
		return f.Follows || (f.Status.IsLocal && f.Mentioned)
	case AudienceMentioned:
		return f.Mentioned
	case AudienceFollowing:
		return f.FollowedBy
	case AudienceAccount:
		return r.ActorURI != "" && r.ActorURI == f.ActorURI
	}
	return false
}

// HeldReplyReason is why a reply is held out of the thread of its parent: the decision of
// MayInteract that did not admit it.
type HeldReplyReason string

// Held reply reasons (held_replies.reason).
const (
	// HeldReplyPending is a reply that a manual rule includes: it waits for the author's approval.
	HeldReplyPending HeldReplyReason = "pending"
	// HeldReplyRefused is a reply that no rule includes.
	HeldReplyRefused HeldReplyReason = "refused"
)

// Valid reports whether r is a known reason.
func (r HeldReplyReason) Valid() bool { return r == HeldReplyPending || r == HeldReplyRefused }

// HeldReply is a remote reply to a local status that the status's reply rules did not admit
// when it arrived (held_replies, ADR-0155 §6). It is stored and shown on its author's profile
// and on its own, but it is not among the replies of its parent nor counted, and it notifies no
// one. A later change of the rules does not release it, as it does not withdraw a reply admitted
// before.
type HeldReply struct {
	StatusID string
	Reason   HeldReplyReason
	HeldAt   time.Time
}

// HeldReplyReasonOf returns the reason to hold a reply that d did not admit; ok is false for a
// reply that d admits.
func HeldReplyReasonOf(d InteractionDecision) (HeldReplyReason, bool) {
	switch d {
	case InteractionAllowed:
		return "", false
	case InteractionPending:
		return HeldReplyPending, true
	}
	return HeldReplyRefused, true
}
