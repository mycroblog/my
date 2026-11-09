-- SPDX-License-Identifier: AGPL-3.0-only

-- The relationship of one account with many others, one query per part, for the pages that show
-- many accounts (FindRelationshipParts). Each reads what the query of one pair of graph.sql and
-- graph_blocks.sql reads.

-- name: ListFollowEdgesFrom :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges WHERE follower_account_id = sqlc.arg(account_id) AND followee_account_id = ANY(sqlc.arg(targets)::uuid[])
ORDER BY followee_account_id;

-- name: ListFollowEdgesTo :many
SELECT follower_account_id, followee_account_id, follow_id, state, since
FROM follow_edges WHERE followee_account_id = sqlc.arg(account_id) AND follower_account_id = ANY(sqlc.arg(targets)::uuid[])
ORDER BY follower_account_id;

-- name: ListFollowPreferences :many
SELECT follow_id, show_boosts FROM follow_preferences WHERE follow_id = ANY(sqlc.arg(follow_ids)::uuid[]) ORDER BY follow_id;

-- name: ListBlockEdgesFrom :many
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges WHERE blocker_account_id = sqlc.arg(account_id) AND blocked_account_id = ANY(sqlc.arg(targets)::uuid[])
ORDER BY blocked_account_id;

-- name: ListBlockEdgesTo :many
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges WHERE blocked_account_id = sqlc.arg(account_id) AND blocker_account_id = ANY(sqlc.arg(targets)::uuid[])
ORDER BY blocker_account_id;

-- name: ListActiveMutesOf :many
SELECT account_id, target_account_id, hide_notifications, expires_at, created_at
FROM mutes
WHERE account_id = sqlc.arg(account_id) AND target_account_id = ANY(sqlc.arg(targets)::uuid[])
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now)::timestamptz)
ORDER BY target_account_id;
