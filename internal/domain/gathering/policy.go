// SPDX-License-Identifier: AGPL-3.0-only

package gathering

// Event is what the policy functions read of an event: its organizer and its current settings.
type Event struct {
	OrganizerAccountID string
	Visibility         Visibility
	PlaceDisclosure    PlaceDisclosure
}

// Viewer is the account a policy function decides for. The zero value is a viewer who is not
// logged in.
type Viewer struct {
	// AccountID is "" for a viewer who is not logged in.
	AccountID string
	// Local reports a local account.
	Local bool
	// Active reports an account that is neither suspended nor deleted.
	Active bool
}

// Facts are the relations between a viewer and an event that internal/app/gathering gathers for
// the policy functions (ADR-0161 §9). Follows count only when accepted (follow_edges).
type Facts struct {
	// Blocked reports a block, either way, between the viewer and the organizer or any
	// co-organizer.
	Blocked bool
	// Coorganizer reports that the viewer is a co-organizer of the event.
	Coorganizer bool
	// Invited reports an invitation of the viewer to the event that was not revoked, whether the
	// viewer answered it or not.
	Invited bool
	// Answered reports that the viewer answered the event (any answer, declined included).
	Answered bool
	// Voted reports that the viewer answered a candidate of the event's date poll (any vote).
	Voted bool
	// Approved reports that an organizer approved the viewer's answer of going or maybe and did
	// not revoke it, and that the viewer has not declined since.
	Approved bool
	// FollowsOrganizer reports that the viewer follows the organizer.
	FollowsOrganizer bool
	// FollowedByOrganizer reports that the organizer follows the viewer.
	FollowedByOrganizer bool
}

// CanSee reports whether viewer sees event: in the list, on its page, as a recipient of its
// notifications, to answer it, in its calendar file, and in the API (ADR-0161 §9). The rules
// apply in this order, and the first that decides wins:
//
//  1. a viewer who is not logged in, remote, suspended, or deleted does not see it;
//  2. a viewer with a block, either way, with the organizer or a co-organizer does not see it;
//  3. the organizer and a co-organizer see it;
//  4. a viewer invited and not revoked sees it, whatever the answer;
//  5. a viewer who answered it or its date poll sees it, so that narrowing the visibility shuts
//     nobody out;
//  6. by the visibility: public is seen, followers when the viewer follows the organizer,
//     following when the organizer follows the viewer, mutuals when they follow each other,
//     and invited is not.
//
// A cancelled or ended event is seen by the same viewers.
func CanSee(e Event, viewer Viewer, f Facts) bool {
	switch {
	case viewer.AccountID == "" || !viewer.Local || !viewer.Active:
		return false
	case f.Blocked:
		return false
	case Organizes(e, viewer, f):
		return true
	case f.Invited, f.Answered, f.Voted:
		return true
	}
	switch e.Visibility {
	case VisibilityPublic:
		return true
	case VisibilityFollowers:
		return f.FollowsOrganizer
	case VisibilityFollowing:
		return f.FollowedByOrganizer
	case VisibilityMutuals:
		return f.FollowsOrganizer && f.FollowedByOrganizer
	}
	return false
}

// Organizes reports whether viewer is the organizer or a co-organizer of the event.
func Organizes(e Event, viewer Viewer, f Facts) bool {
	return (viewer.AccountID != "" && viewer.AccountID == e.OrganizerAccountID) || f.Coorganizer
}

// CanSeePlace reports whether viewer sees a place of kind k of the event (ADR-0161 §9): CanSee
// holds, and the viewer organizes it, or the place is a voice chat room (which its own visibility
// guards), or the event shows its places to every viewer, or an organizer approved the viewer.
func CanSeePlace(e Event, k PlaceKind, viewer Viewer, f Facts) bool {
	if !CanSee(e, viewer, f) {
		return false
	}
	return Organizes(e, viewer, f) || k == PlaceVoice || e.PlaceDisclosure == DisclosureViewers || f.Approved
}

// OrganizerFacts are the facts of an account that decide whether it may organize an event
// (ADR-0161 §3).
type OrganizerFacts struct {
	// Local reports a local account.
	Local bool
	// Active reports an account that is neither suspended nor deleted.
	Active bool
	// Limited reports that a moderator limited the account.
	Limited bool
	// EveryoneCreates reports the instance setting event_creators = everyone.
	EveryoneCreates bool
	// MayCreate reports that the account's roles grant the permission create_events.
	MayCreate bool
}

// CanOrganize reports whether an account with the facts f may organize an event: a local account
// that is neither suspended, deleted, nor limited, when every local user may create events or its
// roles grant create_events (ADR-0161 §3). It is checked when an event is created and when its
// organizer revises it; a co-organizer needs none of it.
func CanOrganize(f OrganizerFacts) bool {
	return f.Local && f.Active && !f.Limited && (f.EveryoneCreates || f.MayCreate)
}
