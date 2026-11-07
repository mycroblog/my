-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertInstanceThemeRevision :exec
INSERT INTO instance_theme_revisions (id, custom_css, revised_by_user_id, revised_at) VALUES ($1, $2, $3, $4);

-- name: InsertInstanceThemeToken :exec
INSERT INTO instance_theme_tokens (revision_id, token, scheme, value) VALUES ($1, $2, $3, $4);

-- name: GetInstanceThemeRevision :one
SELECT id, custom_css, revised_by_user_id, revised_at FROM instance_theme_revisions WHERE id = $1;

-- name: ListInstanceThemeTokens :many
SELECT token, scheme, value FROM instance_theme_tokens WHERE revision_id = $1 ORDER BY token, scheme;

-- name: GetLatestInstanceThemeRevisionID :one
SELECT id FROM instance_theme_revisions ORDER BY id DESC LIMIT 1;

-- name: GetInstanceThemeHead :one
SELECT revision_id FROM instance_theme WHERE id = 1;

-- name: UpsertInstanceThemeHead :exec
INSERT INTO instance_theme (id, revision_id) VALUES (1, $1)
ON CONFLICT (id) DO UPDATE SET revision_id = excluded.revision_id;

-- name: DeleteInstanceThemeHead :exec
DELETE FROM instance_theme;
