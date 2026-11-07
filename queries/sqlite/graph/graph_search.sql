-- SPDX-License-Identifier: AGPL-3.0-only

-- The relations of a viewer with the candidates of the prefix search of accounts (ADR-0092
-- section 1): one query per relation over the candidates' IDs. The slice comes after every
-- other argument: sqlc numbers the arguments before it expands the slice.

-- ListBlockedAmong returns the accounts among ids that viewer blocks.
-- name: ListBlockedAmong :many
SELECT blocked_account_id FROM block_edges
WHERE blocker_account_id = sqlc.arg(viewer) AND blocked_account_id IN (sqlc.slice(ids))
ORDER BY blocked_account_id;

-- ListBlockersAmong returns the accounts among ids that block viewer.
-- name: ListBlockersAmong :many
SELECT blocker_account_id FROM block_edges
WHERE blocked_account_id = sqlc.arg(viewer) AND blocker_account_id IN (sqlc.slice(ids))
ORDER BY blocker_account_id;

-- ListMutedAmong returns the accounts among ids that viewer mutes at now.
-- name: ListMutedAmong :many
SELECT target_account_id FROM mutes
WHERE account_id = sqlc.arg(viewer) AND (expires_at IS NULL OR expires_at > CAST(sqlc.arg(now) AS INTEGER))
  AND target_account_id IN (sqlc.slice(ids))
ORDER BY target_account_id;

-- ListFollowedAmong returns the accounts among ids that viewer actively follows.
-- name: ListFollowedAmong :many
SELECT followee_account_id FROM follow_edges
WHERE follower_account_id = sqlc.arg(viewer) AND state = 'active' AND followee_account_id IN (sqlc.slice(ids))
ORDER BY followee_account_id;

-- ListFollowCountersAmong returns the counters of the accounts among ids that have them.
-- name: ListFollowCountersAmong :many
SELECT account_id, followers_count, following_count FROM follow_counters
WHERE account_id IN (sqlc.slice(ids))
ORDER BY account_id;
