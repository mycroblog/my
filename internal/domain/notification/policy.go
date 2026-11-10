// SPDX-License-Identifier: AGPL-3.0-only

package notification

import (
	"errors"
	"time"
)

// The notification policy (ADR-0158 §1-§7): for each condition on the source of a notification,
// what the recipient wants done with the notifications that meet it, and the requests in which
// the filtered notifications wait, grouped by source.

// Condition is a condition of the notification policy on the source of a notification and its
// relationship with the recipient (notification_policies.policy_condition).
type Condition string

// Conditions (ADR-0158 §1).
const (
	// ConditionNotFollowing: the recipient does not follow the source with an accepted follow.
	ConditionNotFollowing Condition = "not_following"
	// ConditionNotFollower: the source does not follow the recipient with an accepted follow.
	ConditionNotFollower Condition = "not_follower"
	// ConditionNewAccount: the source's account was created less than NewAccountAge ago.
	ConditionNewAccount Condition = "new_account"
	// ConditionUnsolicitedPrivateMention: a mention in a direct post that does not answer an
	// exchange the recipient started.
	ConditionUnsolicitedPrivateMention Condition = "unsolicited_private_mention"
	// ConditionLimitedAccount: a moderator limited the source or its domain (unsolicited_reach).
	ConditionLimitedAccount Condition = "limited_account"
)

// Conditions are the conditions of the policy, in the order the API and the settings show them.
var Conditions = []Condition{
	ConditionNotFollowing, ConditionNotFollower, ConditionNewAccount, ConditionUnsolicitedPrivateMention, ConditionLimitedAccount,
}

// Valid reports whether c is a known condition.
func (c Condition) Valid() bool {
	for _, k := range Conditions {
		if c == k {
			return true
		}
	}
	return false
}

// Action is what the policy does with a notification (notification_policies.policy_action).
type Action string

// Actions (ADR-0158 §2), from the mildest to the strictest.
const (
	// ActionAccept lists the notification and delivers it as the settings say.
	ActionAccept Action = "accept"
	// ActionFilter creates the notification in its source's request, out of the list and the
	// unread count, and delivers it nowhere.
	ActionFilter Action = "filter"
	// ActionDrop creates no notification.
	ActionDrop Action = "drop"
)

// Valid reports whether a is a known action.
func (a Action) Valid() bool { return a == ActionAccept || a == ActionFilter || a == ActionDrop }

// strictness orders the actions: the strictest of those that match wins.
func (a Action) strictness() int {
	switch a {
	case ActionDrop:
		return 2
	case ActionFilter:
		return 1
	}
	return 0
}

// NewAccountAge is how young a source's account is to meet ConditionNewAccount. It is fixed, as
// in Mastodon.
const NewAccountAge = 30 * 24 * time.Hour

// ErrInvalidPolicy reports a policy choice of an unknown condition or action.
var ErrInvalidPolicy = errors.New("notification: unknown policy condition or action")

// PolicyChoice is a recipient's action for one condition (notification_policies, a current
// value). A condition without a choice accepts.
type PolicyChoice struct {
	AccountID string
	Condition Condition
	Action    Action
	UpdatedAt time.Time
}

// Validate checks the condition and the action.
func (c PolicyChoice) Validate() error {
	if !c.Condition.Valid() || !c.Action.Valid() {
		return ErrInvalidPolicy
	}
	return nil
}

// Policy is a recipient's action for each condition; the zero Policy accepts everything.
type Policy struct {
	actions map[Condition]Action
}

// NewPolicy returns the policy of the saved choices; unknown ones are ignored.
func NewPolicy(choices []PolicyChoice) Policy {
	p := Policy{actions: map[Condition]Action{}}
	for _, c := range choices {
		if c.Validate() == nil {
			p.actions[c.Condition] = c.Action
		}
	}
	return p
}

// Action returns the action for c: the choice, or ActionAccept without one.
func (p Policy) Action(c Condition) Action {
	if a, ok := p.actions[c]; ok {
		return a
	}
	return ActionAccept
}

// Strict reports whether some condition does not accept: without one, no notification needs
// its facts read.
func (p Policy) Strict() bool {
	for _, c := range Conditions {
		if p.Action(c) != ActionAccept {
			return true
		}
	}
	return false
}

