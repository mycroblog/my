-- The voice chat rooms of the events (ADR-0161, section 10).

-- name: InsertGatheringVoiceRoom :execrows
INSERT INTO gathering_voice_rooms (gathering_id, confirmation_id, room_id, opened_at) VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: InsertGatheringVoiceRoomFailure :execrows
INSERT INTO gathering_voice_room_failures (gathering_id, confirmation_id, reason, failed_at) VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- ListGatheringVoiceRooms returns the rooms of the dates of gathering_ids.
-- name: ListGatheringVoiceRooms :many
SELECT gathering_id, confirmation_id, room_id, opened_at FROM gathering_voice_rooms
WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[])
ORDER BY gathering_id, opened_at;

-- ListGatheringVoiceRoomFailures returns the failures of the rooms of the dates of gathering_ids.
-- name: ListGatheringVoiceRoomFailures :many
SELECT gathering_id, confirmation_id, reason, failed_at FROM gathering_voice_room_failures
WHERE gathering_id = ANY(sqlc.arg(gathering_ids)::uuid[])
ORDER BY gathering_id, failed_at;

-- name: GetGatheringVoiceRoomByRoom :one
SELECT gathering_id, confirmation_id, room_id, opened_at FROM gathering_voice_rooms WHERE room_id = $1;
