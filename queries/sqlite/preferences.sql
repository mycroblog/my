-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertUserPreferences :exec
INSERT INTO user_preferences (
  user_id, locale, default_visibility, default_quote_policy, default_post_language, require_media_description,
  timeline_auto_insert, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateUserPreferences :execrows
UPDATE user_preferences SET
  locale = ?, default_visibility = ?, default_quote_policy = ?, default_post_language = ?,
  require_media_description = ?, timeline_auto_insert = ?, updated_at = ?
WHERE user_id = ?;

-- name: GetUserPreferences :one
SELECT user_id, locale, default_visibility, default_quote_policy, default_post_language, require_media_description,
  timeline_auto_insert, updated_at
FROM user_preferences WHERE user_id = ?;

-- name: DeleteUserTimelineLanguages :exec
DELETE FROM user_timeline_languages WHERE user_id = ?;

-- name: InsertUserTimelineLanguage :exec
INSERT INTO user_timeline_languages (user_id, language) VALUES (?, ?);

-- name: ListUserTimelineLanguages :many
SELECT language FROM user_timeline_languages WHERE user_id = ? ORDER BY language;

-- name: UpsertUserTimeZone :exec
INSERT INTO user_time_zones (user_id, time_zone) VALUES (?, ?)
ON CONFLICT (user_id) DO UPDATE SET time_zone = excluded.time_zone;

-- name: GetUserTimeZone :one
SELECT time_zone FROM user_time_zones WHERE user_id = ?;
