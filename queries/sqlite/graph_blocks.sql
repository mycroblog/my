-- SPDX-License-Identifier: AGPL-3.0-only

-- name: InsertBlock :exec
INSERT INTO blocks (id, uri, blocker_account_id, blocked_account_id, created_at) VALUES (?, ?, ?, ?, ?);

-- name: InsertBlockRetraction :exec
INSERT INTO block_retractions (block_id, retracted_at) VALUES (?, ?);

-- name: GetBlock :one
SELECT id, uri, blocker_account_id, blocked_account_id, created_at FROM blocks WHERE id = ?;

-- GetBlockByURI returns the block with an activity URI and its retraction, if any.
-- name: GetBlockByURI :one
SELECT b.id, b.uri, b.blocker_account_id, b.blocked_account_id, b.created_at, r.retracted_at
FROM blocks b
LEFT JOIN block_retractions r ON r.block_id = b.id
WHERE b.uri = ?;

-- name: GetBlockEdge :one
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges WHERE blocker_account_id = ? AND blocked_account_id = ?;

-- InsertBlockEdge inserts nothing when the pair already has an edge.
-- name: InsertBlockEdge :execrows
INSERT INTO block_edges (blocker_account_id, blocked_account_id, block_id) VALUES (?, ?, ?)
ON CONFLICT (blocker_account_id, blocked_account_id) DO NOTHING;

-- name: DeleteBlockEdge :exec
DELETE FROM block_edges WHERE blocker_account_id = ? AND blocked_account_id = ? AND block_id = ?;

-- name: DeleteAllBlockEdges :exec
DELETE FROM block_edges;

-- name: ListBlockHistories :many
SELECT b.id, b.uri, b.blocker_account_id, b.blocked_account_id, b.created_at, r.retracted_at
FROM blocks b
LEFT JOIN block_retractions r ON r.block_id = b.id
ORDER BY b.id;

-- name: ListBlockEdges :many
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges ORDER BY blocker_account_id, blocked_account_id;

-- ListBlockingPage pages the edges out of a blocker, newest block first; an empty before starts
-- at the newest.
-- name: ListBlockingPage :many
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges
WHERE blocker_account_id = sqlc.arg(account_id)
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR block_id < CAST(sqlc.arg(before) AS TEXT))
ORDER BY block_id DESC
LIMIT sqlc.arg(page_limit);

-- ListBlockersPage pages the edges into a blocked account in block order; an empty after starts
-- at the first.
-- name: ListBlockersPage :many
SELECT blocker_account_id, blocked_account_id, block_id
FROM block_edges
WHERE blocked_account_id = sqlc.arg(account_id)
  AND block_id > CAST(sqlc.arg(after) AS TEXT)
ORDER BY block_id
LIMIT sqlc.arg(page_limit);

