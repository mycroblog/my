// SPDX-License-Identifier: AGPL-3.0-only

// Package voice holds the use cases of the voice chat rooms (ADR-0100). Only this module writes
// the rooms, their settings, hosts, co-hosts, participations, roles, grants, raised hands,
// invitations, bans, and endings (schema/<dialect>/14-voice.sql, data-model §4.14).
//
// The records live behind Repository, whose database adapters (internal/adapter/db/*) pass
// testkit.VoiceRepositoryContract. A command writes the rows of its events and then emits the
// matching domain event of events.go in the same transaction; the projectors that
// RegisterProjectors registers derive voice_room_heads and voice_room_participants from those
// rows, and RebuildProjections derives both again from every event (data-model §9).
package voice
