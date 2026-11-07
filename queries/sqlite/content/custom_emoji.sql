-- SPDX-License-Identifier: AGPL-3.0-only
-- The custom emoji that the administrators manage and that the picker and local texts use
-- (ADR-0051). Owned by internal/app/content.

-- name: InsertCustomEmoji :exec
INSERT INTO custom_emojis (id, shortcode, domain, uri, created_by_user_id, created_at)
VALUES (@id, @shortcode, @domain, @uri, @created_by_user_id, @created_at);

-- name: DeleteCustomEmojiRow :exec
DELETE FROM custom_emojis WHERE id = @id;

-- name: DeleteCustomEmojiImages :exec
DELETE FROM custom_emoji_images WHERE emoji_id = @emoji_id;

-- name: UpsertCustomEmojiSettings :exec
INSERT INTO custom_emoji_settings (emoji_id, enabled, visible_in_picker, category)
VALUES (@emoji_id, @enabled, @visible_in_picker, @category)
ON CONFLICT (emoji_id) DO UPDATE SET enabled = excluded.enabled,
  visible_in_picker = excluded.visible_in_picker, category = excluded.category;

-- name: DeleteCustomEmojiSettings :exec
DELETE FROM custom_emoji_settings WHERE emoji_id = @emoji_id;

-- name: GetCustomEmojiSettings :one
SELECT emoji_id, enabled, visible_in_picker, category FROM custom_emoji_settings WHERE emoji_id = @emoji_id;

-- name: InsertCustomEmojiAlias :exec
INSERT INTO custom_emoji_aliases (emoji_id, alias) VALUES (@emoji_id, @alias) ON CONFLICT DO NOTHING;

-- name: DeleteCustomEmojiAliases :exec
DELETE FROM custom_emoji_aliases WHERE emoji_id = @emoji_id;

-- ListCustomEmojiAliases returns the aliases of the emoji with an ID in @emoji_ids.
-- name: ListCustomEmojiAliases :many
SELECT emoji_id, alias FROM custom_emoji_aliases
WHERE emoji_id IN (SELECT value FROM json_each(CAST(@emoji_ids AS TEXT)))
ORDER BY emoji_id, alias;

-- ListAdminCustomEmojis returns a page of the custom emoji, newest first, with their current
-- images and settings. @locality is 0 for every emoji, 1 for the local ones, 2 for the remote
-- ones; an empty @domain and @shortcode select every domain and shortcode, and @shortcode
-- matches a part of the shortcode in lower case; a @created_by keeps the emoji that a user made.
-- name: ListAdminCustomEmojis :many
SELECT e.id, e.shortcode, e.domain, e.uri, e.created_by_user_id, e.created_at,
  i.id AS image_id, i.media_id AS image_media_id, i.revised_at AS image_revised_at,
  CAST(COALESCE(s.enabled, 1) AS INTEGER) AS enabled,
  CAST(COALESCE(s.visible_in_picker, 1) AS INTEGER) AS visible_in_picker,
  CAST(COALESCE(s.category, '') AS TEXT) AS category
FROM custom_emojis e
JOIN custom_emoji_images i ON i.emoji_id = e.id
LEFT JOIN custom_emoji_settings s ON s.emoji_id = e.id
WHERE i.id = (SELECT MAX(l.id) FROM custom_emoji_images l WHERE l.emoji_id = e.id)
  AND (CAST(@locality AS INTEGER) = 0 OR (CAST(@locality AS INTEGER) = 1) = (e.domain = ''))
  AND (CAST(@domain AS TEXT) = '' OR e.domain = CAST(@domain AS TEXT))
  AND (CAST(@shortcode AS TEXT) = '' OR instr(lower(e.shortcode), CAST(@shortcode AS TEXT)) > 0)
  AND (CAST(@created_by AS TEXT) = '' OR e.created_by_user_id = CAST(@created_by AS TEXT))
  AND (CAST(@before_id AS TEXT) = '' OR e.id < CAST(@before_id AS TEXT))
ORDER BY e.id DESC
LIMIT @row_limit;

