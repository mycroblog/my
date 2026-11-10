// SPDX-License-Identifier: AGPL-3.0-only

package gathering

// The voice chat room of an event (ADR-0161 §10): the start of each date of an event with a voice
// place opens a room that its organizer hosts. When the room cannot open, the event stays and its
// organizer is told why; the server also checks at the confirmation of the date and one day
// before the start whether the room will open.

// RoomReason is why the voice chat room of an event cannot open, or could not.
type RoomReason string

// Room reasons.
const (
	// RoomDisabled: the voice chat is off.
	RoomDisabled RoomReason = "disabled"
	// RoomNotEligible: the organizer may not host a room.
	RoomNotEligible RoomReason = "not_eligible"
	// RoomTooManyRooms: as many rooms are open as the operator allows.
	RoomTooManyRooms RoomReason = "too_many_rooms"
	// RoomHostingElsewhere: the organizer hosts another open room.
	RoomHostingElsewhere RoomReason = "hosting_elsewhere"
	// RoomUnattended: the room opened, but nobody came before its wait ended.
	RoomUnattended RoomReason = "unattended"
)

// Valid reports whether r is a known reason.
func (r RoomReason) Valid() bool {
	switch r {
	case RoomDisabled, RoomNotEligible, RoomTooManyRooms, RoomHostingElsewhere, RoomUnattended:
		return true
	}
	return false
}

// AtStart reports whether r is a reason that kept the room shut at the start of the event, which
// gathering_voice_room_failures records; RoomUnattended is of a room that opened.
func (r RoomReason) AtStart() bool { return r.Valid() && r != RoomUnattended }

// RoomCheckpoint is when the server found out that the room of an event cannot open, or could not.
type RoomCheckpoint string

// Room checkpoints.
const (
	// CheckpointConfirmed: the date was confirmed or changed.
	CheckpointConfirmed RoomCheckpoint = "confirmed"
	// CheckpointDayBefore: RoomCheckBefore the start.
	CheckpointDayBefore RoomCheckpoint = "day_before"
	// CheckpointStart: the start, when the room was to open.
	CheckpointStart RoomCheckpoint = "start"
	// CheckpointEnded: the room ended unattended.
	CheckpointEnded RoomCheckpoint = "ended"
)
