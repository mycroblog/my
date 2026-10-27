-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertMediaFile :exec
INSERT INTO media_files (id, owner_account_id, origin, created_at) VALUES ($1, $2, $3, $4);

-- name: InsertUploadedMedia :exec
INSERT INTO uploaded_media (media_id, blob_key, content_type, file_size) VALUES ($1, $2, $3, $4);

-- name: InsertMediaProcessingResult :exec
INSERT INTO media_processing_results (
  id, media_id, purpose, outcome, media_type, width, height, duration_ms, blurhash, processed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: InsertMediaVariant :exec
INSERT INTO media_variants (result_id, size, motion, blob_key, content_type, file_size, width, height)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- HasCurrentMediaProcessingResult reports whether the newest processing result of a media file
-- is for a purpose.
-- name: HasCurrentMediaProcessingResult :one
SELECT EXISTS (
  SELECT 1 FROM media_processing_results r
  WHERE r.purpose = sqlc.arg(purpose) AND r.id = (
    SELECT c.id FROM media_processing_results c WHERE c.media_id = sqlc.arg(media_id) ORDER BY c.id DESC LIMIT 1)
);

-- name: UpsertMediaFocus :exec
INSERT INTO media_focus (media_id, focus_x, focus_y) VALUES ($1, $2, $3)
ON CONFLICT (media_id) DO UPDATE SET focus_x = excluded.focus_x, focus_y = excluded.focus_y;

-- name: DeleteUnattachedMediaFile :execrows
DELETE FROM media_files
WHERE media_files.id = sqlc.arg(id)
  AND NOT EXISTS (SELECT 1 FROM status_revision_attachments a WHERE a.media_id = sqlc.arg(id))
  AND NOT EXISTS (SELECT 1 FROM custom_emoji_images c WHERE c.media_id = sqlc.arg(id));

-- name: DeleteUploadedMedia :exec
DELETE FROM uploaded_media WHERE media_id = $1;

-- name: DeleteMediaVariantsOf :exec
DELETE FROM media_variants WHERE result_id IN (SELECT id FROM media_processing_results WHERE media_id = $1);

-- name: DeleteMediaProcessingResults :exec
DELETE FROM media_processing_results WHERE media_id = $1;

-- name: DeleteMediaFocus :exec
DELETE FROM media_focus WHERE media_id = $1;

-- name: GetMediaFile :one
SELECT id, owner_account_id, origin, created_at FROM media_files WHERE id = $1;

-- name: GetUploadedMedia :one
SELECT media_id, blob_key, content_type, file_size FROM uploaded_media WHERE media_id = $1;

-- name: ListMediaProcessingResults :many
SELECT id, media_id, purpose, outcome, media_type, width, height, duration_ms, blurhash, processed_at
FROM media_processing_results WHERE media_id = $1 ORDER BY id;

-- name: ListMediaVariantsOf :many
SELECT v.result_id, v.size, v.motion, v.blob_key, v.content_type, v.file_size, v.width, v.height
FROM media_variants v JOIN media_processing_results r ON r.id = v.result_id
WHERE r.media_id = $1 ORDER BY v.result_id, v.size, v.motion;

-- name: GetMediaFocus :one
SELECT media_id, focus_x, focus_y FROM media_focus WHERE media_id = $1;

-- Uploads by ID, which is a UUIDv7 and so also orders and bounds them by creation time, that no
-- status revision attaches and no custom emoji uses as its image.
-- name: ListUnattachedUploads :many
SELECT m.id FROM media_files m
WHERE m.origin = 'upload' AND m.id > COALESCE(sqlc.narg(after_id)::uuid, '00000000-0000-0000-0000-000000000000') AND m.id < sqlc.arg(before_id)
  AND NOT EXISTS (SELECT 1 FROM status_revision_attachments a WHERE a.media_id = m.id)
  AND NOT EXISTS (SELECT 1 FROM custom_emoji_images c WHERE c.media_id = m.id)
ORDER BY m.id
LIMIT sqlc.arg(row_limit);

-- The uploads of an owner by ID, with whether a status revision attaches each or a custom emoji
-- uses it as its image, for the erasure of a deleted account (data-model section 8): a custom
-- emoji of the instance outlives the administrator who uploaded its image.
-- name: ListOwnerUploads :many
SELECT m.id, CAST(EXISTS (SELECT 1 FROM status_revision_attachments a WHERE a.media_id = m.id)
  OR EXISTS (SELECT 1 FROM custom_emoji_images c WHERE c.media_id = m.id) AS BOOLEAN) AS attached
FROM media_files m
WHERE m.owner_account_id = sqlc.arg(owner_account_id) AND m.origin = 'upload' AND m.id > COALESCE(sqlc.narg(after_id)::uuid, '00000000-0000-0000-0000-000000000000')
ORDER BY m.id
LIMIT sqlc.arg(row_limit);

-- name: UpsertMediaDescription :exec
INSERT INTO media_descriptions (media_id, description) VALUES ($1, $2)
ON CONFLICT (media_id) DO UPDATE SET description = excluded.description;

-- name: DeleteMediaDescription :exec
DELETE FROM media_descriptions WHERE media_id = $1;

-- name: GetMediaDescription :one
SELECT media_id, description FROM media_descriptions WHERE media_id = $1;

-- name: IsMediaAttached :one
SELECT EXISTS (SELECT 1 FROM status_revision_attachments WHERE media_id = $1);

-- ListReadyMediaVariants returns the variants of the current (newest) processing result of each
-- listed media file, with the file, for the files whose current result is ready.
-- name: ListReadyMediaVariants :many
SELECT m.id, m.owner_account_id, m.origin, m.created_at,
  v.result_id, v.size, v.motion, v.blob_key, v.content_type, v.file_size, v.width, v.height
FROM media_files m
JOIN media_processing_results r ON r.id = (
  SELECT c.id FROM media_processing_results c WHERE c.media_id = m.id ORDER BY c.id DESC LIMIT 1)
JOIN media_variants v ON v.result_id = r.id
WHERE m.id = ANY(sqlc.arg(ids)::uuid[])
  AND r.outcome = 'ready'
ORDER BY m.id, v.size, v.motion;

-- SumLocalMediaSize returns the bytes that the local uploads keep: the variants of the current
-- (newest) processing result of each upload, and the originals of the uploads that wait for
-- their first processing (ADR-0055 section 3, my.media.stored_bytes).
-- name: SumLocalMediaSize :one
SELECT CAST(COALESCE(SUM(s.file_size), 0) AS BIGINT) FROM (
  SELECT v.file_size FROM media_files m
  JOIN media_processing_results r ON r.id = (
    SELECT c.id FROM media_processing_results c WHERE c.media_id = m.id ORDER BY c.id DESC LIMIT 1)
  JOIN media_variants v ON v.result_id = r.id
  WHERE m.origin = 'upload'
  UNION ALL
  SELECT u.file_size FROM uploaded_media u
  WHERE NOT EXISTS (SELECT 1 FROM media_processing_results p WHERE p.media_id = u.media_id)
) AS s;

-- name: UpsertMediaBlobDeletion :exec
INSERT INTO media_blob_deletions (blob_key, delete_after) VALUES (@blob_key, @delete_after)
ON CONFLICT (blob_key) DO UPDATE SET delete_after = excluded.delete_after;

-- name: DeleteMediaBlobDeletion :execrows
DELETE FROM media_blob_deletions WHERE blob_key = @blob_key;

-- ListDueMediaBlobDeletions returns at most @batch_limit keys due at or before @now, the earliest
-- first, for the cleanup to delete.
-- name: ListDueMediaBlobDeletions :many
SELECT blob_key FROM media_blob_deletions WHERE delete_after <= @now
ORDER BY delete_after, blob_key LIMIT @batch_limit;
