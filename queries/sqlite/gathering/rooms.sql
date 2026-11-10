-- The voice chat rooms of the events (ADR-0161, section 10).

-- name: InsertGatheringVoiceRoom :execrows
INSERT INTO gathering_voice_rooms (gathering_id, confirmation_id, room_id, opened_at) VALUES (?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- name: InsertGatheringVoiceRoomFailure :execrows
INSERT INTO gathering_voice_room_failures (gathering_id, confirmation_id, reason, failed_at) VALUES (?, ?, ?, ?)
ON CONFLICT DO NOTHING;

-- ListGatheringVoiceRooms returns the rooms of the dates of gathering_ids, a JSON array.
-- name: ListGatheringVoiceRooms :many
SELECT gathering_id, confirmation_id, room_id, opened_at FROM gathering_voice_rooms
WHERE gathering_id IN (SELECT value FROM json_each(CAST(sqlc.arg(gathering_ids) AS TEXT)))
ORDER BY gathering_id, opened_at;

-- ListGatheringVoiceRoomFailures returns the failures of the rooms of the dates of gathering_ids,
-- a JSON array.
-- name: ListGatheringVoiceRoomFailures :many
SELECT gathering_id, confirmation_id, reason, failed_at FROM gathering_voice_room_failures
WHERE gathering_id IN (SELECT value FROM json_each(CAST(sqlc.arg(gathering_ids) AS TEXT)))
ORDER BY gathering_id, failed_at;

-- name: GetGatheringVoiceRoomByRoom :one
SELECT gathering_id, confirmation_id, room_id, opened_at FROM gathering_voice_rooms WHERE room_id = ?;
