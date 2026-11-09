-- SPDX-License-Identifier: AGPL-3.0-only

-- Parts of many statuses at once, one query per part, for the pages that present many statuses.
-- Each reads what the query of one status of content.sql and interaction.sql reads. The lists are JSON arrays.

-- name: ListStatusDeletionsOf :many
SELECT status_id, uri, author_account_id, origin, moderation_action_id, deleted_at FROM status_deletions
WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY status_id;

-- name: ListTagsByIDs :many
SELECT id, name, name_normalized, created_at FROM tags WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;

-- name: ListStatusCountersOf :many
SELECT status_id, replies_count, boosts_count, favourites_count FROM status_counters WHERE status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY status_id;

-- ListStatusesMentioning returns the statuses among ids that mention account_id.
-- name: ListStatusesMentioning :many
SELECT DISTINCT status_id FROM status_mention_index WHERE account_id = sqlc.arg(account_id) AND status_id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY status_id;

-- name: ListAccountsMentionedIn :many
SELECT DISTINCT account_id FROM status_mention_index WHERE status_id = sqlc.arg(status_id) AND account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT))) ORDER BY account_id;

-- name: ListCustomEmojisByIDs :many
SELECT id, shortcode, domain, uri, created_by_user_id, created_at FROM custom_emojis WHERE id IN (SELECT value FROM json_each(CAST(sqlc.arg(ids) AS TEXT))) ORDER BY id;

-- name: InsertStatusMentions :exec
INSERT INTO status_mention_index (account_id, status_id)
SELECT value, CAST(sqlc.arg(status_id) AS TEXT) FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT)) WHERE true
ON CONFLICT (account_id, status_id) DO NOTHING;

-- name: InsertStatusRevisionMentions :exec
INSERT INTO status_revision_mentions (revision_id, account_id)
SELECT CAST(sqlc.arg(revision_id) AS TEXT), value FROM json_each(CAST(sqlc.arg(account_ids) AS TEXT));

-- InsertStatusDeletions records the deletions of many statuses by one moderation action, a JSON
-- array, as InsertStatusDeletion records one.
-- name: InsertStatusDeletions :exec
INSERT INTO status_deletions (status_id, uri, author_account_id, origin, moderation_action_id, deleted_at)
SELECT value ->> '$.status_id', value ->> '$.uri', value ->> '$.author_account_id',
  CAST(sqlc.arg(origin) AS TEXT), CAST(sqlc.narg(moderation_action_id) AS TEXT), CAST(sqlc.arg(deleted_at) AS INTEGER)
FROM json_each(CAST(sqlc.arg(deletions) AS TEXT));

-- InsertStatusSensitivityOverrides is InsertStatusSensitivityOverride of many statuses, a JSON
-- array; WHERE true keeps ON CONFLICT from reading as a join constraint.
-- name: InsertStatusSensitivityOverrides :exec
INSERT INTO status_sensitivity_overrides (status_id, source_kind, source_id)
SELECT value, 'action', CAST(sqlc.arg(source_id) AS TEXT) FROM json_each(CAST(sqlc.arg(status_ids) AS TEXT)) WHERE true
ON CONFLICT (status_id, source_kind, source_id) DO NOTHING;
