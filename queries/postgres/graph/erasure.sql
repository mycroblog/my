-- SPDX-License-Identifier: AGPL-3.0-only

-- The erasure of deleted accounts' data (data-model sections 4.2 and 8, #397). Each module
-- erases its own tables; the statements whose rows grow with the number of users or followers
-- take a bounded batch (@lim). A list argument is an array of strings.

-- ListFollowEdgesToErase returns up to @lim live follows into or out of the account.
-- name: ListFollowEdgesToErase :many
SELECT * FROM follow_edges
WHERE follower_account_id = sqlc.arg(account_id) OR followee_account_id = sqlc.arg(account_id)
ORDER BY follow_id
LIMIT sqlc.arg(lim);

-- name: ListFollowIDsToErase :many
SELECT id FROM follows
WHERE follower_account_id = sqlc.arg(account_id) OR followee_account_id = sqlc.arg(account_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseFollowAcceptances :exec
DELETE FROM follow_acceptances WHERE follow_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFollowRejections :exec
DELETE FROM follow_rejections WHERE follow_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFollowTerminations :exec
DELETE FROM follow_terminations WHERE follow_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFollowResends :exec
DELETE FROM follow_resends WHERE follow_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFollowPreferences :exec
DELETE FROM follow_preferences WHERE follow_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFollowEdgesOfFollows :exec
DELETE FROM follow_edges WHERE follow_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseFollows :exec
DELETE FROM follows WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListBlockIDsToErase :many
SELECT id FROM blocks
WHERE blocker_account_id = sqlc.arg(account_id) OR blocked_account_id = sqlc.arg(account_id)
ORDER BY id
LIMIT sqlc.arg(lim);

-- name: EraseBlockRetractions :exec
DELETE FROM block_retractions WHERE block_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseBlockEdgesOfBlocks :exec
DELETE FROM block_edges WHERE block_id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseBlocks :exec
DELETE FROM blocks WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: EraseMutesBy :execrows
DELETE FROM mutes WHERE mutes.account_id = sqlc.arg(account_id) AND mutes.target_account_id IN (
  SELECT m.target_account_id FROM mutes m WHERE m.account_id = sqlc.arg(account_id)
  ORDER BY m.target_account_id LIMIT sqlc.arg(lim));

-- name: EraseMutesOf :execrows
DELETE FROM mutes WHERE mutes.target_account_id = sqlc.arg(account_id) AND mutes.account_id IN (
  SELECT m.account_id FROM mutes m WHERE m.target_account_id = sqlc.arg(account_id)
  ORDER BY m.account_id LIMIT sqlc.arg(lim));

-- name: EraseDomainMutesBy :execrows
DELETE FROM account_domain_mutes WHERE account_domain_mutes.account_id = sqlc.arg(account_id) AND account_domain_mutes.domain IN (
  SELECT m.domain FROM account_domain_mutes m WHERE m.account_id = sqlc.arg(account_id)
  ORDER BY m.domain LIMIT sqlc.arg(lim));

-- name: EraseFollowCounters :exec
DELETE FROM follow_counters WHERE account_id = sqlc.arg(account_id);
