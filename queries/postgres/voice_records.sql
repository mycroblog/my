-- SPDX-License-Identifier: AGPL-3.0-only

-- The records of the voice chat rooms that the moderators read from a report, that the purge
-- erases after their retention, and that the erasure of an account empties (ADR-0100 sections 11
-- and 13).

-- name: ListVoiceRoomRevisions :many
SELECT id, room_id, name, description, visibility, speakers, icon_emoji_key, icon_custom_emoji_id, revised_by_account_id, revised_at
FROM voice_room_revisions WHERE room_id = $1 ORDER BY id;

-- ListVoiceRoomParticipations returns the participations of a room with their ends (NULL
-- columns for one that has not ended), in the order they started.
-- name: ListVoiceRoomParticipations :many
SELECT p.id, p.room_id, p.account_id, p.joined_at, e.reason AS end_reason, e.ended_at
FROM voice_room_participations p
LEFT JOIN voice_room_participation_ends e ON e.participation_id = p.id
WHERE p.room_id = $1
ORDER BY p.id;

-- name: ListVoiceRoomBans :many
SELECT id, room_id, account_id, banned_by_account_id, banned_at FROM voice_room_bans
WHERE room_id = $1 ORDER BY id;

-- ListVoiceRoomsToPurge returns at most row_limit rooms, in ID order after the room after (from
-- the first with ''), that ended before ended_before and still have records of their
-- participants.
-- name: ListVoiceRoomsToPurge :many
SELECT e.room_id FROM voice_room_endings e
WHERE e.ended_at < sqlc.arg(ended_before)
  AND (sqlc.arg(after)::text = '' OR e.room_id::text > sqlc.arg(after)::text)
  AND (EXISTS (SELECT 1 FROM voice_room_participations p WHERE p.room_id = e.room_id)
    OR EXISTS (SELECT 1 FROM voice_room_speak_grants g WHERE g.room_id = e.room_id)
    OR EXISTS (SELECT 1 FROM voice_room_invitations i WHERE i.room_id = e.room_id)
    OR EXISTS (SELECT 1 FROM voice_room_bans b WHERE b.room_id = e.room_id))
ORDER BY e.room_id
LIMIT sqlc.arg(row_limit);

-- ListVoiceRoomRecordAccounts returns the accounts that the records of the participants of a
-- room name, as the participant or as the one who acted.
-- name: ListVoiceRoomRecordAccounts :many
SELECT p.account_id FROM voice_room_participations p WHERE p.room_id = sqlc.arg(room_id)
UNION SELECT c.changed_by_account_id FROM voice_room_role_changes c
  JOIN voice_room_participations p ON p.id = c.participation_id WHERE p.room_id = sqlc.arg(room_id)
UNION SELECT g.account_id FROM voice_room_speak_grants g WHERE g.room_id = sqlc.arg(room_id)
UNION SELECT g.changed_by_account_id FROM voice_room_speak_grants g WHERE g.room_id = sqlc.arg(room_id)
UNION SELECT i.invitee_account_id FROM voice_room_invitations i WHERE i.room_id = sqlc.arg(room_id)
UNION SELECT i.invited_by_account_id FROM voice_room_invitations i WHERE i.room_id = sqlc.arg(room_id)
UNION SELECT x.revoked_by_account_id FROM voice_room_invitation_revocations x
  JOIN voice_room_invitations i ON i.id = x.invitation_id WHERE i.room_id = sqlc.arg(room_id)
UNION SELECT b.account_id FROM voice_room_bans b WHERE b.room_id = sqlc.arg(room_id)
UNION SELECT b.banned_by_account_id FROM voice_room_bans b WHERE b.room_id = sqlc.arg(room_id);

-- The PurgeVoiceRoom* statements delete the records of the participants of the rooms (@ids).

-- name: PurgeVoiceRoomParticipationEnds :exec
DELETE FROM voice_room_participation_ends WHERE participation_id IN (
  SELECT p.id FROM voice_room_participations p WHERE p.room_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeVoiceRoomRoleChanges :exec
DELETE FROM voice_room_role_changes WHERE participation_id IN (
  SELECT p.id FROM voice_room_participations p WHERE p.room_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeVoiceRoomHandRaises :exec
DELETE FROM voice_room_hand_raises WHERE participation_id IN (
  SELECT p.id FROM voice_room_participations p WHERE p.room_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeVoiceRoomParticipations :exec
DELETE FROM voice_room_participations WHERE room_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeVoiceRoomSpeakGrants :exec
DELETE FROM voice_room_speak_grants WHERE room_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeVoiceRoomInvitationRevocations :exec
DELETE FROM voice_room_invitation_revocations WHERE invitation_id IN (
  SELECT i.id FROM voice_room_invitations i WHERE i.room_id = ANY(sqlc.arg(ids)::uuid[]));

-- name: PurgeVoiceRoomInvitations :exec
DELETE FROM voice_room_invitations WHERE room_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: PurgeVoiceRoomBans :exec
DELETE FROM voice_room_bans WHERE room_id = ANY(sqlc.arg(ids)::uuid[]);

-- ListVoiceRoomsRevisedBy returns at most row_limit rooms, in ID order, with revisions that an
-- account wrote and whose name or description is not empty yet.
-- name: ListVoiceRoomsRevisedBy :many
SELECT DISTINCT room_id FROM voice_room_revisions
WHERE revised_by_account_id = sqlc.arg(account_id) AND (name <> '' OR description <> '')
ORDER BY room_id
LIMIT sqlc.arg(row_limit);

-- EraseVoiceRoomRevisionTexts empties the names and descriptions of the revisions that an
-- account wrote in the rooms (@ids).
-- name: EraseVoiceRoomRevisionTexts :exec
UPDATE voice_room_revisions SET name = '', description = ''
WHERE revised_by_account_id = sqlc.arg(account_id) AND room_id = ANY(sqlc.arg(ids)::uuid[]);
