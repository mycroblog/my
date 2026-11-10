-- SPDX-License-Identifier: AGPL-3.0-only

-- The interactions with many statuses at once, one query per kind, for the pages that present
-- many statuses. Each reads what the query of one status of interaction.sql and reaction.sql
-- reads, in the same order within a status.

-- name: ListFavouritesOn :many
SELECT account_id, status_id, uri, created_at, id FROM favourites
WHERE account_id = sqlc.arg(account_id)::uuid AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: ListBoostEdgesOn :many
SELECT booster_account_id, status_id, boost_id FROM boost_edges
WHERE booster_account_id = sqlc.arg(account_id)::uuid AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: ListBookmarksOn :many
SELECT account_id, status_id, created_at, id FROM bookmarks
WHERE account_id = sqlc.arg(account_id)::uuid AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: ListStatusPinsOn :many
SELECT account_id, status_id, slot, created_at, id FROM status_pins
WHERE account_id = sqlc.arg(account_id)::uuid AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id;

-- name: ListAccountReactionsOn :many
SELECT id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at FROM reactions
WHERE account_id = sqlc.arg(account_id)::uuid AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY status_id, id;

-- ListReactionCountsOf returns the most used reactions of each status, at most page_limit of each,
-- as ListReactionCounts returns those of one: a reaction is kept while fewer than page_limit of
-- the status's come before it in that order (more uses, then the emoji key).
-- name: ListReactionCountsOf :many
SELECT c.status_id, c.emoji_key, c.custom_emoji_id, c.count FROM status_reaction_counts c
WHERE c.status_id = ANY(sqlc.arg(status_ids)::uuid[])
  AND (
    SELECT COUNT(*) FROM status_reaction_counts o
    WHERE o.status_id = c.status_id AND (o.count > c.count OR (o.count = c.count AND o.emoji_key < c.emoji_key))
  ) < sqlc.arg(page_limit)::bigint
ORDER BY c.status_id, c.count DESC, c.emoji_key;

-- name: ListBoostsByIDs :many
SELECT id, uri, booster_account_id, status_id, visibility, is_local, boosted_at FROM boosts
WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- What many accounts did with many statuses, for presenting one status to many viewers, each as
-- the query of one account above reads it (account_ids and status_ids are arrays).

-- name: ListFavouritesOfAccountsOn :many
SELECT account_id, status_id, uri, created_at, id FROM favourites
WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[]) AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY account_id, status_id;

-- name: ListBoostEdgesOfAccountsOn :many
SELECT booster_account_id, status_id, boost_id FROM boost_edges
WHERE booster_account_id = ANY(sqlc.arg(account_ids)::uuid[]) AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY booster_account_id, status_id;

-- name: ListBookmarksOfAccountsOn :many
SELECT account_id, status_id, created_at, id FROM bookmarks
WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[]) AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY account_id, status_id;

-- name: ListStatusPinsOfAccountsOn :many
SELECT account_id, status_id, slot, created_at, id FROM status_pins
WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[]) AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY account_id, status_id;

-- name: ListReactionsOfAccountsOn :many
SELECT id, account_id, status_id, emoji_key, custom_emoji_id, uri, created_at FROM reactions
WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[]) AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY account_id, status_id, id;

-- name: ListMentionsOfAccountsIn :many
SELECT DISTINCT account_id, status_id FROM status_mention_index WHERE account_id = ANY(sqlc.arg(account_ids)::uuid[]) AND status_id = ANY(sqlc.arg(status_ids)::uuid[]) ORDER BY account_id, status_id;
