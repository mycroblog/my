// SPDX-License-Identifier: AGPL-3.0-only

// Package voice holds the use cases of the voice chat rooms (ADR-0100). Only this module writes
// the rooms, their settings, hosts, co-hosts, participations, roles, grants, raised hands,
// invitations, bans, and endings (schema/14-voice).
//
// The records live behind Repository, whose database adapters (internal/adapter/db/*) pass
// voicetest.VoiceRepositoryContract. A command writes the rows of its events and then emits the
// matching domain event of events.go in the same transaction; the projectors that
// RegisterProjectors registers derive voice_room_heads and voice_room_participants from those
// rows, by the same rules as the rebuild, and RebuildProjections derives both again from every
// event. The changes that touch neither projection (speak grants, invitations and their
// revocations, bans) emit their events too, for the other modules only.
//
// A room's head is its newest revision and host change, whether it ended, and the number of open
// participations. An account has one open participation at most, since it keys its participant
// row: the projector refuses a second one with ErrInAnotherRoom and the transaction writes
// nothing, so a move to another room ends the earlier participation first.
package voice
