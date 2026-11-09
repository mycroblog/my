-- SPDX-License-Identifier: AGPL-3.0-only

-- Parts of many statuses at once, one query per part, for the pages that present many statuses.
-- Each reads what the query of one status of content.sql and interaction.sql reads.

-- name: ListStatusDeletionsOf :many
SELECT status_id, uri, author_account_id, origin, moderation_action_id, deleted_at FROM status_deletions
WHERE status_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY status_id;

-- name: ListTagsByIDs :many
SELECT id, name, name_normalized, created_at FROM tags WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- name: ListStatusCountersOf :many
SELECT status_id, replies_count, boosts_count, favourites_count FROM status_counters WHERE status_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY status_id;

-- ListStatusesMentioning returns the statuses among ids that mention account_id.
-- name: ListStatusesMentioning :many
SELECT DISTINCT status_id FROM status_mention_index WHERE account_id = sqlc.arg(account_id) AND status_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY status_id;

-- name: ListAccountsMentionedIn :many
SELECT DISTINCT account_id FROM status_mention_index WHERE status_id = sqlc.arg(status_id) AND account_id = ANY(sqlc.arg(account_ids)::uuid[]) ORDER BY account_id;

-- name: ListCustomEmojisByIDs :many
SELECT id, shortcode, domain, uri, created_by_user_id, created_at FROM custom_emojis WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- name: InsertStatusMentions :exec
INSERT INTO status_mention_index (account_id, status_id)
SELECT unnest(sqlc.arg(account_ids)::uuid[]), sqlc.arg(status_id)::uuid
ON CONFLICT (account_id, status_id) DO NOTHING;

-- name: InsertStatusRevisionMentions :exec
INSERT INTO status_revision_mentions (revision_id, account_id)
SELECT sqlc.arg(revision_id)::uuid, unnest(sqlc.arg(account_ids)::uuid[]);

-- InsertStatusDeletions records the deletions of many statuses by one moderation action, as
-- InsertStatusDeletion records one (the unnests of a select list advance together).
-- name: InsertStatusDeletions :exec
INSERT INTO status_deletions (status_id, uri, author_account_id, origin, moderation_action_id, deleted_at)
SELECT unnest(sqlc.arg(status_ids)::uuid[]), unnest(sqlc.arg(uris)::text[]), unnest(sqlc.arg(author_account_ids)::uuid[]),
  sqlc.arg(origin)::text, sqlc.narg(moderation_action_id)::uuid, sqlc.arg(deleted_at)::timestamptz;

-- InsertStatusSensitivityOverrides is InsertStatusSensitivityOverride of many statuses.
-- name: InsertStatusSensitivityOverrides :exec
INSERT INTO status_sensitivity_overrides (status_id, source_kind, source_id)
SELECT unnest(sqlc.arg(status_ids)::uuid[]), 'action', sqlc.arg(source_id)::uuid
ON CONFLICT (status_id, source_kind, source_id) DO NOTHING;

-- The tags, emoji, and attachments of a revision, each with one statement as its mentions (the
-- unnests of a select list advance together).

-- name: InsertStatusRevisionTags :exec
INSERT INTO status_revision_tags (revision_id, tag_id)
SELECT sqlc.arg(revision_id)::uuid, unnest(sqlc.arg(tag_ids)::uuid[]);

-- name: InsertStatusRevisionEmojis :exec
INSERT INTO status_revision_emojis (revision_id, emoji_id)
SELECT sqlc.arg(revision_id)::uuid, unnest(sqlc.arg(emoji_ids)::uuid[]);

-- name: InsertStatusRevisionAttachments :exec
INSERT INTO status_revision_attachments (revision_id, position, media_id, description)
SELECT sqlc.arg(revision_id)::uuid, unnest(sqlc.arg(positions)::int[]), unnest(sqlc.arg(media_ids)::uuid[]),
  NULLIF(unnest(sqlc.arg(descriptions)::text[]), '');

-- InsertTagsIfAbsent is InsertTagIfAbsent of many tags, and ListTagsByNames reads the tags of
-- many normalized names, as GetTagByName reads one.
-- name: InsertTagsIfAbsent :exec
INSERT INTO tags (id, name, name_normalized, created_at)
SELECT unnest(sqlc.arg(ids)::uuid[]), unnest(sqlc.arg(names)::text[]), unnest(sqlc.arg(names_normalized)::text[]), sqlc.arg(created_at)::timestamptz
ON CONFLICT (name_normalized) DO NOTHING;

-- name: ListTagsByNames :many
SELECT id, name, name_normalized, created_at FROM tags WHERE name_normalized = ANY(sqlc.arg(names_normalized)::text[]) ORDER BY name_normalized;
