-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertUserPreferences :exec
INSERT INTO user_preferences (
  user_id, locale, default_visibility, default_post_language, require_media_description,
  timeline_auto_insert, link_card_size, locale_follows_browser, time_zone_follows_browser, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateUserPreferences :execrows
UPDATE user_preferences SET
  locale = ?, default_visibility = ?, default_post_language = ?,
  require_media_description = ?, timeline_auto_insert = ?, link_card_size = ?,
  locale_follows_browser = ?, time_zone_follows_browser = ?, updated_at = ?
WHERE user_id = ?;

-- name: GetUserPreferences :one
SELECT user_id, locale, default_visibility, default_post_language, require_media_description,
  timeline_auto_insert, link_card_size, locale_follows_browser, time_zone_follows_browser, updated_at
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

-- name: ListUserInteractionPolicies :many
SELECT kind, approval, audience FROM user_interaction_policies WHERE user_id = sqlc.arg(user_id) ORDER BY kind, approval;

-- name: DeleteUserInteractionPolicies :exec
DELETE FROM user_interaction_policies WHERE user_id = sqlc.arg(user_id);

-- InsertUserInteractionPolicies inserts the rules of a user's defaults, a JSON array.
-- name: InsertUserInteractionPolicies :exec
INSERT INTO user_interaction_policies (user_id, kind, approval, audience)
SELECT CAST(sqlc.arg(user_id) AS TEXT), value ->> '$.kind', value ->> '$.approval', value ->> '$.audience'
FROM json_each(CAST(sqlc.arg(rules) AS TEXT));
