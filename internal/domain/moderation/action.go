// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ActionKind is the kind of a moderation action on an account (moderation_actions.kind).
type ActionKind string

// Kinds of actions on an account (data-model §5.2).
const (
	ActionWarn                  ActionKind = "warn"
	ActionMarkStatusesSensitive ActionKind = "mark_statuses_sensitive"
	ActionDeleteStatuses        ActionKind = "delete_statuses"
	ActionForceSensitive        ActionKind = "force_sensitive"
	ActionLimit                 ActionKind = "limit"
	ActionRestrictPosting       ActionKind = "restrict_posting"
	ActionRestrictInteraction   ActionKind = "restrict_interaction"
	ActionDisableLogin          ActionKind = "disable_login"
	ActionSuspend               ActionKind = "suspend"
	ActionBan                   ActionKind = "ban"
)

// accountRestrictions is the table of data-model §5.2: the restrictions each kind of action
// imposes on its account. Changing it requires rebuilding account_restrictions.
var accountRestrictions = map[ActionKind][]Restriction{
	ActionWarn:                  nil,
	ActionMarkStatusesSensitive: nil,
	ActionDeleteStatuses:        nil,
	ActionForceSensitive:        {RestrictMediaSensitive},
	ActionLimit:                 {RestrictVisibilityCeilingPrivate, RestrictPublicDiscovery, RestrictUnsolicitedReach},
	ActionRestrictPosting:       {RestrictPost},
	ActionRestrictInteraction:   {RestrictInteract},
	ActionDisableLogin:          {RestrictLogin},
	ActionSuspend:               {RestrictLogin, RestrictPost, RestrictInteract, RestrictProfileView, RestrictFederation},
	ActionBan:                   {RestrictLogin, RestrictPost, RestrictInteract, RestrictProfileView, RestrictFederation},
}

// Valid reports whether k is one of the kinds of actions on an account.
func (k ActionKind) Valid() bool {
	_, ok := accountRestrictions[k]
	return ok
}

// Restrictions returns the restrictions an action of kind k imposes (data-model §5.2).
func (k ActionKind) Restrictions() []Restriction { return accountRestrictions[k] }

// OnStatuses reports whether an action of kind k acts on statuses of its account that it names
// (moderation_action_statuses): mark_statuses_sensitive and delete_statuses. Such an action
// names at least one status and never expires; the other kinds name none.
func (k ActionKind) OnStatuses() bool {
	return k == ActionMarkStatusesSensitive || k == ActionDeleteStatuses
}

// Liftable reports whether an action of kind k can be lifted: every kind but ban, which deletes
// its account (FR-703).
func (k ActionKind) Liftable() bool { return k != ActionBan }

// MaxActionStatuses is the number of statuses that one action on statuses names at most.
const MaxActionStatuses = 100

// BanDeletionDelay is how long after a ban its account is deleted: the ban records the
// account's deletion_scheduled lifecycle event due then (data-model §4.2, §8).
const BanDeletionDelay = 30 * 24 * time.Hour

// Category is the reason category of an action or a report.
type Category string

// Categories.
const (
	CategorySpam      Category = "spam"
	CategoryLegal     Category = "legal"
	CategoryViolation Category = "violation"
	CategoryOther     Category = "other"
)

// Valid reports whether c is one of the categories.
func (c Category) Valid() bool {
	switch c {
	case CategorySpam, CategoryLegal, CategoryViolation, CategoryOther:
		return true
	}
	return false
}

// Issuer is who issued an action on an account.
type Issuer string

// Issuers.
const (
	IssuerModerator Issuer = "moderator"
	IssuerSystem    Issuer = "system"
	IssuerRemote    Issuer = "remote"
)

// Valid reports whether i is one of the issuers.
func (i Issuer) Valid() bool {
	switch i {
	case IssuerModerator, IssuerSystem, IssuerRemote:
		return true
	}
	return false
}

var (
	// ErrInvalidAction reports an action with a missing or invalid field.
	ErrInvalidAction = errors.New("moderation: invalid action")
	// ErrActionNeverExpires reports an end on a kind of action that never expires: a ban and
	// the actions on statuses. It wraps ErrInvalidAction.
	ErrActionNeverExpires = fmt.Errorf("%w: the kind of action never expires", ErrInvalidAction)
	// ErrInvalidActionStatuses reports statuses of an action on statuses that are missing,
	// repeated, or too many, or statuses named by another kind. It wraps ErrInvalidAction.
	ErrInvalidActionStatuses = fmt.Errorf("%w: the statuses of the action", ErrInvalidAction)
)

// MaxDuration bounds how long a moderation action or a sign-up block lasts: ten years. A longer
// one is refused rather than kept until it is lifted.
const MaxDuration = 10 * 365 * 24 * time.Hour

// MaxDurationSeconds is MaxDuration in seconds, the bound of the expires_in of the client API.
const MaxDurationSeconds = int64(MaxDuration / time.Second)

// ValidDuration reports whether d is a duration that an action or a sign-up block may last:
// from 0 (until it is lifted) to MaxDuration.
func ValidDuration(d time.Duration) bool {
	return d >= 0 && d <= MaxDuration
}

