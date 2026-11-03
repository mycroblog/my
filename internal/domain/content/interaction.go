// SPDX-License-Identifier: AGPL-3.0-only

package content

import (
	"errors"
	"time"
)

// This file holds the interactions of accounts with statuses (FR-107, FR-113): boosts,
// favourites, bookmarks, and pins (data-model §4.4).

// ActivityURI returns the IRI of an activity of this instance other than Create, such as the
// Announce of a boost or the Like of a favourite (ADR-0018: /ap/activities/{uuid}).
func ActivityURI(host, id string) string {
	return "https://" + host + "/ap/activities/" + id
}

var (
	// ErrNotBoostable reports a status whose visibility does not allow the account to boost it.
	ErrNotBoostable = errors.New("content: the status cannot be boosted")
	// ErrNotPinnable reports a status that the account cannot pin: someone else's, or direct.
	ErrNotPinnable = errors.New("content: the status cannot be pinned")
	// ErrPinLimit reports an account that already pins MaxPins statuses.
	ErrPinLimit = errors.New("content: too many pinned statuses")
)

// MaxPins is the largest number of statuses one account pins to its profile (FR-403 Featured).
const MaxPins = 5

// Boostable reports whether a status of visibility v may be boosted by an account; isAuthor is
// true when that account wrote the status. Public, local, and unlisted statuses may be boosted
// by anyone who can see them; a followers-only status only by its author, to the author's own
// followers; a direct status by no one.
func (v Visibility) Boostable(isAuthor bool) bool {
	switch v {
	case VisibilityPublic, VisibilityLocal, VisibilityUnlisted:
		return true
	case VisibilityPrivate:
		return isAuthor
	default:
		return false
	}
}

// Pinnable reports whether a status of visibility v may be pinned by its author. Direct
// statuses may not: the Featured collection is shown to the author's profile visitors, each
// still filtered through CanView.
func (v Visibility) Pinnable() bool { return v.Valid() && v != VisibilityDirect }

// Boost is a boost (Announce) of a status: the immutable facts fixed when it is created.
// Visibility copies the original status's visibility, so a boost reaches no one the original
// does not.
type Boost struct {
	ID               string
	URI              string // the IRI of the Announce
	BoosterAccountID string
	StatusID         string
	Visibility       Visibility
	IsLocal          bool // the booster is a local account
	BoostedAt        time.Time
}

// Federates reports whether the boost is delivered to other servers. A boost of a local status
// never is (ADR-0017): its copied visibility marks it as one that stays on this instance.
func (b Boost) Federates() bool {
	return b.IsLocal && b.Visibility != VisibilityLocal && b.Visibility != VisibilityDirect
}

// CountsAsLocalPost reports whether the boost counts toward its booster's local posts, the
// usage.localPosts of NodeInfo. Mastodon and GoToSocial store a boost as a status and count
// those of local accounts.
func (b Boost) CountsAsLocalPost() bool { return b.IsLocal }

// BoostRetractionOrigin says why a boost was retracted.
type BoostRetractionOrigin string

// Boost retraction origins (boost_retractions.origin).
const (
	RetractedByBooster BoostRetractionOrigin = "booster"
	RetractedByRemote  BoostRetractionOrigin = "remote"
)

// BoostRetraction is the retraction of a boost (Undo Announce). A boost is retracted at most
// once; boosting the status again creates a new boost.
type BoostRetraction struct {
	BoostID     string
	Origin      BoostRetractionOrigin
	RetractedAt time.Time
}

// BoostEdge is the live boost of a status by an account (the boost_edges projection): a boost
// that is not retracted, of a status that is not deleted. There is at most one per
// (booster, status).
type BoostEdge struct {
	BoosterAccountID string
	StatusID         string
	BoostID          string
}

// EdgeOf returns the edge of boost b.
func EdgeOf(b Boost) BoostEdge {
	return BoostEdge{BoosterAccountID: b.BoosterAccountID, StatusID: b.StatusID, BoostID: b.ID}
}

// Favourite is an account's favourite (Like) of a status, a current value (data-model §4.4).
type Favourite struct {
	ID        string
	AccountID string
	StatusID  string
	URI       string // the IRI of the Like
	CreatedAt time.Time
}

// Bookmark is an account's private bookmark of a status (FR-113), a current value.
type Bookmark struct {
	ID        string
	AccountID string
	StatusID  string
	CreatedAt time.Time
}

// Pin is a status that its author pinned to the profile (the Featured collection), a current
// value.
type Pin struct {
	ID        string
	AccountID string
	StatusID  string
	CreatedAt time.Time
}
