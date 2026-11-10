// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	appaccount "github.com/mycroblog/my/internal/app/account"
	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/content"
)

// This file holds the interaction policies of statuses (ADR-0155 §4, §5): who may reply to,
// boost, and quote a status, as its local author chose or a remote status declares.

// ErrInvalidInteractionPolicy reports an interaction policy that a local author may not choose
// (content.InteractionSetting.Validate).
var ErrInvalidInteractionPolicy = content.ErrInvalidInteractionPolicy

// The interaction policies as part of this module's public API, for the modules that do not
// import this module's domain.
type (
	// InteractionKind is content.InteractionKind.
	InteractionKind = content.InteractionKind
	// InteractionApproval is content.InteractionApproval.
	InteractionApproval = content.InteractionApproval
	// InteractionAudience is content.InteractionAudience.
	InteractionAudience = content.InteractionAudience
	// InteractionRule is content.InteractionRule.
	InteractionRule = content.InteractionRule
	// InteractionPolicy is content.InteractionPolicy.
	InteractionPolicy = content.InteractionPolicy
	// InteractionSetting is content.InteractionSetting.
	InteractionSetting = content.InteractionSetting
	// InteractionSettings is content.InteractionSettings.
	InteractionSettings = content.InteractionSettings
)

// The values of the interaction policies' types that other modules use.
const (
	InteractionReply    = content.InteractionReply
	InteractionAnnounce = content.InteractionAnnounce
	InteractionQuote    = content.InteractionQuote
	ApprovalAutomatic   = content.ApprovalAutomatic
	ApprovalManual      = content.ApprovalManual
	AudienceEveryone    = content.AudienceEveryone
	AudienceFollowers   = content.AudienceFollowers
	AudienceMentioned   = content.AudienceMentioned
	AudienceNobody      = content.AudienceNobody
	AudienceFollowing   = content.AudienceFollowing
	AudienceAuthor      = content.AudienceAuthor
	AudienceAccount     = content.AudienceAccount
)

// InteractionKinds are content.InteractionKinds.
var InteractionKinds = content.InteractionKinds

// DefaultInteractionSettings are content.DefaultInteractionSettings.
var DefaultInteractionSettings = content.DefaultInteractionSettings

// SettingsOf returns the settings that the rules of a local status record
// (content.SettingsOf).
func SettingsOf(p content.InteractionPolicy) content.InteractionSettings {
	return content.SettingsOf(p)
}

// InteractionPolicyRepository stores the interaction policies of statuses
// (status_interaction_policies); the status repositories implement it.
type InteractionPolicyRepository interface {
	// ReplaceInteractionPolicy replaces in tx the rules of the kinds of a status with rules,
	// which are of those kinds: one statement removes the rules and one inserts the new ones.
	// No kinds only inserts, for a new status.
	ReplaceInteractionPolicy(ctx context.Context, tx uow.Tx, statusID string, kinds []content.InteractionKind, rules content.InteractionPolicy) error
	// FindInteractionPolicies returns the rules of the statuses of statusIDs, by status ID, with
	// one query; a status without rules is absent. FindInteractionPolicy returns those of one,
	// none when it has none.
	FindInteractionPolicies(ctx context.Context, statusIDs []string) (map[string]content.InteractionPolicy, error)
	FindInteractionPolicy(ctx context.Context, statusID string) (content.InteractionPolicy, error)
}

// InteractionPolicyChange is a change of the settings of some kinds; the kinds left out keep
// their settings.
type InteractionPolicyChange map[content.InteractionKind]content.InteractionSetting

// Validate reports ErrInvalidInteractionPolicy for an unknown kind or a setting a local author
// may not choose.
func (c InteractionPolicyChange) Validate() error {
	for k, set := range c {
		if !k.Valid() {
			return fmt.Errorf("%w: kind %q", ErrInvalidInteractionPolicy, k)
		}
		if err := set.Validate(); err != nil {
			return fmt.Errorf("%s: %w", k, err)
		}
	}
	return nil
}

// apply returns s with the settings of c.
func (c InteractionPolicyChange) apply(s content.InteractionSettings) content.InteractionSettings {
	for k, set := range c {
		s = s.With(k, set)
	}
	return s
}

// kinds returns the kinds of c in their order.
func (c InteractionPolicyChange) kinds() []content.InteractionKind {
	keys := slices.Collect(maps.Keys(c))
	return slices.DeleteFunc(slices.Clone(content.InteractionKinds), func(k content.InteractionKind) bool { return !slices.Contains(keys, k) })
}

// Event of the interaction policies (data-model §4.4):
//   - EventInteractionPolicyChanged: the author of a local status changed who may interact with
//     it. The federation module delivers an Update of the status when its Note says the policy
//     of a changed kind.
const EventInteractionPolicyChanged = "content.interaction_policy_changed"

// InteractionPolicyChanged is emitted when the author of a local status changes its interaction
// policy.
type InteractionPolicyChanged struct {
	Status StatusRef
	// Kinds are the kinds whose settings changed, in their order.
	Kinds []content.InteractionKind
	// Settings are the status's settings after the change.
	Settings content.InteractionSettings
}

