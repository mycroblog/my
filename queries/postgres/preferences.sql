-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertUserPreferences :exec
INSERT INTO user_preferences (
  user_id, locale, default_visibility, default_quote_policy, default_post_language, require_media_description,
  timeline_auto_insert, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateUserPreferences :execrows
UPDATE user_preferences SET
  locale = $2, default_visibility = $3, default_quote_policy = $4, default_post_language = $5,
  require_media_description = $6, timeline_auto_insert = $7, updated_at = $8
WHERE user_id = $1;

-- name: GetUserPreferences :one
SELECT user_id, locale, default_visibility, default_quote_policy, default_post_language, require_media_description,
  timeline_auto_insert, updated_at
FROM user_preferences WHERE user_id = $1;

-- GetUserPreferencesForUpdate is GetUserPreferences that locks the row until the transaction ends.
-- name: GetUserPreferencesForUpdate :one
SELECT user_id, locale, default_visibility, default_quote_policy, default_post_language, require_media_description,
  timeline_auto_insert, updated_at
FROM user_preferences WHERE user_id = $1
FOR UPDATE;

-- name: DeleteUserTimelineLanguages :exec
DELETE FROM user_timeline_languages WHERE user_id = $1;

-- name: InsertUserTimelineLanguage :exec
INSERT INTO user_timeline_languages (user_id, language) VALUES ($1, $2);

-- name: ListUserTimelineLanguages :many
SELECT language FROM user_timeline_languages WHERE user_id = $1 ORDER BY language;

-- name: UpsertUserTimeZone :exec
INSERT INTO user_time_zones (user_id, time_zone) VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET time_zone = excluded.time_zone;

-- name: GetUserTimeZone :one
SELECT time_zone FROM user_time_zones WHERE user_id = $1;
