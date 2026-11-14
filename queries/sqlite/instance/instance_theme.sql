-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertInstanceThemeSet :exec
INSERT INTO instance_theme_sets (id, document, saved_by_user_id, saved_at) VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetInstanceThemeSet :one
SELECT id, document, saved_by_user_id, saved_at FROM instance_theme_sets WHERE id = ?;

-- name: GetLatestInstanceThemeSetID :one
SELECT id FROM instance_theme_sets ORDER BY id DESC LIMIT 1;

-- name: InsertInstanceThemePublication :exec
INSERT INTO instance_theme_publications (id, set_id, published_by_user_id, published_at) VALUES (?, ?, ?, ?)
ON CONFLICT (id) DO NOTHING;

-- name: GetInstanceThemePublication :one
SELECT id, set_id, published_by_user_id, published_at FROM instance_theme_publications WHERE id = ?;

-- name: GetLatestInstanceThemePublicationID :one
SELECT id FROM instance_theme_publications ORDER BY id DESC LIMIT 1;

-- name: GetPreviousInstanceThemePublication :one
SELECT id, set_id, published_by_user_id, published_at FROM instance_theme_publications
WHERE id < sqlc.arg(before_id) AND set_id <> sqlc.arg(set_id) ORDER BY id DESC LIMIT 1;

-- name: GetInstanceThemeHead :one
SELECT revision_id FROM instance_theme WHERE id = 1;

-- name: UpsertInstanceThemeHead :exec
INSERT INTO instance_theme (id, revision_id) VALUES (1, ?)
ON CONFLICT (id) DO UPDATE SET revision_id = excluded.revision_id;

-- name: DeleteInstanceThemeHead :exec
DELETE FROM instance_theme;

-- name: GetInstanceThemeDraft :one
SELECT set_id FROM instance_theme_draft WHERE id = 1;

-- name: UpsertInstanceThemeDraft :exec
INSERT INTO instance_theme_draft (id, set_id) VALUES (1, ?)
ON CONFLICT (id) DO UPDATE SET set_id = excluded.set_id;

-- name: DeleteInstanceThemeDraft :exec
DELETE FROM instance_theme_draft;
