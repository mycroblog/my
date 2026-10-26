// SPDX-License-Identifier: AGPL-3.0-only

package voice

// Room is what the policy functions read of a room: whether it ended, its current host, and
// its current settings.
type Room struct {
	Ended         bool
	HostAccountID string
	Visibility    Visibility
	Speakers      Speakers
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

// Facts are the relations between a viewer and a room that internal/app/voice gathers for the
// policy functions (ADR-0100 §4). Follows count only when accepted (follow_edges).
type Facts struct {
	// Blocked reports a block, either way, between the viewer and the host or any co-host.
	Blocked bool
	// Banned reports that the host or a co-host removed the viewer from the room, which bans
	// the viewer from it.
	Banned bool
	// Cohost reports that the viewer is a co-host of the room.
	Cohost bool
	// Participant reports that the viewer is in the room now.
	Participant bool
	// Invited reports an invitation of the viewer to the room that was not revoked, whether the
	// viewer answered it or not.
	Invited bool
	// FollowsHost reports that the viewer follows the current host.
	FollowsHost bool
	// FollowedByHost reports that the current host follows the viewer.
	FollowedByHost bool
	// Limited reports that a moderator limited the viewer's account.
	Limited bool
	// SpeakGranted reports an explicit permission to speak in the room that was not revoked: a
	// granted raised hand, a grant of the host or a co-host, or an invitation as a speaker.
	SpeakGranted bool
}

// mutual reports that the viewer and the current host follow each other.
func (f Facts) mutual() bool { return f.FollowsHost && f.FollowedByHost }

// CanSee reports whether viewer sees room: in the room list, the right rail, the hashtag page,
// the profile entry, the recipients of the notifications, joining, and the API (ADR-0100 §4).
// The rules apply in this order, and the first that decides wins:
//
//  1. an ended room is not seen (its page shows only the summary);
//  2. a viewer who is not logged in, remote, suspended, or deleted does not see it;
//  3. a viewer with a block, either way, with the host or a co-host does not see it;
//  4. a viewer banned from the room does not see it;
//  5. the host, a co-host, or a participant sees it;
//  6. a viewer invited and not revoked sees it;
//  7. by the visibility: local is seen, followers when the viewer follows the host, mutuals
//     when they follow each other, and invited is not.
func CanSee(room Room, viewer Viewer, f Facts) bool {
	switch {
	case room.Ended:
		return false
	case viewer.AccountID == "" || !viewer.Local || !viewer.Active:
		return false
	case f.Blocked:
		return false
	case f.Banned:
		return false
	case viewer.AccountID == room.HostAccountID || f.Cohost || f.Participant:
		return true
	case f.Invited:
		return true
	}
	switch room.Visibility {
	case VisibilityLocal:
		return true
	case VisibilityFollowers:
		return f.FollowsHost
	case VisibilityMutuals:
		return f.mutual()
	}
	return false
}

// CanSpeakFreely reports whether participant, a listener in room, may become a speaker without
// asking (ADR-0100 §4). The rules apply in this order, and the first that decides wins:
//
//  1. the host and a co-host may;
//  2. a limited account may not without an explicit permission;
//  3. an explicit permission (a granted raised hand, a grant, an invitation as a speaker) may;
//  4. by the speakers setting: everyone may, followers when the participant follows the host,
//     mutuals when they follow each other, and manual may not.
func CanSpeakFreely(room Room, participant Viewer, f Facts) bool {
	switch {
	case participant.AccountID != "" && participant.AccountID == room.HostAccountID, f.Cohost:
		return true
	case f.Limited && !f.SpeakGranted:
		return false
	case f.SpeakGranted:
		return true
	}
	switch room.Speakers {
	case SpeakersEveryone:
		return true
	case SpeakersFollowers:
		return f.FollowsHost
	case SpeakersMutuals:
		return f.mutual()
	}
	return false
}

// HostFacts are the facts of an account that decide whether it may host a room (ADR-0100 §2).
type HostFacts struct {
	// Local reports a local account.
	Local bool
	// Active reports an account that is neither suspended nor deleted.
	Active bool
	// Limited reports that a moderator limited the account.
	Limited bool
	// EveryoneStarts reports the instance setting voice_room_starters = everyone.
	EveryoneStarts bool
	// MayStart reports that the account's roles grant the permission start_voice_rooms.
	MayStart bool
}

// CanHost reports whether an account with the facts f may host a room: a local account that is
// neither suspended, deleted, nor limited, when every local user may start rooms or its roles
// grant start_voice_rooms (ADR-0100 §2). It is checked when a room opens, when the host hands it
// over, and when the host moves on its own (§6).
func CanHost(f HostFacts) bool {
	return f.Local && f.Active && !f.Limited && (f.EveryoneStarts || f.MayStart)
}

// Candidate is a co-host who might take over a room, with whether CanHost holds for them.
type Candidate struct {
	AccountID string
	Eligible  bool
}

// NextHost returns the co-host who takes over a room when its host disconnects, leaves the
// eligibility, or leaves without choosing: the eligible one among cohosts, given in the order of
// their nomination, who was nominated first. It reports false when none is eligible, and the
// room ends (ADR-0100 §2, §6).
func NextHost(cohosts []Candidate) (string, bool) {
	for _, c := range cohosts {
		if c.Eligible && c.AccountID != "" {
			return c.AccountID, true
		}
	}
	return "", false
}
