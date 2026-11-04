// SPDX-License-Identifier: AGPL-3.0-only

package graph

import "github.com/mycroblog/my/internal/domain/graph"

// Event types of the graph module, for uow.Projectors (data-model §4.5). Every follow command
// emits exactly one of them inside its transaction, after writing the event rows, so that the
// projectors of this module (follow_edges, follow_counters, block_edges) and of other modules
// (timeline's feed_follow_edges and feed_exclusions, notification's follow notifications) see
// the same facts. Each event carries everything its subscribers need; they never read the graph
// tables. Blocking emits one EventFollowTerminated per follow it ends, then EventBlocked. Mutes
// are a current value that this module reads at query time; their commands emit EventMuted and
// EventUnmuted for other modules' projections (timeline's feed_exclusions, notification's
// notification_exclusions). Domain mutes are a current value too; their commands emit
// EventDomainMuted and EventDomainUnmuted for timeline's feed_domain_mutes and notification's
// notification_domain_exclusions.
//
//   - EventFollowed: a follow became active on creation (the followee is not locked).
//   - EventFollowRequested: a follow awaits the followee's approval (the followee is locked).
//   - EventFollowAccepted: a pending follow became active.
//   - EventFollowRejected: a pending follow was declined.
//   - EventFollowTerminated: a pending or active follow ended.
//   - EventFollowResent: a remote follower sent a Follow again, under another URI, for a follow
//     that is already active.
//   - EventFollowPreferencesChanged: the follower changed the preferences of a live follow.
//   - EventBlocked: an account blocked another.
//   - EventUnblocked: a block was lifted.
//   - EventMuted: an account muted another, or replaced the settings and expiry of a mute.
//   - EventUnmuted: an account removed its mute of another, or the cleanup deleted an expired
//     mute (not emitted without a mute).
//   - EventDomainMuted: an account muted a domain (not emitted when it already did).
//   - EventDomainUnmuted: an account removed its mute of a domain (not emitted without one).
const (
	EventFollowed                 = "graph.followed"
	EventFollowRequested          = "graph.follow_requested"
	EventFollowAccepted           = "graph.follow_accepted"
	EventFollowRejected           = "graph.follow_rejected"
	EventFollowTerminated         = "graph.follow_terminated"
	EventFollowResent             = "graph.follow_resent"
	EventFollowPreferencesChanged = "graph.follow_preferences_changed"
	EventBlocked                  = "graph.blocked"
	EventUnblocked                = "graph.unblocked"
	EventMuted                    = "graph.muted"
	EventUnmuted                  = "graph.unmuted"
	EventDomainMuted              = "graph.domain_muted"
	EventDomainUnmuted            = "graph.domain_unmuted"
)

// Muted is emitted when a mute is stored. Mute.ExpiresAt is nil for a mute until unmuted.
type Muted struct {
	Mute graph.Mute
}

// EventType implements uow.Event.
func (Muted) EventType() string { return EventMuted }

// Unmuted is emitted when an account removes its mute of a target, or when the cleanup deletes
// an expired mute.
type Unmuted struct {
	AccountID       string
	TargetAccountID string
}

// EventType implements uow.Event.
func (Unmuted) EventType() string { return EventUnmuted }

// DomainMuted is emitted when an account mutes a domain.
type DomainMuted struct {
	Mute graph.DomainMute
}

// EventType implements uow.Event.
func (DomainMuted) EventType() string { return EventDomainMuted }

// DomainUnmuted is emitted when an account removes its mute of a domain.
type DomainUnmuted struct {
	AccountID string
	Domain    string
}

// EventType implements uow.Event.
func (DomainUnmuted) EventType() string { return EventDomainUnmuted }

// WasActive reports whether the follow was active when it ended, rather than pending.
func (e FollowTerminated) WasActive() bool { return e.PreviousState == graph.StateActive }

// Active reports whether the follow was active, rather than pending, when its preferences changed.
func (e FollowPreferencesChanged) Active() bool { return e.State == graph.StateActive }

// Followed is emitted when a follow is created and accepted at once.
type Followed struct {
	Follow      graph.Follow
	Acceptance  graph.Acceptance
	Preferences graph.Preferences
}

// EventType implements uow.Event.
func (Followed) EventType() string { return EventFollowed }

// FollowRequested is emitted when a follow is created pending the followee's approval.
type FollowRequested struct {
	Follow      graph.Follow
	Preferences graph.Preferences
}

// EventType implements uow.Event.
func (FollowRequested) EventType() string { return EventFollowRequested }

// FollowAccepted is emitted when a pending follow is accepted.
type FollowAccepted struct {
	Follow     graph.Follow
	Acceptance graph.Acceptance
	// Preferences are the follow's preferences at acceptance, for subscribers that start
	// projecting the follow now that it is active.
	Preferences graph.Preferences
}

// EventType implements uow.Event.
func (FollowAccepted) EventType() string { return EventFollowAccepted }

// FollowRejected is emitted when a pending follow is rejected.
type FollowRejected struct {
	Follow    graph.Follow
	Rejection graph.Rejection
}

// EventType implements uow.Event.
func (FollowRejected) EventType() string { return EventFollowRejected }

// FollowTerminated is emitted when a pending or active follow ends.
type FollowTerminated struct {
	Follow      graph.Follow
	Termination graph.Termination
	// PreviousState is the state the follow ended in: graph.StatePending or graph.StateActive.
	PreviousState graph.State
	// Unannounced reports that the other account's server is not told of the end: the
	// suspension of its domain, which delivers nothing to it (ADR-0058).
	Unannounced bool
}

// EventType implements uow.Event.
func (FollowTerminated) EventType() string { return EventFollowTerminated }

// FollowResent is emitted when a remote follower sends a Follow again for an active follow.
type FollowResent struct {
	Follow graph.Follow
	Resend graph.Resend
}

// EventType implements uow.Event.
func (FollowResent) EventType() string { return EventFollowResent }

// FollowPreferencesChanged is emitted when the preferences of a live follow change.
type FollowPreferencesChanged struct {
	Follow      graph.Follow
	State       graph.State // the follow's state when its preferences changed
	Preferences graph.Preferences
}

// EventType implements uow.Event.
func (FollowPreferencesChanged) EventType() string { return EventFollowPreferencesChanged }

// Blocked is emitted when an account blocks another, after the FollowTerminated events of the
// follows between them that the block ended.
type Blocked struct {
	Block graph.Block
}

// EventType implements uow.Event.
func (Blocked) EventType() string { return EventBlocked }

// Unblocked is emitted when a block is lifted.
type Unblocked struct {
	Block      graph.Block
	Retraction graph.BlockRetraction
}

// EventType implements uow.Event.
func (Unblocked) EventType() string { return EventUnblocked }
