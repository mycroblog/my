// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Appeals (FR-707, ADR-0038): a local account disputes an action on it once, within the
// instance's appeal window from the action (a ban, also before its account is deleted), and a
// moderator upholds or overturns it. Overturning lifts the action, a ban included, and cancels
// the deletion a ban scheduled; the statuses that delete_statuses deleted stay deleted.

// AppealState is where an appeal stands (appeal_states.state).
type AppealState string

// States of an appeal.
const (
	AppealPending    AppealState = "pending"
	AppealUpheld     AppealState = "upheld"
	AppealOverturned AppealState = "overturned"
)

// Valid reports whether s is one of the states of an appeal.
func (s AppealState) Valid() bool {
	switch s {
	case AppealPending, AppealUpheld, AppealOverturned:
		return true
	}
	return false
}

// AppealDecisionKind is how a moderator decided an appeal (moderation_appeal_decisions.decision).
type AppealDecisionKind string

// Decisions on an appeal.
const (
	// AppealDecisionUpheld keeps the action as it is.
	AppealDecisionUpheld AppealDecisionKind = "upheld"
	// AppealDecisionOverturned lifts the action.
	AppealDecisionOverturned AppealDecisionKind = "overturned"
)

// Valid reports whether k is one of the decisions.
func (k AppealDecisionKind) Valid() bool {
	return k == AppealDecisionUpheld || k == AppealDecisionOverturned
}

// State returns the state of an appeal with decision k.
func (k AppealDecisionKind) State() AppealState { return AppealState(k) }

// MaxAppealText is the number of characters of an appeal's text and of a decision's reason.
const MaxAppealText = 2000

// DefaultAppealWindow is how long after an action it can be appealed when nothing sets the
// window: the default of the instance setting appeal_window_days (ADR-0038).
const DefaultAppealWindow = 180 * 24 * time.Hour

// AppealDeadline returns the last moment action a can be appealed in a window of window from the
// action: for a ban, also no later than the deletion of its account, BanDeletionDelay after it
// (the decider: 「30 日のあいだは、BAN への異議申し立てを受け付ける」).
func AppealDeadline(a Action, window time.Duration) time.Time {
	if a.Kind == ActionBan {
		window = min(window, BanDeletionDelay)
	}
	return a.IssuedAt.Add(window)
}

// ErrInvalidAppeal reports an appeal or a decision with a missing or invalid field.
var ErrInvalidAppeal = errors.New("moderation: invalid appeal")

// Appeal is a local account's appeal of an action on it (moderation_appeals, a resource). An
// action has at most one.
type Appeal struct {
	ID       string
	ActionID string
	// AccountID is the account that appealed: the action's target.
	AccountID   string
	Text        string
	SubmittedAt time.Time
}

// Validate checks the invariants of a new appeal: its fields and a text of 1 to MaxAppealText
// characters that is not only white space.
func (a Appeal) Validate() error {
	if a.ID == "" || a.ActionID == "" || a.AccountID == "" || a.SubmittedAt.IsZero() {
		return fmt.Errorf("%w: a field is empty", ErrInvalidAppeal)
	}
	if err := checkText("text", a.Text, MaxAppealText); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidAppeal, err)
	}
	if strings.TrimSpace(a.Text) == "" {
		return fmt.Errorf("%w: %w", ErrInvalidAppeal, &TextError{Field: "text"})
	}
	return nil
}

// AppealDecision is a moderator's decision on an appeal (moderation_appeal_decisions, an
// event). An appeal is decided once.
type AppealDecision struct {
	AppealID string
	Decision AppealDecisionKind
	// PublicReason is the moderators' explanation to the account; "" for none.
	PublicReason string
	// DecidedByUserID is the moderator; "" for the CLI.
	DecidedByUserID string
	DecidedAt       time.Time
}

// Validate checks the invariants of a new decision: its appeal, a known decision, its time, and
// a valid reason.
func (d AppealDecision) Validate() error {
	switch {
	case d.AppealID == "" || d.DecidedAt.IsZero():
		return fmt.Errorf("%w: a field of the decision is empty", ErrInvalidAppeal)
	case !d.Decision.Valid():
		return fmt.Errorf("%w: decision %q", ErrInvalidAppeal, d.Decision)
	}
	if err := checkText("public_reason", d.PublicReason, MaxAppealText); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidAppeal, err)
	}
	return nil
}

// AppealStatus is the current state of an appeal (appeal_states, a projection).
type AppealStatus struct {
	AppealID string
	State    AppealState
	// Since is when the appeal entered its state: when it was submitted or decided.
	Since time.Time
}

// AppealRecord is an appeal with its decision, as the moderators and the account read it.
type AppealRecord struct {
	Appeal   Appeal
	Decision *AppealDecision // nil while pending
}

// State returns where the appeal stands.
func (r AppealRecord) State() AppealState {
	if r.Decision == nil {
		return AppealPending
	}
	return r.Decision.Decision.State()
}