// ValidDurationSeconds reports whether seconds is an expires_in of the client API: from 1 to
// MaxDurationSeconds. Callers convert it to a time.Duration only after this check, so that the
// conversion cannot overflow.
func ValidDurationSeconds(seconds int64) bool {
	return seconds >= 1 && seconds <= MaxDurationSeconds
}

// Action is one moderation action on an account (moderation_actions, an event). It is
// immutable; lifting it is a separate Lift event, and an expired action is not lifted: its
// restrictions end at ExpiresAt when they are read.
type Action struct {
	ID              string
	TargetAccountID string
	Kind            ActionKind
	ReasonCategory  Category
	PublicReason    string // shown to the target; "" for none
	PrivateNote     string // "" for none
	ReportID        string // "" when the action does not follow from a report
	Issuer          Issuer
	IssuedByUserID  string // "" for the system and the CLI
	NotifyTarget    bool
	ExpiresAt       *time.Time // nil: until lifted
	IssuedAt        time.Time
	// StatusIDs are the statuses of the target that an action on statuses acts on, in ID
	// order (moderation_action_statuses); nil for the other kinds.
	StatusIDs []string
}

// MaxActionText is the number of characters of each text of a moderation action and of a lift:
// an action's public reason or comment and its private note, and a lift's reason.
const MaxActionText = 5000

// Validate checks the invariants of a new action: a known kind, category, and issuer, valid
// texts, an end after its start, no end for a ban or an action on statuses, and from 1 to
// MaxActionStatuses distinct statuses for an action on statuses and none for another kind.
func (a Action) Validate() error {
	if err := checkTexts(textField{"public_reason", a.PublicReason}, textField{"private_note", a.PrivateNote}); err != nil {
		return err
	}
	switch {
	case a.ID == "" || a.TargetAccountID == "" || a.IssuedAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidAction)
	case !a.Kind.Valid():
		return fmt.Errorf("%w: kind %q", ErrInvalidAction, a.Kind)
	case !a.ReasonCategory.Valid():
		return fmt.Errorf("%w: category %q", ErrInvalidAction, a.ReasonCategory)
	case !a.Issuer.Valid():
		return fmt.Errorf("%w: issuer %q", ErrInvalidAction, a.Issuer)
	case (a.Kind == ActionBan || a.Kind.OnStatuses()) && a.ExpiresAt != nil:
		return fmt.Errorf("%w: %s", ErrActionNeverExpires, a.Kind)
	case a.ExpiresAt != nil && !a.ExpiresAt.After(a.IssuedAt):
		return fmt.Errorf("%w: it must expire after it is issued", ErrInvalidAction)
	case a.ExpiresAt != nil && a.ExpiresAt.Sub(a.IssuedAt) > MaxDuration:
		return fmt.Errorf("%w: it must expire within %s", ErrInvalidAction, MaxDuration)
	}
	return a.validateStatuses()
}

func (a Action) validateStatuses() error {
	if !a.Kind.OnStatuses() {
		if len(a.StatusIDs) > 0 {
			return fmt.Errorf("%w: %s names no statuses", ErrInvalidActionStatuses, a.Kind)
		}
		return nil
	}
	if len(a.StatusIDs) == 0 || len(a.StatusIDs) > MaxActionStatuses {
		return fmt.Errorf("%w: %s names 1 to %d statuses", ErrInvalidActionStatuses, a.Kind, MaxActionStatuses)
	}
	seen := make(map[string]bool, len(a.StatusIDs))
	for _, id := range a.StatusIDs {
		if id == "" || seen[id] {
			return fmt.Errorf("%w: status %q is empty or repeated", ErrInvalidActionStatuses, id)
		}
		seen[id] = true
	}
	return nil
}

// Rows returns the account_restrictions rows that a imposes.
func (a Action) Rows() []RestrictionRow {
	rs := a.Kind.Restrictions()
	out := make([]RestrictionRow, len(rs))
	for i, r := range rs {
		out[i] = RestrictionRow{Target: a.TargetAccountID, Restriction: r, SourceActionID: a.ID, EffectiveUntil: a.ExpiresAt}
	}
	return out
}

// Lift is the event that an action on an account was lifted (moderation_action_lifts).
type Lift struct {
	ActionID       string
	LiftedByUserID string // "" for the system
	Reason         string // "" for none
	LiftedAt       time.Time
}

// Validate checks the invariants of a new lift: its action, its time, and a valid reason.
func (l Lift) Validate() error {
	if l.ActionID == "" || l.LiftedAt.IsZero() {
		return fmt.Errorf("%w: a field of the lift is empty", ErrInvalidAction)
	}
	return checkTexts(textField{"reason", l.Reason})
}

// DomainActionKind is the kind of a moderation action on a domain
// (domain_moderation_actions.kind).
type DomainActionKind string

// Kinds of actions on a domain.
const (
	DomainLimit          DomainActionKind = "limit"
	DomainSuspend        DomainActionKind = "suspend"
	DomainRejectMedia    DomainActionKind = "reject_media"
	DomainRejectReports  DomainActionKind = "reject_reports"
	DomainForceSensitive DomainActionKind = "force_sensitive"
)

