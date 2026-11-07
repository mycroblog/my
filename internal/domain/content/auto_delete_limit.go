// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// The bounds of the server maximum of the automatic deletion (ADR-0151 §3).
const (
	// MaxAutoDeleteGraceDays bounds the grace period of a newly bound account.
	MaxAutoDeleteGraceDays = 365
	// DefaultAutoDeleteGraceDays is the grace period until the operator sets one.
	DefaultAutoDeleteGraceDays = 30
)

// ErrAutoDeleteOverLimit reports a policy that the server maximum does not allow: turning the
// automatic deletion off, keeping posts longer than the maximum, or an exception that the
// operator does not allow.
var ErrAutoDeleteOverLimit = errors.New("content: automatic deletion policy over the server maximum")

// AutoDeleteLimit is the server maximum of the automatic deletion (ADR-0151 §3): while MaxAgeDays
// is positive, no policy of an account without the permission keep_posts_longer keeps a post
// longer than MaxAgeDays days, and only the exceptions that the operator allows keep posts.
type AutoDeleteLimit struct {
	// MaxAgeDays is the most days any policy keeps a post; zero sets no maximum.
	MaxAgeDays int
	// AllowPinned lets the pins keep posts, KeptPins of them, the newest pins first.
	AllowPinned bool
	KeptPins    int
	// AllowBookmarked, AllowFavourited, AllowMedia, and AllowPolls let those exceptions keep
	// posts; AllowVisibilities the kept visibilities, and AllowThresholds the thresholds of
	// boosts and favourites.
	AllowBookmarked, AllowFavourited, AllowMedia, AllowPolls bool
	AllowVisibilities, AllowThresholds                       bool
	// GraceDays is how many days after the notice a newly bound account is held to the maximum
	// (§5).
	GraceDays int
}

// Enabled reports whether the maximum is on.
func (l AutoDeleteLimit) Enabled() bool { return l.MaxAgeDays > 0 }

// Applies reports whether the maximum holds an account: it is on, and the account does not have
// keep_posts_longer (exempt).
func (l AutoDeleteLimit) Applies(exempt bool) bool { return l.Enabled() && !exempt }

// Grace is the grace period of a newly bound account.
func (l AutoDeleteLimit) Grace() time.Duration { return time.Duration(l.GraceDays) * 24 * time.Hour }

// Check reports the first property of the saved policy p that the maximum does not allow, as an
// *AutoDeletePolicyError wrapping ErrAutoDeleteOverLimit, or nil when p is within it (ADR-0151 §3).
// Only an account that the maximum applies to is checked.
func (l AutoDeleteLimit) Check(p AutoDeletePolicy) error {
	if !l.Enabled() {
		return nil
	}
	over := func(property string) error {
		return &AutoDeletePolicyError{Property: property, Err: fmt.Errorf("%w: the maximum is %d days", ErrAutoDeleteOverLimit, l.MaxAgeDays)}
	}
	switch {
	case !p.Enabled:
		return over("enabled")
	case p.MinAgeDays > l.MaxAgeDays:
		return over("min_age_days")
	case p.KeepPinned && !l.AllowPinned:
		return over("keep_pinned")
	case p.KeepBookmarked && !l.AllowBookmarked:
		return over("keep_bookmarked")
	case p.KeepFavourited && !l.AllowFavourited:
		return over("keep_favourited")
	case p.KeepMedia && !l.AllowMedia:
		return over("keep_media")
	case p.KeepPolls && !l.AllowPolls:
		return over("keep_polls")
	case len(p.KeepVisibilities) > 0 && !l.AllowVisibilities:
		return over("keep_visibilities")
	case p.KeepMinBoosts > 0 && !l.AllowThresholds:
		return over("keep_min_boosts")
	case p.KeepMinFavourites > 0 && !l.AllowThresholds:
		return over("keep_min_favourites")
	}
	return nil
}

