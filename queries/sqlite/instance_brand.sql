-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertInstanceBrandRevision :exec
INSERT INTO instance_brand_revisions (id, kind, action, background_color, revised_by_user_id, revised_at)
VALUES (?, ?, ?, ?, ?, ?);

-- name: InsertInstanceBrandFile :exec
INSERT INTO instance_brand_files (brand_revision_id, rendition, blob_key, content_type, width, height, byte_size, sha256)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetInstanceBrandRevision :one
SELECT id, kind, action, background_color, revised_by_user_id, revised_at FROM instance_brand_revisions WHERE id = ?;

-- name: ListInstanceBrandFiles :many
SELECT rendition, blob_key, content_type, width, height, byte_size, sha256
FROM instance_brand_files WHERE brand_revision_id = ? ORDER BY rendition;

-- name: ListInstanceBrandRevisionHeads :many
SELECT id, kind, action FROM instance_brand_revisions ORDER BY id;

-- name: GetInstanceBrandHead :one
SELECT revision_id FROM instance_brands WHERE kind = ?;

-- name: ListInstanceBrandHeads :many
SELECT kind, revision_id FROM instance_brands ORDER BY kind;

-- name: UpsertInstanceBrandHead :exec
INSERT INTO instance_brands (kind, revision_id) VALUES (?, ?)
ON CONFLICT (kind) DO UPDATE SET revision_id = excluded.revision_id;

-- name: DeleteInstanceBrandHead :exec
DELETE FROM instance_brands WHERE kind = ?;

-- name: DeleteAllInstanceBrandHeads :exec
DELETE FROM instance_brands;
