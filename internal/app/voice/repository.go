// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"errors"
	"time"

	"github.com/mycroblog/my/internal/app/uow"
	"github.com/mycroblog/my/internal/domain/voice"
)

// ErrNotFound reports a room, a participant, an invitation, or a host grace that does not exist.
var ErrNotFound = errors.New("voice: not found")

// ErrInAnotherRoom reports a participation of an account that already has a participation that
// has not ended, in another room or in the same one (two joins that raced): an account has one
// participation at most (ADR-0100 §5), so a command that moves it ends the other participation
// first, and a command must not read this error as "a different room".
var ErrInAnotherRoom = errors.New("voice: the account is in another room")

// Room is a room (voice_rooms).
type Room struct {
	ID                string
	OpenedByAccountID string
	OpenedAt          time.Time
	Transport         voice.Transport
}

// Revision is a revision of the settings of a room (voice_room_revisions and its hashtags).
type Revision struct {
	ID                 string
	RoomID             string
	Settings           voice.Settings
	RevisedByAccountID string
	RevisedAt          time.Time
}

// HostChange is a new host of a room (voice_room_host_changes).
type HostChange struct {
	ID            string
	RoomID        string
	HostAccountID string
	Reason        voice.HostChangeReason
	ChangedAt     time.Time
}

// CohostChange is a co-host assigned or removed (voice_room_cohost_changes).
type CohostChange struct {
	ID        string
	RoomID    string
	AccountID string
	Change    voice.CohostChange
	ChangedAt time.Time
}

// Participation is an account in a room (voice_room_participations).
type Participation struct {
	ID        string
	RoomID    string
	AccountID string
	JoinedAt  time.Time
}

// ParticipationEnd is the end of a participation (voice_room_participation_ends).
type ParticipationEnd struct {
	ParticipationID string
	Reason          voice.ParticipationEndReason
	EndedAt         time.Time
}

// RoleChange is a participant becoming a speaker or a listener (voice_room_role_changes).
type RoleChange struct {
	ID                 string
	ParticipationID    string
	Role               voice.Role
	ChangedByAccountID string
	ChangedAt          time.Time
}

// SpeakGrant is an explicit permission to speak granted or revoked (voice_room_speak_grants).
type SpeakGrant struct {
	ID                 string
	RoomID             string
	AccountID          string
	Change             voice.SpeakGrantChange
	ChangedByAccountID string
	ChangedAt          time.Time
}

// HandRaise is a change of a participant's raised hand (voice_room_hand_raises).
type HandRaise struct {
	ID              string
	ParticipationID string
	Change          voice.HandRaiseChange
	ChangedAt       time.Time
}

// Invitation is an invitation to a room (voice_room_invitations).
type Invitation struct {
	ID                 string
	RoomID             string
	InviteeAccountID   string
	Role               voice.Role
	InvitedByAccountID string
	InvitedAt          time.Time
}

// InvitationRevocation is the revocation of an invitation (voice_room_invitation_revocations).
type InvitationRevocation struct {
	InvitationID       string
	RevokedByAccountID string
	RevokedAt          time.Time
}

// InvitationRecord is an invitation with whether it was revoked.
type InvitationRecord struct {
	Invitation
	Revoked bool
}

// Ban is an account removed from a room and banned from it (voice_room_bans).
type Ban struct {
	ID                string
	RoomID            string
	AccountID         string
	BannedByAccountID string
	BannedAt          time.Time
}

// Ending is the end of a room (voice_room_endings).
type Ending struct {
	RoomID string
	Reason voice.EndReason
	// EndedByAccountID is "" for an automatic end.
	EndedByAccountID string
	EndedAt          time.Time
}

// Head is the current revision, host, and state of a room (voice_room_heads).
type Head struct {
	RoomID        string
	RevisionID    string
	HostAccountID string
	State         voice.State
	// ParticipantsCount is how many participations of an open room have not ended; 0 for an
	// ended room.
	ParticipantsCount int
}

// Participant is a participation that has not ended (voice_room_participants).
type Participant struct {
	AccountID       string
	RoomID          string
	ParticipationID string
	// Role is the newest role change of the participation, RoleListener without one.
	Role voice.Role
	// CohostChangeID is the newest co-host assignment of the account in the room since this
	// participation started, while it is not followed by a removal and the account is not the
	// current host; "" otherwise. A co-host who leaves and joins again is not a co-host until
	// the host nominates them again. The nominations are in the order of these IDs.
	CohostChangeID string
	HandRaised     bool
}

// Cohost reports whether the participant is a co-host.
func (p Participant) Cohost() bool { return p.CohostChangeID != "" }

// HostGrace is the grace of a disconnected host (voice_room_host_graces).
type HostGrace struct {
	RoomID string
	EndsAt time.Time
}

// RoomRecord is a room with its head, its current revision, and its ending.
type RoomRecord struct {
	Room
	Head     Head
	Revision Revision
	// Ending is nil for an open room.
	Ending *Ending
}

