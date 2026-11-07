// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// The bounds of an automatic deletion policy (FR-118).
const (
	// MinAutoDeleteAgeDays and MaxAutoDeleteAgeDays bound how many days after its publication a
	// post is deleted.
	MinAutoDeleteAgeDays = 1
	MaxAutoDeleteAgeDays = 3650
	// MaxAutoDeleteThreshold bounds the boosts and favourites thresholds.
	MaxAutoDeleteThreshold = 1_000_000
	// DefaultAutoDeleteAgeDays is the age of a policy that has never been saved.
	DefaultAutoDeleteAgeDays = 30
)

// ErrInvalidAutoDeletePolicy reports a policy value out of its bounds.
var ErrInvalidAutoDeletePolicy = errors.New("content: invalid automatic deletion policy")

// AutoDeletePolicyError names the property of a policy that is invalid.
type AutoDeletePolicyError struct {
	// Property is the snake_case name of the property (min_age_days, keep_visibilities, ...).
	Property string
	Err      error
}

func (e *AutoDeletePolicyError) Error() string { return fmt.Sprintf("%s: %v", e.Property, e.Err) }

func (e *AutoDeletePolicyError) Unwrap() error { return e.Err }

// AutoDeletePolicy is a local account's automatic deletion of its own old posts (FR-118): once
// enabled, a post published more than MinAgeDays days ago is deleted as its author would delete
// it, unless one of the exceptions keeps it. Boosts are not posts and are never deleted by it.
type AutoDeletePolicy struct {
	AccountID string
	Enabled   bool
	// MinAgeDays is how many days after its publication a post is deleted.
	MinAgeDays int
	// The exceptions: a post the author pinned, bookmarked, or favourited, one whose current
	// revision has media attachments or a poll, and one posted with a visibility in
	// KeepVisibilities (a set, in the order of Visibilities).
	KeepPinned       bool
	KeepBookmarked   bool
	KeepFavourited   bool
	KeepMedia        bool
	KeepPolls        bool
	KeepVisibilities []Visibility
	// KeepMinBoosts and KeepMinFavourites keep a post boosted or favourited at least that many
	// times; zero sets no threshold.
	KeepMinBoosts     int
	KeepMinFavourites int
	// MaxKeptPins, when positive, keeps only that many of the author's pins, the newest first,
	// while KeepPinned is on: the server maximum sets it on the effective policy
	// (EffectiveAutoDeletePolicy); a saved policy has zero, which keeps every pin.
	MaxKeptPins int
	UpdatedAt   time.Time
}

// DefaultAutoDeletePolicy is the policy of an account that has never saved one: disabled, with
// the exceptions that keep what the author marked (pins, bookmarks, favourites) and direct posts.
func DefaultAutoDeletePolicy(accountID string) AutoDeletePolicy {
	return AutoDeletePolicy{
		AccountID: accountID, MinAgeDays: DefaultAutoDeleteAgeDays,
		KeepPinned: true, KeepBookmarked: true, KeepFavourited: true,
		KeepVisibilities: []Visibility{VisibilityDirect},
	}
}

// Normalize validates p and returns it with KeepVisibilities as a set in the order of
// Visibilities. It fails with an *AutoDeletePolicyError.
func (p AutoDeletePolicy) Normalize() (AutoDeletePolicy, error) {
	if p.MinAgeDays < MinAutoDeleteAgeDays || p.MinAgeDays > MaxAutoDeleteAgeDays {
		return p, &AutoDeletePolicyError{Property: "min_age_days", Err: fmt.Errorf("%w: %d days", ErrInvalidAutoDeletePolicy, p.MinAgeDays)}
	}
	for name, n := range map[string]int{"keep_min_boosts": p.KeepMinBoosts, "keep_min_favourites": p.KeepMinFavourites} {
		if n < 0 || n > MaxAutoDeleteThreshold {
			return p, &AutoDeletePolicyError{Property: name, Err: fmt.Errorf("%w: %d", ErrInvalidAutoDeletePolicy, n)}
		}
	}
	kept := make([]Visibility, 0, len(p.KeepVisibilities))
	for _, v := range Visibilities() {
		if slices.Contains(p.KeepVisibilities, v) {
			kept = append(kept, v)
		}
	}
	for _, v := range p.KeepVisibilities {
		if !v.Valid() {
			return p, &AutoDeletePolicyError{Property: "keep_visibilities", Err: fmt.Errorf("%w: %q", ErrInvalidVisibility, v)}
		}
	}
	p.KeepVisibilities = kept
	return p, nil
}

// Cutoff is the publication time before which the policy deletes a post at now.
func (p AutoDeletePolicy) Cutoff(now time.Time) time.Time {
	return now.AddDate(0, 0, -p.MinAgeDays)
}
