-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertUserPreferences :exec
INSERT INTO user_preferences (
  user_id, locale, default_visibility, default_post_language, require_media_description,
  timeline_auto_insert, link_card_size, locale_follows_browser, time_zone_follows_browser, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: UpdateUserPreferences :execrows
UPDATE user_preferences SET
  locale = $2, default_visibility = $3, default_post_language = $4,
  require_media_description = $5, timeline_auto_insert = $6, link_card_size = $7,
  locale_follows_browser = $8, time_zone_follows_browser = $9, updated_at = $10
WHERE user_id = $1;

-- name: GetUserPreferences :one
SELECT user_id, locale, default_visibility, default_post_language, require_media_description,
  timeline_auto_insert, link_card_size, locale_follows_browser, time_zone_follows_browser, updated_at
FROM user_preferences WHERE user_id = $1;

-- GetUserPreferencesForUpdate is GetUserPreferences that locks the row until the transaction ends.
-- name: GetUserPreferencesForUpdate :one
SELECT user_id, locale, default_visibility, default_post_language, require_media_description,
  timeline_auto_insert, link_card_size, locale_follows_browser, time_zone_follows_browser, updated_at
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

-- name: ListUserInteractionPolicies :many
SELECT kind, approval, audience FROM user_interaction_policies WHERE user_id = sqlc.arg(user_id)::uuid ORDER BY kind, approval;

-- name: DeleteUserInteractionPolicies :exec
DELETE FROM user_interaction_policies WHERE user_id = sqlc.arg(user_id)::uuid;

-- InsertUserInteractionPolicies inserts the rules of a user's defaults (the unnests of a select
-- list advance together).
-- name: InsertUserInteractionPolicies :exec
INSERT INTO user_interaction_policies (user_id, kind, approval, audience)
SELECT sqlc.arg(user_id)::uuid, unnest(sqlc.arg(kinds)::text[]), unnest(sqlc.arg(approvals)::text[]), unnest(sqlc.arg(audiences)::text[]);
