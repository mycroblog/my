-- SPDX-License-Identifier: AGPL-3.0-only

-- The parts of many media files at once, one query per part, for the pages that present many
-- attachments (FindMediaParts). Each reads what the query of one of media.sql reads, in the same
-- order within a file. The lists are JSON arrays.

-- name: ListMediaFilesByIDs :many
SELECT id, owner_account_id, origin, created_at FROM media_files WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(media_ids) AS TEXT))) ORDER BY id;

-- name: ListUploadedMediaByIDs :many
SELECT media_id, blob_key, content_type, file_size FROM uploaded_media WHERE media_id IN (SELECT value FROM json_each(CAST(sqlc.arg(media_ids) AS TEXT))) ORDER BY media_id;

-- name: ListMediaProcessingResultsOf :many
SELECT id, media_id, purpose, outcome, media_type, width, height, duration_ms, blurhash, processed_at
FROM media_processing_results WHERE media_id IN (SELECT value FROM json_each(CAST(sqlc.arg(media_ids) AS TEXT))) ORDER BY media_id, id;

-- name: ListMediaVariantsOfMedia :many
SELECT v.result_id, v.size, v.motion, v.blob_key, v.content_type, v.file_size, v.width, v.height
FROM media_variants v JOIN media_processing_results r ON r.id = v.result_id
WHERE r.media_id IN (SELECT value FROM json_each(CAST(sqlc.arg(media_ids) AS TEXT))) ORDER BY v.result_id, v.size, v.motion;

-- name: ListMediaFocusOf :many
SELECT media_id, focus_x, focus_y FROM media_focus WHERE media_id IN (SELECT value FROM json_each(CAST(sqlc.arg(media_ids) AS TEXT))) ORDER BY media_id;

-- name: ListMediaDescriptionsOf :many
SELECT media_id, description FROM media_descriptions WHERE media_id IN (SELECT value FROM json_each(CAST(sqlc.arg(media_ids) AS TEXT))) ORDER BY media_id;