-- name: UpsertMute :exec
INSERT INTO mutes (account_id, target_account_id, hide_notifications, expires_at, created_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (account_id, target_account_id) DO UPDATE SET
  hide_notifications = excluded.hide_notifications,
  expires_at = excluded.expires_at,
  created_at = excluded.created_at;

-- DeleteMute deletes the mute and reports, by the rows it deleted, whether there was one.
-- name: DeleteMute :execrows
DELETE FROM mutes WHERE account_id = ? AND target_account_id = ?;

-- DeleteExpiredMutes deletes at most batch_limit mutes that have expired at now, the earliest
-- expired first, and returns them.
-- name: DeleteExpiredMutes :many
DELETE FROM mutes
WHERE rowid IN (
  SELECT m.rowid FROM mutes AS m
  WHERE m.expires_at <= CAST(sqlc.arg(now) AS INTEGER)
  ORDER BY m.expires_at
  LIMIT sqlc.arg(batch_limit)
)
RETURNING account_id, target_account_id, hide_notifications, expires_at, created_at;

-- GetActiveMute returns the mute unless it has expired at now.
-- name: GetActiveMute :one
SELECT account_id, target_account_id, hide_notifications, expires_at, created_at
FROM mutes
WHERE account_id = sqlc.arg(account_id) AND target_account_id = sqlc.arg(target_account_id)
  AND (expires_at IS NULL OR expires_at > CAST(sqlc.arg(now) AS INTEGER));

-- ListActiveMutesPage pages the mutes of an account that have not expired at now, newest first
-- by (created_at, target_account_id); an empty before_target_account_id starts at the newest.
-- name: ListActiveMutesPage :many
SELECT account_id, target_account_id, hide_notifications, expires_at, created_at
FROM mutes
WHERE account_id = sqlc.arg(account_id)
  AND (expires_at IS NULL OR expires_at > CAST(sqlc.arg(now) AS INTEGER))
  AND (CAST(sqlc.arg(before_target_account_id) AS TEXT) = ''
    OR created_at < sqlc.arg(before_created_at)
    OR (created_at = sqlc.arg(before_created_at) AND target_account_id < CAST(sqlc.arg(before_target_account_id) AS TEXT)))
ORDER BY created_at DESC, target_account_id DESC
LIMIT sqlc.arg(page_limit);

-- ListActiveMutersPage pages the mutes of a target that have not expired at now, by the
-- muting account; an empty after starts at the first.
-- name: ListActiveMutersPage :many
SELECT account_id, target_account_id, hide_notifications, expires_at, created_at
FROM mutes
WHERE target_account_id = sqlc.arg(target_account_id)
  AND (expires_at IS NULL OR expires_at > CAST(sqlc.arg(now) AS INTEGER))
  AND account_id > CAST(sqlc.arg(after) AS TEXT)
ORDER BY account_id
LIMIT sqlc.arg(page_limit);

-- ListAllMutes returns every mute, expired ones included, ordered by account and target, as the
-- source of other modules' projections.
-- name: ListAllMutes :many
SELECT account_id, target_account_id, hide_notifications, expires_at, created_at
FROM mutes ORDER BY account_id, target_account_id;

-- Domain mutes (current value, FR-304).

-- InsertDomainMute inserts nothing when the account already mutes the domain.
-- name: InsertDomainMute :execrows
INSERT INTO account_domain_mutes (account_id, domain, created_at) VALUES (?, ?, ?)
ON CONFLICT (account_id, domain) DO NOTHING;

-- DeleteDomainMute deletes the domain mute and reports, by the rows it deleted, whether there
-- was one.
-- name: DeleteDomainMute :execrows
DELETE FROM account_domain_mutes WHERE account_id = ? AND domain = ?;

-- name: CountDomainMutes :one
SELECT COUNT(*) FROM account_domain_mutes WHERE account_id = ?;

-- MutesAnyDomain reports whether the account mutes one of the domains (a JSON array).
-- name: MutesAnyDomain :one
SELECT EXISTS (
  SELECT 1 FROM account_domain_mutes
  WHERE account_id = sqlc.arg(account_id) AND domain IN (SELECT value FROM json_each(CAST(sqlc.arg(domains) AS TEXT)))
);

-- ListDomainMutesPage pages the domain mutes of an account in the order of their domains; an
-- empty before_domain starts at the first.
-- name: ListDomainMutesPage :many
SELECT account_id, domain, created_at FROM account_domain_mutes
WHERE account_id = sqlc.arg(account_id) AND (CAST(sqlc.arg(before_domain) AS TEXT) = '' OR domain > CAST(sqlc.arg(before_domain) AS TEXT))
ORDER BY domain
LIMIT sqlc.arg(page_limit);

-- ListAllDomainMutes returns every domain mute, ordered by account and domain, as the source of
-- other modules' projections.
-- name: ListAllDomainMutes :many
SELECT account_id, domain, created_at FROM account_domain_mutes ORDER BY account_id, domain;