// EffectiveAutoDeletePolicy is the policy that the automatic deletion uses for an account whose
// saved policy is saved, under the server maximum limit, exempt when the account has the
// permission keep_posts_longer (ADR-0151 §4). Without a maximum, or exempt, it is saved itself.
// Otherwise it is enabled; it deletes after the saved days when saved is enabled and they are
// fewer than the maximum, after the maximum otherwise; and it keeps only the exceptions of saved
// that the operator allows, the newest KeptPins pins of the pins.
func EffectiveAutoDeletePolicy(saved AutoDeletePolicy, limit AutoDeleteLimit, exempt bool) AutoDeletePolicy {
	if !limit.Applies(exempt) {
		return saved
	}
	p := saved
	p.Enabled = true
	if !saved.Enabled || saved.MinAgeDays > limit.MaxAgeDays {
		p.MinAgeDays = limit.MaxAgeDays
	}
	p.KeepPinned = saved.KeepPinned && limit.AllowPinned
	p.MaxKeptPins = 0
	if p.KeepPinned && limit.KeptPins < MaxPins {
		p.MaxKeptPins = max(limit.KeptPins, 1)
	}
	p.KeepBookmarked = saved.KeepBookmarked && limit.AllowBookmarked
	p.KeepFavourited = saved.KeepFavourited && limit.AllowFavourited
	p.KeepMedia = saved.KeepMedia && limit.AllowMedia
	p.KeepPolls = saved.KeepPolls && limit.AllowPolls
	if !limit.AllowVisibilities {
		p.KeepVisibilities = nil
	}
	if !limit.AllowThresholds {
		p.KeepMinBoosts, p.KeepMinFavourites = 0, 0
	}
	return p
}

// AutoDeleteBound reports whether an account is bound by the maximum (ADR-0151 §5): its effective
// policy deletes posts that its saved policy keeps. That is when they differ in anything but the
// cap on the kept pins, or when the cap leaves out some of the account's pins.
func AutoDeleteBound(saved, effective AutoDeletePolicy, pins int) bool {
	if effective.MaxKeptPins > 0 && pins > effective.MaxKeptPins {
		return true
	}
	return !saved.sameRules(effective)
}

// sameRules reports whether p and q keep the same posts apart from the cap on the kept pins.
func (p AutoDeletePolicy) sameRules(q AutoDeletePolicy) bool {
	return p.Enabled == q.Enabled && p.MinAgeDays == q.MinAgeDays &&
		p.KeepPinned == q.KeepPinned && p.KeepBookmarked == q.KeepBookmarked && p.KeepFavourited == q.KeepFavourited &&
		p.KeepMedia == q.KeepMedia && p.KeepPolls == q.KeepPolls && slices.Equal(p.KeepVisibilities, q.KeepVisibilities) &&
		p.KeepMinBoosts == q.KeepMinBoosts && p.KeepMinFavourites == q.KeepMinFavourites
}

// AutoDeleteLimitNotice is the advance notice to an account that the maximum binds it
// (status_auto_delete_limit_notices, an event, ADR-0151 §5): from EnforceFrom, its posts go by
// its effective policy, which keeps them at most MaxAgeDays days. The notice in effect of a bound
// account (status_auto_delete_limits, its projection) reads back without NoticedAt.
type AutoDeleteLimitNotice struct {
	ID          string
	AccountID   string
	MaxAgeDays  int
	NoticedAt   time.Time
	EnforceFrom time.Time
}

// NewAutoDeleteLimitNotice is the notice with id to the account that limit newly binds at now,
// held to it after the grace period.
func NewAutoDeleteLimitNotice(id, accountID string, limit AutoDeleteLimit, now time.Time) AutoDeleteLimitNotice {
	return AutoDeleteLimitNotice{ID: id, AccountID: accountID, MaxAgeDays: limit.MaxAgeDays, NoticedAt: now, EnforceFrom: now.Add(limit.Grace())}
}

// LoweredBy reports whether limit is lower than the maximum that n told of, so that a bound
// account whose notice in effect is n is due a new one.
func (n AutoDeleteLimitNotice) LoweredBy(limit AutoDeleteLimit) bool {
	return n.MaxAgeDays > limit.MaxAgeDays
}

// Enforced reports whether the account of the notice is held to its effective policy at now.
func (n AutoDeleteLimitNotice) Enforced(now time.Time) bool { return !now.Before(n.EnforceFrom) }