// domainRestrictions maps each kind of domain action to the restrictions it imposes
// (data-model §5.2).
var domainRestrictions = map[DomainActionKind][]Restriction{
	DomainLimit:          {RestrictPublicDiscovery, RestrictUnsolicitedReach},
	DomainSuspend:        {RestrictProfileView, RestrictFederation},
	DomainRejectMedia:    {RestrictMediaFetch},
	DomainRejectReports:  {RestrictReportIntake},
	DomainForceSensitive: {RestrictMediaSensitive},
}

// Valid reports whether k is one of the kinds of actions on a domain.
func (k DomainActionKind) Valid() bool {
	_, ok := domainRestrictions[k]
	return ok
}

// Restrictions returns the restrictions an action of kind k imposes on its domain.
func (k DomainActionKind) Restrictions() []Restriction { return domainRestrictions[k] }

// DomainAction is one moderation action on a domain and its subdomains
// (domain_moderation_actions, an event). Like Action, it is immutable and ends by a DomainLift
// or by expiring.
type DomainAction struct {
	ID string
	// Domain is a canonical host: lower-case IDNA A-labels without a trailing dot or a port.
	Domain        string
	Kind          DomainActionKind
	PublicComment string // "" for none
	PrivateNote   string // "" for none
	// Obfuscate hides part of the domain where the instance lists its domain actions publicly.
	Obfuscate      bool
	IssuedByUserID string     // "" for the CLI
	ExpiresAt      *time.Time // nil: until lifted
	IssuedAt       time.Time
}

// Validate checks the invariants of a new domain action.
func (a DomainAction) Validate() error {
	if err := checkTexts(textField{"public_comment", a.PublicComment}, textField{"private_note", a.PrivateNote}); err != nil {
		return err
	}
	switch {
	case a.ID == "" || a.IssuedAt.IsZero():
		return fmt.Errorf("%w: a field is empty", ErrInvalidAction)
	case !ValidDomain(a.Domain):
		return fmt.Errorf("%w: domain %q", ErrInvalidAction, a.Domain)
	case !a.Kind.Valid():
		return fmt.Errorf("%w: kind %q", ErrInvalidAction, a.Kind)
	case a.ExpiresAt != nil && !a.ExpiresAt.After(a.IssuedAt):
		return fmt.Errorf("%w: it must expire after it is issued", ErrInvalidAction)
	case a.ExpiresAt != nil && a.ExpiresAt.Sub(a.IssuedAt) > MaxDuration:
		return fmt.Errorf("%w: it must expire within %s", ErrInvalidAction, MaxDuration)
	}
	return nil
}

// Rows returns the domain_restrictions rows that a imposes.
func (a DomainAction) Rows() []RestrictionRow {
	rs := a.Kind.Restrictions()
	out := make([]RestrictionRow, len(rs))
	for i, r := range rs {
		out[i] = RestrictionRow{Target: a.Domain, Restriction: r, SourceActionID: a.ID, EffectiveUntil: a.ExpiresAt}
	}
	return out
}

// DomainLift is the event that a domain action was lifted (domain_moderation_action_lifts).
type DomainLift struct {
	ActionID       string
	LiftedByUserID string // "" for the CLI
	LiftedAt       time.Time
}

// ValidDomain reports whether d looks like a canonical host: non-empty, lower case, without a
// scheme, path, port, NUL, or empty label. Callers canonicalize hosts (IDNA A-labels) before.
func ValidDomain(d string) bool {
	if d == "" || len(d) > 253 || !validText(d) || d != strings.ToLower(d) || strings.ContainsAny(d, "/:@ \t\n") {
		return false
	}
	for label := range strings.SplitSeq(d, ".") {
		if label == "" {
			return false
		}
	}
	return true
}

// ActionState is whether an action on an account or a domain is in effect at some time.
type ActionState string

// States of an action.
const (
	ActionActive  ActionState = "active"
	ActionExpired ActionState = "expired"
	ActionLifted  ActionState = "lifted"
)

// stateAt returns the state at now of an action that ends at expiresAt (nil: until lifted) and
// is lifted when lifted is true. A lifted action is lifted even after it would have expired.
func stateAt(expiresAt *time.Time, lifted bool, now time.Time) ActionState {
	switch {
	case lifted:
		return ActionLifted
	case !InEffect(expiresAt, now):
		return ActionExpired
	}
	return ActionActive
}

// DomainActionRecord is a domain action with its lift, as the moderators list it.
type DomainActionRecord struct {
	Action DomainAction
	Lift   *DomainLift // nil until lifted
}

// State returns whether the action is in effect at now.
func (r DomainActionRecord) State(now time.Time) ActionState {
	return stateAt(r.Action.ExpiresAt, r.Lift != nil, now)
}

// ActionRecord is an action on an account with its lift, as the moderators list it.
type ActionRecord struct {
	Action Action
	Lift   *Lift // nil until lifted
}

// State returns whether the action is in effect at now.
func (r ActionRecord) State(now time.Time) ActionState {
	return stateAt(r.Action.ExpiresAt, r.Lift != nil, now)
}
