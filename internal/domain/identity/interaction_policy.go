// SPDX-License-Identifier: AGPL-3.0-only

package identity

import (
	"fmt"
	"slices"
)

// This file holds a user's defaults of who may reply to, boost, and quote their new posts
// (ADR-0155 §4, §5; user_interaction_policies), in the form of the content module's interaction
// policies.

// InteractionKind is an interaction that a policy governs: reply, announce (boost), or quote.
type InteractionKind string

// Interaction kinds.
const (
	InteractionReply    InteractionKind = "reply"
	InteractionAnnounce InteractionKind = "announce"
	InteractionQuote    InteractionKind = "quote"
)

// InteractionKinds are the kinds in the order they are presented and stored.
var InteractionKinds = []InteractionKind{InteractionReply, InteractionAnnounce, InteractionQuote}

// InteractionAudience is whom a default admits; the author always may interact.
type InteractionAudience string

// Interaction audiences, narrowest first: the author alone, the accounts a post mentions, the
// author's followers and the mentioned accounts, and everyone.
const (
	AudienceNobody    InteractionAudience = "nobody"
	AudienceMentioned InteractionAudience = "mentioned"
	AudienceFollowers InteractionAudience = "followers"
	AudienceEveryone  InteractionAudience = "everyone"
)

// interactionAudiences are the audiences, narrowest first.
var interactionAudiences = []InteractionAudience{AudienceNobody, AudienceMentioned, AudienceFollowers, AudienceEveryone}

// InteractionSetting is the default of one kind: whom to admit at once, and whom to admit after
// approving them, a wider audience or none ("").
type InteractionSetting struct {
	Automatic InteractionAudience
	Manual    InteractionAudience
}

// Validate reports ErrInvalidInteractionPolicy for an unknown audience or a manual audience that
// is not wider than the automatic one.
func (s InteractionSetting) Validate() error {
	auto := slices.Index(interactionAudiences, s.Automatic)
	if auto < 0 {
		return fmt.Errorf("%w: automatic %q", ErrInvalidInteractionPolicy, s.Automatic)
	}
	if s.Manual != "" && slices.Index(interactionAudiences, s.Manual) <= auto {
		return fmt.Errorf("%w: manual %q with automatic %q", ErrInvalidInteractionPolicy, s.Manual, s.Automatic)
	}
	return nil
}

// InteractionDefaults are the defaults of every kind.
type InteractionDefaults struct {
	Reply, Announce, Quote InteractionSetting
}

// DefaultInteractionDefaults are the defaults of a new user (ADR-0155 §5): everyone at once, no
// manual approval.
var DefaultInteractionDefaults = InteractionDefaults{
	Reply:    InteractionSetting{Automatic: AudienceEveryone},
	Announce: InteractionSetting{Automatic: AudienceEveryone},
	Quote:    InteractionSetting{Automatic: AudienceEveryone},
}

// Of returns the default of kind.
func (d InteractionDefaults) Of(kind InteractionKind) InteractionSetting {
	switch kind {
	case InteractionReply:
		return d.Reply
	case InteractionAnnounce:
		return d.Announce
	}
	return d.Quote
}

// With returns d with the default of kind replaced; an unknown kind changes nothing.
func (d InteractionDefaults) With(kind InteractionKind, s InteractionSetting) InteractionDefaults {
	switch kind {
	case InteractionReply:
		d.Reply = s
	case InteractionAnnounce:
		d.Announce = s
	case InteractionQuote:
		d.Quote = s
	}
	return d
}

// ValidateInteractionDefaults reports ErrInvalidInteractionPolicy for an unknown kind or an
// invalid setting among the defaults of change.
func ValidateInteractionDefaults(change map[InteractionKind]InteractionSetting) error {
	for k, s := range change {
		if !slices.Contains(InteractionKinds, k) {
			return fmt.Errorf("%w: kind %q", ErrInvalidInteractionPolicy, k)
		}
		if err := s.Validate(); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	}
	return nil
}
