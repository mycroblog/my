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
// was suspended (relationship_severances, FR-717, ADR-0058): the suspension ends every follow
// between the account and the suspended accounts, in both directions, and a severance counts
// the active ones that the account lost. Lifting the suspension does not restore them.
type Severance struct {
	ID string
	// DomainActionID is the suspension of a domain, ActionID that of a remote account; exactly
	// one is set.
	DomainActionID string
	ActionID       string
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
	case (s.DomainActionID == "") == (s.ActionID == ""):
		return fmt.Errorf("%w: a severance has exactly one action", ErrInvalidSeverance)
	case s.FollowersLost < 0 || s.FollowingLost < 0 || s.FollowersLost+s.FollowingLost == 0:
		return fmt.Errorf("%w: a severance counts at least one follow and none below zero", ErrInvalidSeverance)
	}
	return nil
}

// SeveranceRecord is a severance with what its action suspended.
type SeveranceRecord struct {
	Severance
	// Domain is the domain that the domain action suspended, "" for an action on an account.
	Domain string
	// TargetAccountID is the remote account that the action suspended, "" for a domain action.
	TargetAccountID string
}
