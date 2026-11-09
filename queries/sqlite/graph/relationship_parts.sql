-- SPDX-License-Identifier: AGPL-3.0-only

-- The relationship of one account with many others, one query per part, for the pages that show
-- many accounts (FindRelationshipParts). Each reads what the query of one pair of graph.sql and
-- graph_blocks.sql reads. The lists are JSON arrays.

-- name: ListFollowEdgesFrom :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges WHERE follower_account_id = sqlc.arg(account_id) AND followee_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(targets) AS TEXT)))
ORDER BY followee_account_id;

-- name: ListFollowEdgesTo :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges WHERE followee_account_id = sqlc.arg(account_id) AND follower_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(targets) AS TEXT)))
ORDER BY follower_account_id;

-- name: ListFollowPreferences :many
SELECT follow_id, show_boosts FROM follow_preferences WHERE follow_id IN (SELECT value FROM json_each(CAST(sqlc.arg(follow_ids) AS TEXT))) ORDER BY follow_id;

-- name: ListBlockEdgesFrom :many
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges WHERE blocker_account_id = sqlc.arg(account_id) AND blocked_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(targets) AS TEXT)))
ORDER BY blocked_account_id;

-- name: ListBlockEdgesTo :many
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges WHERE blocked_account_id = sqlc.arg(account_id) AND blocker_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(targets) AS TEXT)))
ORDER BY blocker_account_id;

-- name: ListActiveMutesOf :many
SELECT account_id, target_account_id, hide_notifications, expires_at, created_at
FROM mutes
WHERE account_id = sqlc.arg(account_id) AND target_account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(targets) AS TEXT)))
  AND (expires_at IS NULL OR expires_at > CAST(sqlc.arg(now) AS INTEGER))
ORDER BY target_account_id;

-- The mutes of the accounts among targets of account_id, as GetActiveMute reads one.
-- name: ListActiveMutesTo :many
SELECT account_id, target_account_id, hide_notifications, expires_at, created_at
FROM mutes
WHERE target_account_id = sqlc.arg(account_id) AND account_id IN (SELECT value FROM json_each(CAST(sqlc.arg(targets) AS TEXT)))
  AND (expires_at IS NULL OR expires_at > CAST(sqlc.arg(now) AS INTEGER))
ORDER BY account_id;