-- ListLocalCustomEmojis returns the local custom emoji with their current images and settings,
-- by shortcode: every one, or without @all_shortcodes those with a shortcode in @shortcodes (a
-- JSON array).
-- name: ListLocalCustomEmojis :many
SELECT e.id, e.shortcode, e.domain, e.uri, e.created_by_user_id, e.created_at,
  i.id AS image_id, i.media_id AS image_media_id, i.revised_at AS image_revised_at,
  CAST(COALESCE(s.enabled, 1) AS INTEGER) AS enabled,
  CAST(COALESCE(s.visible_in_picker, 1) AS INTEGER) AS visible_in_picker,
  CAST(COALESCE(s.category, '') AS TEXT) AS category
FROM custom_emojis e
JOIN custom_emoji_images i ON i.emoji_id = e.id
LEFT JOIN custom_emoji_settings s ON s.emoji_id = e.id
WHERE e.domain = ''
  AND i.id = (SELECT MAX(l.id) FROM custom_emoji_images l WHERE l.emoji_id = e.id)
  AND (CAST(@all_shortcodes AS INTEGER) = 1
    OR e.shortcode IN (SELECT value FROM json_each(CAST(@shortcodes AS TEXT))))
ORDER BY e.shortcode;

-- ListUnreferencedRemoteEmojis returns, in ID order after the cursor, the IDs of the remote custom
-- emoji that no status revision and no reaction refers to and that the administrators have not
-- disabled (the purge of ADR-0051; a disabled emoji stays, so that it stays disabled when it
-- arrives again).
-- name: ListUnreferencedRemoteEmojis :many
SELECT e.id FROM custom_emojis e
WHERE e.domain <> '' AND e.id > CAST(@after AS TEXT)
  AND NOT EXISTS (SELECT 1 FROM status_revision_emojis r WHERE r.emoji_id = e.id)
  AND NOT EXISTS (SELECT 1 FROM reactions x WHERE x.custom_emoji_id = e.id)
  AND NOT EXISTS (SELECT 1 FROM custom_emoji_settings s WHERE s.emoji_id = e.id AND s.enabled = 0)
ORDER BY e.id
LIMIT @row_limit;

-- name: ListCustomEmojiImageMediaIDs :many
SELECT DISTINCT media_id FROM custom_emoji_images WHERE emoji_id = @emoji_id ORDER BY media_id;

-- DeleteUnreferencedRemoteEmoji deletes the row of a remote custom emoji while no status
-- revision and no reaction refers to it and it is not disabled.
-- name: DeleteUnreferencedRemoteEmoji :execrows
DELETE FROM custom_emojis
WHERE custom_emojis.id = sqlc.arg(id) AND custom_emojis.domain <> ''
  AND NOT EXISTS (SELECT 1 FROM status_revision_emojis r WHERE r.emoji_id = sqlc.arg(id))
  AND NOT EXISTS (SELECT 1 FROM reactions x WHERE x.custom_emoji_id = sqlc.arg(id))
  AND NOT EXISTS (SELECT 1 FROM custom_emoji_settings s WHERE s.emoji_id = sqlc.arg(id) AND s.enabled = 0);

-- DeleteUnreferencedRemoteMedia deletes the remote subtype of a remote media file while no
-- custom emoji image and no attachment uses it.
-- name: DeleteUnreferencedRemoteMedia :execrows
DELETE FROM remote_media
WHERE remote_media.media_id = sqlc.arg(media_id)
  AND NOT EXISTS (SELECT 1 FROM custom_emoji_images c WHERE c.media_id = sqlc.arg(media_id))
  AND NOT EXISTS (SELECT 1 FROM status_revision_attachments a WHERE a.media_id = sqlc.arg(media_id));

-- name: DeleteRemoteMediaFile :exec
DELETE FROM media_files WHERE media_files.id = sqlc.arg(id) AND media_files.origin = 'remote';

-- ListExistingCustomEmojiIDs returns, in ID order, the custom emoji among ids that exist (the
-- audit log). The ids are a JSON array.
-- name: ListExistingCustomEmojiIDs :many
SELECT id FROM custom_emojis WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;
