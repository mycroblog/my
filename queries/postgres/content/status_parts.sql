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
