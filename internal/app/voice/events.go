// SPDX-License-Identifier: AGPL-3.0-only

package voice

import (
	"context"
	"fmt"

	"github.com/mycroblog/my/internal/app/uow"
)

// Domain events of the voice module. A command writes the rows of the event first and then
// emits it in the same transaction; the projectors read those rows.
const (
	// EventRoomOpened follows a room, its first revision, and its first host change.
	EventRoomOpened = "voice.room_opened"
	// EventRoomRevised follows a revision of the settings.
	EventRoomRevised = "voice.room_revised"
	// EventHostChanged follows a new host.
	EventHostChanged = "voice.host_changed"
	// EventCohostChanged follows a co-host assigned or removed.
	EventCohostChanged = "voice.cohost_changed"
	// EventParticipationStarted follows an account joining a room, with its first role change
	// when it joins as a speaker.
	EventParticipationStarted = "voice.participation_started"
	// EventParticipationEnded follows the end of a participation.
	EventParticipationEnded = "voice.participation_ended"
	// EventRoleChanged follows a participant becoming a speaker or a listener.
	EventRoleChanged = "voice.role_changed"
	// EventHandRaiseChanged follows a change of a raised hand.
	EventHandRaiseChanged = "voice.hand_raise_changed"
	// EventRoomEnded follows the end of a room, after the ends of its participations.
	EventRoomEnded = "voice.room_ended"
	// EventSpeakGrantChanged follows an explicit permission to speak granted or revoked.
	EventSpeakGrantChanged = "voice.speak_grant_changed"
	// EventInvited follows an invitation to a room.
	EventInvited = "voice.invited"
	// EventInvitationRevoked follows the revocation of an invitation.
	EventInvitationRevoked = "voice.invitation_revoked"
	// EventBanned follows an account removed from a room and banned from it, before the end of
	// its participation.
	EventBanned = "voice.banned"
)

// RoomOpened carries a room that opened.
type RoomOpened struct {
	Room     Room
	Revision Revision
	Host     HostChange
}

// EventType implements uow.Event.
func (RoomOpened) EventType() string { return EventRoomOpened }

// RoomRevised carries a revision of the settings.
type RoomRevised struct{ Revision Revision }

// EventType implements uow.Event.
func (RoomRevised) EventType() string { return EventRoomRevised }

// HostChanged carries a new host.
type HostChanged struct{ Change HostChange }

// EventType implements uow.Event.
func (HostChanged) EventType() string { return EventHostChanged }

// CohostChanged carries a co-host assigned or removed.
type CohostChanged struct{ Change CohostChange }

// EventType implements uow.Event.
func (CohostChanged) EventType() string { return EventCohostChanged }

// ParticipationStarted carries an account joining a room.
type ParticipationStarted struct{ Participation Participation }

// EventType implements uow.Event.
func (ParticipationStarted) EventType() string { return EventParticipationStarted }

// ParticipationEnded carries the end of a participation.
type ParticipationEnded struct {
	Participation Participation
	End           ParticipationEnd
}

// EventType implements uow.Event.
func (ParticipationEnded) EventType() string { return EventParticipationEnded }

// RoleChanged carries a participant becoming a speaker or a listener.
type RoleChanged struct {
	Participation Participation
	Change        RoleChange
}

// EventType implements uow.Event.
func (RoleChanged) EventType() string { return EventRoleChanged }

// HandRaiseChanged carries a change of a raised hand.
type HandRaiseChanged struct {
	Participation Participation
	Change        HandRaise
}

// EventType implements uow.Event.
func (HandRaiseChanged) EventType() string { return EventHandRaiseChanged }

// RoomEnded carries the end of a room.
type RoomEnded struct{ Ending Ending }

// EventType implements uow.Event.
func (RoomEnded) EventType() string { return EventRoomEnded }

// SpeakGrantChanged carries an explicit permission to speak granted or revoked.
type SpeakGrantChanged struct{ Grant SpeakGrant }

// EventType implements uow.Event.
func (SpeakGrantChanged) EventType() string { return EventSpeakGrantChanged }

// Invited carries an invitation to a room.
type Invited struct{ Invitation Invitation }

// EventType implements uow.Event.
func (Invited) EventType() string { return EventInvited }

// InvitationRevoked carries the revocation of an invitation.
type InvitationRevoked struct {
	Invitation Invitation
	Revocation InvitationRevocation
}

// EventType implements uow.Event.
func (InvitationRevoked) EventType() string { return EventInvitationRevoked }

// Banned carries an account banned from a room.
type Banned struct{ Ban Ban }

// EventType implements uow.Event.
func (Banned) EventType() string { return EventBanned }

// RegisterProjectors registers the projectors of the module's own projections: each event
// that changes them derives again the rows of voice_room_heads and voice_room_participants that it touches, with
// the same rules as RebuildProjections.
func RegisterProjectors(r *uow.Projectors, repo ProjectionRepository) {
	participant := func(ctx context.Context, tx uow.Tx, p Participation) error {
		return repo.RefreshParticipant(ctx, tx, p.RoomID, p.AccountID)
	}
	r.Register(EventRoomOpened, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(RoomOpened).Room.ID)
	})
	r.Register(EventRoomRevised, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return repo.RefreshHead(ctx, tx, e.(RoomRevised).Revision.RoomID)
	})
	r.Register(EventHostChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		// The new host stops being a co-host, so the participants of the room change too.
		roomID := e.(HostChanged).Change.RoomID
		if err := repo.RefreshRoomParticipants(ctx, tx, roomID); err != nil {
			return err
		}
		return repo.RefreshHead(ctx, tx, roomID)
	})
	r.Register(EventCohostChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		c := e.(CohostChanged).Change
		return repo.RefreshParticipant(ctx, tx, c.RoomID, c.AccountID)
	})
	r.Register(EventParticipationStarted, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		p := e.(ParticipationStarted).Participation
		if err := participant(ctx, tx, p); err != nil {
			return err
		}
		return repo.RefreshHead(ctx, tx, p.RoomID)
	})
	r.Register(EventParticipationEnded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		p := e.(ParticipationEnded).Participation
		if err := participant(ctx, tx, p); err != nil {
			return err
		}
		return repo.RefreshHead(ctx, tx, p.RoomID)
	})
	r.Register(EventRoleChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return participant(ctx, tx, e.(RoleChanged).Participation)
	})
	r.Register(EventHandRaiseChanged, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		return participant(ctx, tx, e.(HandRaiseChanged).Participation)
	})
	r.Register(EventRoomEnded, func(ctx context.Context, tx uow.Tx, e uow.Event) error {
		roomID := e.(RoomEnded).Ending.RoomID
		if err := repo.RefreshRoomParticipants(ctx, tx, roomID); err != nil {
			return err
		}
		return repo.RefreshHead(ctx, tx, roomID)
	})
}

// RebuildProjections derives voice_room_heads and voice_room_participants again from the
// events, in one transaction (data-model §9). voice_room_host_graces is a current value that no
// event records, and stays.
func RebuildProjections(ctx context.Context, u uow.UnitOfWork, repo ProjectionRepository) error {
	err := u.Tx(ctx, func(tx uow.Tx) error { return repo.RebuildProjections(ctx, tx) })
	if err != nil {
		return fmt.Errorf("rebuild the voice projections: %w", err)
	}
	return nil
}