// Needs reports whether the action of c is not ActionAccept, so that its fact decides something.
func (p Policy) Needs(c Condition) bool { return p.Action(c) != ActionAccept }

// PolicyApplies reports whether the policy classifies notifications of type t: those that the
// actions of other users make (ADR-0158 §3). The notifications that the user asked for, those of
// the moderators, and the notices of the instance (moderation, appeals, closures, supporters,
// announcements) are never filtered or dropped.
func PolicyApplies(t Type) bool {
	switch t {
	case TypeFollow, TypeFollowRequest, TypeMention, TypeBoost, TypeFavourite, TypeReaction, TypeQuote, TypeVoiceRoomInvited:
		return true
	}
	return false
}

// Facts are what the conditions read of a notification's source and its relationship with the
// recipient. A fact that the policy does not need may be left false.
type Facts struct {
	// Accepted: the recipient accepted the source's request; it meets no condition.
	Accepted bool
	// Following: the recipient follows the source with an accepted follow.
	Following bool
	// Follower: the source follows the recipient with an accepted follow.
	Follower bool
	// NewAccount: the source's account is younger than NewAccountAge.
	NewAccount bool
	// UnsolicitedPrivateMention: the notification is a mention in a direct post that does not
	// answer an exchange the recipient started.
	UnsolicitedPrivateMention bool
	// LimitedAccount: the source or its domain has unsolicited_reach.
	LimitedAccount bool
}

// Matches returns the conditions that f meets, in the order of Conditions. A source that the
// recipient accepted meets none; one that the recipient follows meets none but
// ConditionLimitedAccount (ADR-0158 §1, §5).
func (f Facts) Matches() []Condition {
	if f.Accepted {
		return nil
	}
	var out []Condition
	if !f.Following {
		out = append(out, ConditionNotFollowing)
		if !f.Follower {
			out = append(out, ConditionNotFollower)
		}
		if f.NewAccount {
			out = append(out, ConditionNewAccount)
		}
		if f.UnsolicitedPrivateMention {
			out = append(out, ConditionUnsolicitedPrivateMention)
		}
	}
	if f.LimitedAccount {
		out = append(out, ConditionLimitedAccount)
	}
	return out
}

// Classify returns what p does with a notification of type t from a source with facts f: the
// strictest action of the conditions it meets, ActionAccept when it meets none or the policy does
// not apply to t. It is the one place that decides it (ADR-0158 §4).
func Classify(t Type, p Policy, f Facts) Action {
	if !PolicyApplies(t) {
		return ActionAccept
	}
	out := ActionAccept
	for _, c := range f.Matches() {
		if a := p.Action(c); a.strictness() > out.strictness() {
			out = a
		}
	}
	return out
}

// NewAccountAt reports whether an account created at createdAt is new at now (ConditionNewAccount).
// A time in the future does not make it new.
func NewAccountAt(createdAt, now time.Time) bool {
	return !createdAt.After(now) && now.Sub(createdAt) < NewAccountAge
}

// AcceptedSource is a source whose notifications the recipient accepted from its request
// (notification_accepted_sources, a resource): it meets no condition from then on.
type AcceptedSource struct {
	RecipientAccountID string
	SourceAccountID    string
	CreatedAt          time.Time
}

// RequestDismissal is the event that the recipient dismissed the request of a source
// (notification_request_dismissals): the filtered notifications up to and including
// LastNotificationID leave the request and are never listed.
type RequestDismissal struct {
	ID                 string
	RecipientAccountID string
	SourceAccountID    string
	LastNotificationID string
	DismissedAt        time.Time
}

// Request is the filtered notifications of one source that wait for the recipient
// (notification_requests, a projection of the filtered notifications, the accepted sources, and
// the dismissals of the requests).
type Request struct {
	RecipientAccountID string
	SourceAccountID    string
	NotificationsCount int64
	LastNotificationID string
	// Last is the notification LastNotificationID, where the read returns it (the lists of the
	// requests); zero elsewhere.
	Last Notification
}

// RequestSummary counts the requests of a recipient: the sources and their notifications.
type RequestSummary struct {
	Accounts      int64
	Notifications int64
}
