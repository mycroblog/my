// SPDX-License-Identifier: AGPL-3.0-only

package moderation

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidSeverance reports a severance that breaks its invariants.
var ErrInvalidSeverance = errors.New("moderation: invalid relationship severance")

// Severance is the event that a local account lost follows when a domain or a remote account
// was suspended, or when the allowlist cut domains off (relationship_severances, FR-717,
// ADR-0058, ADR-0159 §3): the suspension or the cutoff ends every follow between the account and
// the accounts it covers, in both directions, and a severance counts the active ones that the
// account lost. Lifting the suspension or allowing the domain again does not restore them.
type Severance struct {
	ID string
	// DomainActionID is the suspension of a domain, ActionID that of a remote account, and
	// AllowlistCutoffID the cutoff of the allowlist; exactly one is set.
	DomainActionID    string
	ActionID          string
	AllowlistCutoffID string
	// AccountID is the local account that lost the follows.
	AccountID string
	// FollowersLost counts the suspended accounts that followed AccountID, FollowingLost those
	// that AccountID followed.
	FollowersLost int
	FollowingLost int
	OccurredAt    time.Time
}

// Validate checks the invariants of a new severance: an ID, an account, exactly one action,
// and counts that are not negative and not both zero (a suspension that cut nothing of an
// account records nothing for it).
func (s Severance) Validate() error {
	switch {
	case s.ID == "" || s.AccountID == "":
		return fmt.Errorf("%w: a severance needs an ID and an account", ErrInvalidSeverance)
	case countSet(s.DomainActionID, s.ActionID, s.AllowlistCutoffID) != 1:
		return fmt.Errorf("%w: a severance has exactly one action", ErrInvalidSeverance)
	case s.FollowersLost < 0 || s.FollowingLost < 0 || s.FollowersLost+s.FollowingLost == 0:
		return fmt.Errorf("%w: a severance counts at least one follow and none below zero", ErrInvalidSeverance)
	}
	return nil
}

// countSet returns how many of ids are not empty.
func countSet(ids ...string) int {
	n := 0
	for _, id := range ids {
		if id != "" {
			n++
		}
	}
	return n
}

// SeveranceRecord is a severance with what its action suspended.
type SeveranceRecord struct {
	Severance
	// Domain is the domain that the domain action suspended, or the domain whose allow was
	// removed for a cutoff of CutoffAllowRemoved; "" for an action on an account and for the
	// cutoff of a switch to the allowlist.
	Domain string
	// TargetAccountID is the remote account that the action suspended, "" for a domain action.
	TargetAccountID string
}