// EventType implements uow.Event.
func (InteractionPolicyChanged) EventType() string { return EventInteractionPolicyChanged }

// newInteractionSettings returns the settings of a new status of author: the author's defaults
// (DefaultInteractionSettings without a function to read them) with the requested change.
func (s *Service) newInteractionSettings(ctx context.Context, author appaccount.Actor, requested InteractionPolicyChange) (content.InteractionSettings, error) {
	if err := requested.Validate(); err != nil {
		return content.InteractionSettings{}, err
	}
	base := content.DefaultInteractionSettings
	if s.d.DefaultInteractionPolicy != nil && author.Local != nil && len(requested) < len(content.InteractionKinds) {
		def, err := s.d.DefaultInteractionPolicy(ctx, author.Local.UserID)
		if err != nil {
			return content.InteractionSettings{}, err
		}
		if def.Validate() == nil {
			base = def
		}
	}
	return requested.apply(base), nil
}

// SetInteractionPolicy changes who may interact with a local status of accountID by the kinds of
// change (ADR-0155 §3, §4) and returns its settings. Interactions approved before stay. It emits
// InteractionPolicyChanged with the kinds whose settings changed, unless none did. A status that
// does not exist or is deleted fails with ErrNotFound, another account's with ErrNotAuthor, and a
// change a local author may not choose with ErrInvalidInteractionPolicy.
func (s *Service) SetInteractionPolicy(ctx context.Context, accountID, statusID string, change InteractionPolicyChange) (content.InteractionSettings, error) {
	if err := change.Validate(); err != nil {
		return content.InteractionSettings{}, err
	}
	if s.d.Quotes == nil {
		return content.InteractionSettings{}, fmt.Errorf("status %s: %w", statusID, ErrNotFound)
	}
	st, err := s.ownStatus(ctx, accountID, statusID)
	if err != nil {
		return content.InteractionSettings{}, err
	}
	cur, err := s.d.Quotes.FindInteractionPolicy(ctx, st.ID)
	if err != nil {
		return content.InteractionSettings{}, err
	}
	before := content.SettingsOf(cur)
	after := change.apply(before)
	var changed []content.InteractionKind
	for _, k := range change.kinds() {
		if before.Of(k) != after.Of(k) || !cur.Declares(k) {
			changed = append(changed, k)
		}
	}
	if len(changed) == 0 {
		return before, nil
	}
	var rules content.InteractionPolicy
	for _, r := range after.Policy() {
		if slices.Contains(changed, r.Kind) {
			rules = append(rules, r)
		}
	}
	err = s.d.UnitOfWork.Tx(ctx, func(tx uow.Tx) error {
		deleted, err := s.d.Repository.IsDeleted(ctx, tx, st.ID)
		if err != nil {
			return err
		}
		if deleted {
			return fmt.Errorf("status %s: %w", st.ID, ErrNotFound)
		}
		if err := s.d.Quotes.ReplaceInteractionPolicy(ctx, tx, st.ID, changed, rules); err != nil {
			return err
		}
		return tx.Emit(ctx, InteractionPolicyChanged{Status: RefOf(st), Kinds: changed, Settings: after})
	})
	if err != nil {
		return content.InteractionSettings{}, err
	}
	if s.d.ReconsiderRequests != nil {
		// The pending requests that the new rules admit at once are approved (ADR-0155 §3). One
		// that this misses keeps waiting for the author's answer, so the change stands either way.
		_ = s.d.ReconsiderRequests(ctx, st.ID)
	}
	return after, nil
}

// FindInteractionPolicy returns the rules of a status: its author's choice for a local status,
// what a remote status declares; none for a remote status that declares none.
func (s *Service) FindInteractionPolicy(ctx context.Context, statusID string) (content.InteractionPolicy, error) {
	if s.d.Quotes == nil {
		return nil, nil
	}
	return s.d.Quotes.FindInteractionPolicy(ctx, statusID)
}

// recordRemoteInteractionPolicy replaces in tx the rules of the remote status st with what its
// interactionPolicy declares now, rules (none when it declares none). A new status (created)
// has nothing to replace.
func (s *Service) recordRemoteInteractionPolicy(ctx context.Context, tx uow.Tx, st content.Status, rules content.InteractionPolicy, created bool) error {
	if s.d.Quotes == nil {
		return nil
	}
	rules = slices.DeleteFunc(slices.Clone(rules), func(r content.InteractionRule) bool {
		return !r.Kind.Valid() || !r.Approval.Valid() || !r.Audience.Valid() || (r.Audience == content.AudienceAccount) != (r.ActorURI != "")
	})
	kinds := content.InteractionKinds
	if created {
		if len(rules) == 0 {
			return nil
		}
		kinds = nil
	}
	return s.d.Quotes.ReplaceInteractionPolicy(ctx, tx, st.ID, kinds, rules)
}

// interactionPolicyOf returns the rules of statusID, none when it has none or there are no
// quotes to store them with.
func interactionPolicyOf(ctx context.Context, repo QuoteRepository, statusID string) (content.InteractionPolicy, error) {
	if repo == nil {
		return nil, nil
	}
	p, err := repo.FindInteractionPolicy(ctx, statusID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return p, err
}
