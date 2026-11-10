// SPDX-License-Identifier: AGPL-3.0-only

package clientapi

import (
	appcontent "github.com/mycroblog/my/internal/app/content"
	"github.com/mycroblog/my/internal/domain/content"
	"github.com/mycroblog/my/internal/domain/identity"
)

// The interaction policies of statuses and the defaults of the preferences (ADR-0155 §10).

// newInteractionRule returns the InteractionRule of the setting of one kind.
func newInteractionRule(automatic, manual string) InteractionRule {
	out := InteractionRule{Automatic: InteractionAudience(automatic)}
	if manual != "" {
		m := InteractionAudience(manual)
		out.Manual = &m
	}
	return out
}

// newInteractionPolicy returns the InteractionPolicy of the settings of a status.
func newInteractionPolicy(s content.InteractionSettings) InteractionPolicy {
	rule := func(set content.InteractionSetting) InteractionRule {
		return newInteractionRule(string(set.Automatic), string(set.Manual))
	}
	return InteractionPolicy{Reply: rule(s.Reply), Announce: rule(s.Announce), Quote: rule(s.Quote)}
}

// newDefaultInteractionPolicy returns the InteractionPolicy of a user's defaults.
func newDefaultInteractionPolicy(d identity.InteractionDefaults) InteractionPolicy {
	rule := func(set identity.InteractionSetting) InteractionRule {
		return newInteractionRule(string(set.Automatic), string(set.Manual))
	}
	return InteractionPolicy{Reply: rule(d.Reply), Announce: rule(d.Announce), Quote: rule(d.Quote)}
}

// changedRules returns the rules that c has by kind, nil for none.
func changedRules(c *InteractionPolicyChange) map[string]InteractionRule {
	if c == nil {
		return nil
	}
	out := map[string]InteractionRule{}
	for kind, r := range map[string]*InteractionRule{"reply": c.Reply, "announce": c.Announce, "quote": c.Quote} {
		if r != nil {
			out[kind] = *r
		}
	}
	return out
}

// interactionPolicyChange returns the change of a status's policy that c asks for.
func interactionPolicyChange(c *InteractionPolicyChange) appcontent.InteractionPolicyChange {
	rules := changedRules(c)
	if len(rules) == 0 {
		return nil
	}
	out := appcontent.InteractionPolicyChange{}
	for kind, r := range rules {
		out[content.InteractionKind(kind)] = content.InteractionSetting{
			Automatic: content.InteractionAudience(r.Automatic), Manual: content.InteractionAudience(deref(r.Manual)),
		}
	}
	return out
}

// interactionDefaultsChange returns the change of a user's defaults that c asks for.
func interactionDefaultsChange(c *InteractionPolicyChange) map[identity.InteractionKind]identity.InteractionSetting {
	rules := changedRules(c)
	if len(rules) == 0 {
		return nil
	}
	out := map[identity.InteractionKind]identity.InteractionSetting{}
	for kind, r := range rules {
		out[identity.InteractionKind(kind)] = identity.InteractionSetting{
			Automatic: identity.InteractionAudience(r.Automatic), Manual: identity.InteractionAudience(deref(r.Manual)),
		}
	}
	return out
}
