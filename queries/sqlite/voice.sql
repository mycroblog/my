-- SPDX-License-Identifier: AGPL-3.0-only

-- The voice chat rooms (ADR-0100 section 12).

-- name: InsertVoiceRoom :exec
INSERT INTO voice_rooms (id, opened_by_account_id, opened_at, transport) VALUES (?, ?, ?, ?);

-- name: InsertVoiceRoomRevision :exec
INSERT INTO voice_room_revisions (id, room_id, name, description, visibility, speakers, icon_emoji_key, icon_custom_emoji_id, revised_by_account_id, revised_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertVoiceRoomRevisionHashtag :exec
INSERT INTO voice_room_revision_hashtags (revision_id, position, name, name_normalized) VALUES (?, ?, ?, ?);

-- name: InsertVoiceRoomHostChange :exec
INSERT INTO voice_room_host_changes (id, room_id, host_account_id, reason, changed_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertVoiceRoomCohostChange :exec
INSERT INTO voice_room_cohost_changes (id, room_id, account_id, change, changed_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertVoiceRoomParticipation :exec
INSERT INTO voice_room_participations (id, room_id, account_id, joined_at) VALUES (?, ?, ?, ?);

-- name: InsertVoiceRoomParticipationEnd :execrows
INSERT INTO voice_room_participation_ends (participation_id, reason, ended_at) VALUES (?, ?, ?)
ON CONFLICT (participation_id) DO NOTHING;

-- name: InsertVoiceRoomRoleChange :exec
INSERT INTO voice_room_role_changes (id, participation_id, role, changed_by_account_id, changed_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertVoiceRoomSpeakGrant :exec
INSERT INTO voice_room_speak_grants (id, room_id, account_id, change, changed_by_account_id, changed_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertVoiceRoomHandRaise :exec
INSERT INTO voice_room_hand_raises (id, participation_id, change, changed_at) VALUES (?, ?, ?, ?);

-- name: InsertVoiceRoomInvitation :execrows
INSERT INTO voice_room_invitations (id, room_id, invitee_account_id, role, invited_by_account_id, invited_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (room_id, invitee_account_id) DO NOTHING;

-- name: InsertVoiceRoomInvitationRevocation :execrows
INSERT INTO voice_room_invitation_revocations (invitation_id, revoked_by_account_id, revoked_at) VALUES (?, ?, ?)
ON CONFLICT (invitation_id) DO NOTHING;

-- name: InsertVoiceRoomBan :execrows
INSERT INTO voice_room_bans (id, room_id, account_id, banned_by_account_id, banned_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (room_id, account_id) DO NOTHING;

-- name: InsertVoiceRoomEnding :execrows
INSERT INTO voice_room_endings (room_id, reason, ended_by_account_id, ended_at, note) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (room_id) DO NOTHING;

-- name: UpsertVoiceRoomHostGrace :exec
INSERT INTO voice_room_host_graces (room_id, ends_at) VALUES (?, ?)
ON CONFLICT (room_id) DO UPDATE SET ends_at = excluded.ends_at;

-- name: DeleteVoiceRoomHostGrace :execrows
DELETE FROM voice_room_host_graces WHERE room_id = ?;

-- name: GetVoiceRoomHostGrace :one
SELECT room_id, ends_at FROM voice_room_host_graces WHERE room_id = ?;

-- name: GetVoiceRoomParticipationEnd :one
SELECT participation_id, reason, ended_at FROM voice_room_participation_ends WHERE participation_id = ?;

-- GetVoiceRoom returns a room with its head, its current revision, and its ending (NULL
-- columns for an open room).
-- name: GetVoiceRoom :one
SELECT r.id, r.opened_by_account_id, r.opened_at, r.transport,
  h.revision_id, h.host_account_id, h.state, h.participants_count,
  v.name, v.description, v.visibility, v.speakers, v.icon_emoji_key, v.icon_custom_emoji_id, v.revised_by_account_id, v.revised_at,
  e.reason AS end_reason, e.ended_by_account_id, e.ended_at, e.note AS end_note
FROM voice_rooms r
JOIN voice_room_heads h ON h.room_id = r.id
JOIN voice_room_revisions v ON v.id = h.revision_id
LEFT JOIN voice_room_endings e ON e.room_id = r.id
WHERE r.id = ?;

-- ListOpenVoiceRooms returns the open rooms, of those whose current revision has the hashtag
-- tag unless it is '', by the most participants first; the columns are those of GetVoiceRoom.
-- name: ListOpenVoiceRooms :many
SELECT r.id, r.opened_by_account_id, r.opened_at, r.transport,
  h.revision_id, h.host_account_id, h.state, h.participants_count,
  v.name, v.description, v.visibility, v.speakers, v.icon_emoji_key, v.icon_custom_emoji_id, v.revised_by_account_id, v.revised_at,
  e.reason AS end_reason, e.ended_by_account_id, e.ended_at, e.note AS end_note
FROM voice_room_heads h
JOIN voice_rooms r ON r.id = h.room_id
JOIN voice_room_revisions v ON v.id = h.revision_id
LEFT JOIN voice_room_endings e ON e.room_id = r.id
WHERE h.state = 'open'
  AND (CAST(sqlc.arg(tag) AS TEXT) = '' OR EXISTS (
    SELECT 1 FROM voice_room_revision_hashtags t WHERE t.revision_id = h.revision_id AND t.name_normalized = CAST(sqlc.arg(tag) AS TEXT)
  ))
ORDER BY h.participants_count DESC, h.room_id;

-- ListHostedVoiceRooms returns at most row_limit rooms that account_id hosted at some time
-- (ADR-0100 section 13), newest first, before the room of an ID (or from the newest with ''); the
-- columns are those of GetVoiceRoom.
-- name: ListHostedVoiceRooms :many
SELECT r.id, r.opened_by_account_id, r.opened_at, r.transport,
  h.revision_id, h.host_account_id, h.state, h.participants_count,
  v.name, v.description, v.visibility, v.speakers, v.icon_emoji_key, v.icon_custom_emoji_id, v.revised_by_account_id, v.revised_at,
  e.reason AS end_reason, e.ended_by_account_id, e.ended_at, e.note AS end_note
FROM voice_rooms r
JOIN voice_room_heads h ON h.room_id = r.id
JOIN voice_room_revisions v ON v.id = h.revision_id
LEFT JOIN voice_room_endings e ON e.room_id = r.id
WHERE r.id IN (SELECT c.room_id FROM voice_room_host_changes c WHERE c.host_account_id = sqlc.arg(account_id))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR r.id < CAST(sqlc.arg(before) AS TEXT))
ORDER BY r.id DESC
LIMIT sqlc.arg(row_limit);

-- ListVoiceRoomRevisionHashtags returns the hashtags of revision_ids, a JSON array.
-- name: ListVoiceRoomRevisionHashtags :many
SELECT revision_id, position, name, name_normalized FROM voice_room_revision_hashtags
WHERE revision_id IN (SELECT value FROM json_each(CAST(sqlc.arg(revision_ids) AS TEXT)))
ORDER BY revision_id, position;

-- name: ListVoiceRoomParticipants :many
SELECT account_id, room_id, participation_id, role, cohost_change_id, hand_raised FROM voice_room_participants
WHERE room_id = ?
ORDER BY participation_id;

-- name: GetVoiceRoomParticipant :one
SELECT account_id, room_id, participation_id, role, cohost_change_id, hand_raised FROM voice_room_participants
WHERE account_id = ?;

-- GetVoiceRoomInvitation returns the invitation of an account to a room and whether it was revoked.
-- name: GetVoiceRoomInvitation :one
SELECT i.id, i.room_id, i.invitee_account_id, i.role, i.invited_by_account_id, i.invited_at,
  CAST(EXISTS (SELECT 1 FROM voice_room_invitation_revocations x WHERE x.invitation_id = i.id) AS INTEGER) AS revoked
FROM voice_room_invitations i
WHERE i.room_id = ? AND i.invitee_account_id = ?;

-- ListVoiceRoomInvitations returns the invitations of a room that were not revoked, after the
-- invitation of an ID (or from the first with ''), in the order they were made.
-- name: ListVoiceRoomInvitations :many
SELECT i.id, i.room_id, i.invitee_account_id, i.role, i.invited_by_account_id, i.invited_at
FROM voice_room_invitations i
WHERE i.room_id = ? AND i.id > ?
  AND NOT EXISTS (SELECT 1 FROM voice_room_invitation_revocations x WHERE x.invitation_id = i.id)
ORDER BY i.id
LIMIT ?;

-- name: IsVoiceRoomBanned :one
SELECT CAST(EXISTS (SELECT 1 FROM voice_room_bans WHERE room_id = ? AND account_id = ?) AS INTEGER) AS banned;

-- IsVoiceRoomSpeakGranted reports whether the newest speak grant of an account in a room is granted.
-- name: IsVoiceRoomSpeakGranted :one
SELECT CAST(COALESCE((
  SELECT g.change = 'granted' FROM voice_room_speak_grants g
  WHERE g.room_id = ? AND g.account_id = ?
  ORDER BY g.id DESC LIMIT 1
), 0) AS INTEGER) AS granted;

-- Projections ---------------------------------------------------------------------------------
-- The rules of voice_room_heads and voice_room_participants. The refreshes of one room or one
-- account and the rebuild state the same rules.

-- name: DeleteVoiceRoomHead :exec
DELETE FROM voice_room_heads WHERE room_id = ?;

-- InsertVoiceRoomHead derives the head of a room: its newest revision and host change, ended
-- when it has an ending, and the participations that have not ended of an open room.
-- name: InsertVoiceRoomHead :exec
INSERT INTO voice_room_heads (room_id, revision_id, host_account_id, state, participants_count)
SELECT r.id,
  (SELECT v.id FROM voice_room_revisions v WHERE v.room_id = r.id ORDER BY v.id DESC LIMIT 1),
  (SELECT c.host_account_id FROM voice_room_host_changes c WHERE c.room_id = r.id ORDER BY c.id DESC LIMIT 1),
  CASE WHEN EXISTS (SELECT 1 FROM voice_room_endings e WHERE e.room_id = r.id) THEN 'ended' ELSE 'open' END,
  CASE WHEN EXISTS (SELECT 1 FROM voice_room_endings e WHERE e.room_id = r.id) THEN 0 ELSE (
    SELECT COUNT(*) FROM voice_room_participations p
    WHERE p.room_id = r.id AND NOT EXISTS (SELECT 1 FROM voice_room_participation_ends x WHERE x.participation_id = p.id)
  ) END
FROM voice_rooms r
WHERE r.id = ?;

-- name: DeleteAllVoiceRoomHeads :exec
DELETE FROM voice_room_heads;

-- name: RebuildVoiceRoomHeads :exec
INSERT INTO voice_room_heads (room_id, revision_id, host_account_id, state, participants_count)
SELECT r.id,
  (SELECT v.id FROM voice_room_revisions v WHERE v.room_id = r.id ORDER BY v.id DESC LIMIT 1),
  (SELECT c.host_account_id FROM voice_room_host_changes c WHERE c.room_id = r.id ORDER BY c.id DESC LIMIT 1),
  CASE WHEN EXISTS (SELECT 1 FROM voice_room_endings e WHERE e.room_id = r.id) THEN 'ended' ELSE 'open' END,
  CASE WHEN EXISTS (SELECT 1 FROM voice_room_endings e WHERE e.room_id = r.id) THEN 0 ELSE (
    SELECT COUNT(*) FROM voice_room_participations p
    WHERE p.room_id = r.id AND NOT EXISTS (SELECT 1 FROM voice_room_participation_ends x WHERE x.participation_id = p.id)
  ) END
FROM voice_rooms r;

-- name: DeleteVoiceRoomParticipant :exec
DELETE FROM voice_room_participants WHERE room_id = ? AND account_id = ?;

-- InsertVoiceRoomParticipant derives the participant row of an account in a room from its
-- participation that has not ended, in a room that has not ended: the newest role change
-- (listener without one), the newest co-host change of the account in the room since the
-- participation started when it is an assignment and the account is not the current host, and
-- the newest hand raise.
-- name: InsertVoiceRoomParticipant :exec
INSERT INTO voice_room_participants (account_id, room_id, participation_id, role, cohost_change_id, hand_raised)
SELECT p.account_id, p.room_id, p.id,
  COALESCE((SELECT c.role FROM voice_room_role_changes c WHERE c.participation_id = p.id ORDER BY c.id DESC LIMIT 1), 'listener'),
  (SELECT n.id FROM (
    SELECT c.id, c.change FROM voice_room_cohost_changes c
    WHERE c.room_id = p.room_id AND c.account_id = p.account_id AND c.id > p.id ORDER BY c.id DESC LIMIT 1
  ) n WHERE n.change = 'assigned' AND p.account_id <> (
    SELECT o.host_account_id FROM voice_room_host_changes o WHERE o.room_id = p.room_id ORDER BY o.id DESC LIMIT 1
  )),
  COALESCE((SELECT h.change = 'raised' FROM voice_room_hand_raises h WHERE h.participation_id = p.id ORDER BY h.id DESC LIMIT 1), 0)
FROM voice_room_participations p
WHERE p.room_id = ? AND p.account_id = ?
  AND NOT EXISTS (SELECT 1 FROM voice_room_participation_ends e WHERE e.participation_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM voice_room_endings x WHERE x.room_id = p.room_id);

-- name: DeleteVoiceRoomParticipants :exec
DELETE FROM voice_room_participants WHERE room_id = ?;

-- name: InsertVoiceRoomParticipants :exec
INSERT INTO voice_room_participants (account_id, room_id, participation_id, role, cohost_change_id, hand_raised)
SELECT p.account_id, p.room_id, p.id,
  COALESCE((SELECT c.role FROM voice_room_role_changes c WHERE c.participation_id = p.id ORDER BY c.id DESC LIMIT 1), 'listener'),
  (SELECT n.id FROM (
    SELECT c.id, c.change FROM voice_room_cohost_changes c
    WHERE c.room_id = p.room_id AND c.account_id = p.account_id AND c.id > p.id ORDER BY c.id DESC LIMIT 1
  ) n WHERE n.change = 'assigned' AND p.account_id <> (
    SELECT o.host_account_id FROM voice_room_host_changes o WHERE o.room_id = p.room_id ORDER BY o.id DESC LIMIT 1
  )),
  COALESCE((SELECT h.change = 'raised' FROM voice_room_hand_raises h WHERE h.participation_id = p.id ORDER BY h.id DESC LIMIT 1), 0)
FROM voice_room_participations p
WHERE p.room_id = ?
  AND NOT EXISTS (SELECT 1 FROM voice_room_participation_ends e WHERE e.participation_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM voice_room_endings x WHERE x.room_id = p.room_id);

-- name: DeleteAllVoiceRoomParticipants :exec
DELETE FROM voice_room_participants;

-- name: RebuildVoiceRoomParticipants :exec
INSERT INTO voice_room_participants (account_id, room_id, participation_id, role, cohost_change_id, hand_raised)
SELECT p.account_id, p.room_id, p.id,
  COALESCE((SELECT c.role FROM voice_room_role_changes c WHERE c.participation_id = p.id ORDER BY c.id DESC LIMIT 1), 'listener'),
  (SELECT n.id FROM (
    SELECT c.id, c.change FROM voice_room_cohost_changes c
    WHERE c.room_id = p.room_id AND c.account_id = p.account_id AND c.id > p.id ORDER BY c.id DESC LIMIT 1
  ) n WHERE n.change = 'assigned' AND p.account_id <> (
    SELECT o.host_account_id FROM voice_room_host_changes o WHERE o.room_id = p.room_id ORDER BY o.id DESC LIMIT 1
  )),
  COALESCE((SELECT h.change = 'raised' FROM voice_room_hand_raises h WHERE h.participation_id = p.id ORDER BY h.id DESC LIMIT 1), 0)
FROM voice_room_participations p
WHERE NOT EXISTS (SELECT 1 FROM voice_room_participation_ends e WHERE e.participation_id = p.id)
  AND NOT EXISTS (SELECT 1 FROM voice_room_endings x WHERE x.room_id = p.room_id);
