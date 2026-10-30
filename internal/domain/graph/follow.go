// SPDX-License-Identifier: AGPL-3.0-only

package graph

import (
	"errors"
	"time"
)

// Follow is one follow relationship from its request on (data-model §4.5, follows). It is
// immutable: acceptance, rejection, and termination are separate events, and following again
// after a rejection or termination creates a new Follow, so earlier relationships stay on record.
type Follow struct {
	ID                string
	URI               string // the IRI of the Follow activity (ADR-0018)
	FollowerAccountID string
	FolloweeAccountID string
	RequestedAt       time.Time
}

// FollowURI returns the IRI of the Follow activity of a local follow on host (ADR-0018: every
// activity other than Create lives at /ap/activities/{uuid}).
func FollowURI(host, followID string) string {
	return "https://" + host + "/ap/activities/" + followID
}

// ErrSelfFollow reports a follow whose follower and followee are the same account.
var ErrSelfFollow = errors.New("graph: an account cannot follow itself")

// Validate checks the invariants of a new follow.
func (f Follow) Validate() error {
	if f.FollowerAccountID == "" || f.FolloweeAccountID == "" {
		return errors.New("graph: a follow needs a follower and a followee")
	}
	if f.FollowerAccountID == f.FolloweeAccountID {
		return ErrSelfFollow
	}
	return nil
}

// Resend is the event that a remote follower sent a Follow again, under another URI, while its
// follow is active (data-model §4.5, follow_resends): its server does not count the follow as
// established. The follow keeps its row, and its Accept is sent again for URI.
type Resend struct {
	URI        string // the IRI of the Follow received again
	FollowID   string
	ReceivedAt time.Time
}

// Acceptor says who accepted a follow.
type Acceptor string

// Acceptors (follow_acceptances.accepted_by).
const (
	// AcceptedByAuto is an automatic acceptance: the followee is not locked.
	AcceptedByAuto Acceptor = "auto"
	// AcceptedByFollowee is a local followee approving a follow request (FR-302).
	AcceptedByFollowee Acceptor = "followee"
	// AcceptedByRemote is an Accept activity received from a remote followee.
	AcceptedByRemote Acceptor = "remote"
)

// Acceptance is the event that a follow became active.
type Acceptance struct {
	FollowID    string
	AcceptedBy  Acceptor
	ActivityURI string // the received Accept activity; "" for local acceptances
	AcceptedAt  time.Time
}

// Rejector says who rejected a follow.
type Rejector string

// Rejectors (follow_rejections.rejected_by).
const (
	RejectedByFollowee Rejector = "followee"
	RejectedByRemote   Rejector = "remote"
	RejectedBySystem   Rejector = "system"
)

// Rejection is the event that a pending follow request was declined.
type Rejection struct {
	FollowID    string
	RejectedBy  Rejector
	ActivityURI string // the received Reject activity; "" for local rejections
	RejectedAt  time.Time
}

// TerminationReason says why a follow ended.
type TerminationReason string

// Termination reasons (follow_terminations.reason).
const (
	// TerminatedUnfollowed is the follower ending the follow, including withdrawing a pending
	// request (Undo Follow).
	TerminatedUnfollowed TerminationReason = "unfollowed"
	// TerminatedRemovedByFollowee is the followee removing the follower.
	TerminatedRemovedByFollowee TerminationReason = "removed_by_followee"
	TerminatedBlocked           TerminationReason = "blocked"
	TerminatedAccountDeleted    TerminationReason = "account_deleted"
	TerminatedModeration        TerminationReason = "moderation"
	// TerminatedMoved is the follow of an account that moved, which the follower follows the
	// target of instead (ADR-0089).
	TerminatedMoved TerminationReason = "moved"
)

// Termination is the event that a pending or active follow ended.
type Termination struct {
	FollowID     string
	Reason       TerminationReason
	TerminatedAt time.Time
}

// State is the state of the follow from one account to another.
type State string

// Follow states. Edges store only StatePending and StateActive; StateNone is the absence of an edge.
const (
	StateNone    State = "none"
	StatePending State = "pending"
	StateActive  State = "active"
)

// Edge is the live follow between two accounts (the follow_edges projection): a follow that is
// neither rejected nor terminated. There is at most one edge per (follower, followee).
type Edge struct {
	FollowerAccountID string
	FolloweeAccountID string
	FollowID          string
	State             State // StatePending or StateActive
	// Since is when the edge reached its state: the request for StatePending and the
	// acceptance for StateActive.
	Since time.Time
}

// PendingEdge returns the edge of a follow awaiting approval.
func PendingEdge(f Follow) Edge {
	return Edge{
		FollowerAccountID: f.FollowerAccountID, FolloweeAccountID: f.FolloweeAccountID,
		FollowID: f.ID, State: StatePending, Since: f.RequestedAt,
	}
}

// ActiveEdge returns the edge of a follow after acceptance a.
func ActiveEdge(f Follow, a Acceptance) Edge {
	return Edge{
		FollowerAccountID: f.FollowerAccountID, FolloweeAccountID: f.FolloweeAccountID,
		FollowID: f.ID, State: StateActive, Since: a.AcceptedAt,
	}
}

// History is a follow together with the events recorded for it: the source of its edge.
type History struct {
	Follow      Follow
	Acceptance  *Acceptance
	Rejection   *Rejection
	Termination *Termination
}

// Edge returns the edge the follow projects to, and false when it projects to none because it
// was rejected or terminated. Updating follow_edges event by event and rebuilding it from
// histories with this function agree.
func (h History) Edge() (Edge, bool) {
	switch {
	case h.Rejection != nil || h.Termination != nil:
		return Edge{}, false
	case h.Acceptance != nil:
		return ActiveEdge(h.Follow, *h.Acceptance), true
	default:
		return PendingEdge(h.Follow), true
	}
}

// Preferences are the follower's settings of one follow (follow_preferences, a current value).
type Preferences struct {
	FollowID string
	// ShowBoosts shows the followee's boosts in the follower's home timeline.
	ShowBoosts bool
}

// DefaultPreferences are the preferences of a new follow: boosts shown. The notifications of the
// followee's new posts are a subscription of the notification module, not a preference of the
// follow.
func DefaultPreferences(followID string) Preferences {
	return Preferences{FollowID: followID, ShowBoosts: true}
}

// Counters are the numbers of active followers and followees of an account (follow_counters).
// They count the follows this instance knows about.
type Counters struct {
	AccountID      string
	FollowersCount int64
	FollowingCount int64
}

// CountEdges derives the counters of every account from the edges: only active edges count.
// Accounts without active edges have no counters, the same as no row.
func CountEdges(edges []Edge) map[string]Counters {
	out := map[string]Counters{}
	for _, e := range edges {
		if e.State != StateActive {
			continue
		}
		er := out[e.FollowerAccountID]
		er.AccountID = e.FollowerAccountID
		er.FollowingCount++
		out[e.FollowerAccountID] = er
		ee := out[e.FolloweeAccountID]
		ee.AccountID = e.FolloweeAccountID
		ee.FollowersCount++
		out[e.FolloweeAccountID] = ee
	}
	return out
}