// Repository is the persistence port of the voice module (data-model §4.14). The writes take the
// transaction of the command; the reads run in tx, or outside any transaction when tx is nil.
type Repository interface {
	ProjectionRepository

	// InsertRoom records a room.
	InsertRoom(ctx context.Context, tx uow.Tx, r Room) error
	// InsertRevision records a revision of the settings with its hashtags in their order.
	InsertRevision(ctx context.Context, tx uow.Tx, r Revision) error
	// InsertHostChange records a new host.
	InsertHostChange(ctx context.Context, tx uow.Tx, c HostChange) error
	// InsertCohostChange records a co-host assigned or removed.
	InsertCohostChange(ctx context.Context, tx uow.Tx, c CohostChange) error
	// InsertParticipation records an account joining a room.
	InsertParticipation(ctx context.Context, tx uow.Tx, p Participation) error
	// InsertParticipationEnd ends a participation and reports whether it did: a participation
	// that ended already keeps its first end.
	InsertParticipationEnd(ctx context.Context, tx uow.Tx, e ParticipationEnd) (bool, error)
	// InsertRoleChange records a participant becoming a speaker or a listener.
	InsertRoleChange(ctx context.Context, tx uow.Tx, c RoleChange) error
	// InsertSpeakGrant records an explicit permission to speak granted or revoked.
	InsertSpeakGrant(ctx context.Context, tx uow.Tx, g SpeakGrant) error
	// InsertHandRaise records a change of a raised hand.
	InsertHandRaise(ctx context.Context, tx uow.Tx, h HandRaise) error
	// InsertInvitation records an invitation and reports whether it did: an account invited to
	// the room already, revoked or not, keeps its first invitation.
	InsertInvitation(ctx context.Context, tx uow.Tx, i Invitation) (bool, error)
	// InsertInvitationRevocation revokes an invitation and reports whether it did: a revoked
	// invitation keeps its first revocation.
	InsertInvitationRevocation(ctx context.Context, tx uow.Tx, r InvitationRevocation) (bool, error)
	// InsertBan bans an account from a room and reports whether it did: a banned account keeps
	// its first ban.
	InsertBan(ctx context.Context, tx uow.Tx, b Ban) (bool, error)
	// InsertEnding ends a room and reports whether it did: a room that ended already keeps its
	// first ending, so the evaluations that race end it once (ADR-0100 §6).
	InsertEnding(ctx context.Context, tx uow.Tx, e Ending) (bool, error)

	// LockRoom locks a room until tx ends, so that the commands and the evaluations of one room
	// serialize; the transaction reads the room after it. A database whose write transactions
	// run one at a time takes no lock.
	LockRoom(ctx context.Context, tx uow.Tx, roomID string) error
	// LockOpening locks the opening of rooms until tx ends, so that the count of the open rooms
	// against voice_max_open_rooms holds.
	LockOpening(ctx context.Context, tx uow.Tx) error

	// PutHostGrace starts or moves the grace of the host of a room.
	PutHostGrace(ctx context.Context, tx uow.Tx, g HostGrace) error
	// DeleteHostGrace deletes the grace of a room and reports whether there was one.
	DeleteHostGrace(ctx context.Context, tx uow.Tx, roomID string) (bool, error)
	// FindHostGrace returns the grace of a room; ErrNotFound without one.
	FindHostGrace(ctx context.Context, tx uow.Tx, roomID string) (HostGrace, error)

	// FindRoom returns a room with its head, its current revision, and its ending; ErrNotFound
	// for an unknown room.
	FindRoom(ctx context.Context, tx uow.Tx, id string) (RoomRecord, error)
	// OpenRooms returns the open rooms, of those whose current revision has the hashtag
	// tagNormalized unless it is "", by the most participants first and then by room ID.
	OpenRooms(ctx context.Context, tx uow.Tx, tagNormalized string) ([]RoomRecord, error)
	// Participants returns the participants of a room in the order they joined.
	Participants(ctx context.Context, tx uow.Tx, roomID string) ([]Participant, error)
	// FindParticipant returns the participation of an account that has not ended; ErrNotFound
	// when the account is in no room.
	FindParticipant(ctx context.Context, tx uow.Tx, accountID string) (Participant, error)
	// FindParticipationEnd returns the end of a participation; ErrNotFound for one that has
	// not ended.
	FindParticipationEnd(ctx context.Context, tx uow.Tx, participationID string) (ParticipationEnd, error)
	// FindInvitation returns the invitation of an account to a room; ErrNotFound without one.
	FindInvitation(ctx context.Context, tx uow.Tx, roomID, accountID string) (InvitationRecord, error)
	// Invitations returns at most limit invitations of a room that were not revoked, in the
	// order they were made, after the invitation afterID unless it is "".
	Invitations(ctx context.Context, tx uow.Tx, roomID, afterID string, limit int) ([]Invitation, error)
	// Banned reports whether an account is banned from a room.
	Banned(ctx context.Context, tx uow.Tx, roomID, accountID string) (bool, error)
	// SpeakGranted reports whether the newest speak grant of an account in a room is granted.
	SpeakGranted(ctx context.Context, tx uow.Tx, roomID, accountID string) (bool, error)
}

// ProjectionRepository derives the projections of the voice module (data-model §9) from the
// events: voice_room_heads from the rooms, their revisions, host changes, participations, and
// endings, and voice_room_participants from the participations that have not ended in rooms
// that have not ended, their role changes and raised hands, the co-host changes of their accounts
// in their rooms since they joined, and the current host.
type ProjectionRepository interface {
	// RefreshHead derives the head of a room again.
	RefreshHead(ctx context.Context, tx uow.Tx, roomID string) error
	// RefreshParticipant derives again the participant row of an account in a room: the row of
	// its participation that has not ended, or none when it has none or the room ended. It fails with ErrInAnotherRoom when the
	// account has a participant row of another room.
	RefreshParticipant(ctx context.Context, tx uow.Tx, roomID, accountID string) error
	// RefreshRoomParticipants derives again every participant row of a room.
	RefreshRoomParticipants(ctx context.Context, tx uow.Tx, roomID string) error
	// RebuildProjections empties voice_room_heads and voice_room_participants and derives
	// both again from every event.
	RebuildProjections(ctx context.Context, tx uow.Tx) error
}
