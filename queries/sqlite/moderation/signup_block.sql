-- SPDX-License-Identifier: AGPL-3.0-only

-- The sign-up block list (data-model 4.3, FR-719). Blocks and their lifts are events: nothing
-- updates a row.

-- name: InsertSignupBlock :exec
INSERT INTO signup_blocks (
  id, kind, value, severity, comment, moderation_action_id, created_by_user_id, expires_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSignupBlock :one
SELECT id, kind, value, severity, comment, moderation_action_id, created_by_user_id, expires_at, created_at
FROM signup_blocks WHERE id = ?;

-- InsertSignupBlockLift inserts nothing when the block is already lifted.
-- name: InsertSignupBlockLift :execrows
INSERT INTO signup_block_lifts (block_id, lifted_by_user_id, lifted_at) VALUES (?, ?, ?)
ON CONFLICT (block_id) DO NOTHING;

-- ListSignupBlocks returns the blocks with their lifts, newest first: those of a kind (empty for
-- every kind), only those in effect at in_effect_at unless it is 0, and those before the block
-- before (empty for the first page).
-- name: ListSignupBlocks :many
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at, l.block_id AS lift_block_id, l.lifted_by_user_id, l.lifted_at
FROM signup_blocks b
LEFT JOIN signup_block_lifts l ON l.block_id = b.id
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR b.kind = sqlc.arg(kind))
  AND (CAST(sqlc.arg(in_effect_at) AS INTEGER) = 0
    OR (l.block_id IS NULL AND (b.expires_at IS NULL OR b.expires_at > sqlc.arg(in_effect_at))))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR b.id < sqlc.arg(before))
ORDER BY b.id DESC
LIMIT sqlc.arg(lim);

-- name: GetSignupBlockRecord :one
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at, l.block_id AS lift_block_id, l.lifted_by_user_id, l.lifted_at
FROM signup_blocks b
LEFT JOIN signup_block_lifts l ON l.block_id = b.id
WHERE b.id = ?;

-- ListSignupBlocksInEffect returns the blocks in effect at now that may match a sign-up: the
-- email_domain blocks of the domains (a JSON array of strings), the email_hash blocks of
-- email_hash, and every ip_range block.
-- name: ListSignupBlocksInEffect :many
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at
FROM signup_blocks b
WHERE NOT EXISTS (SELECT 1 FROM signup_block_lifts l WHERE l.block_id = b.id)
  AND (b.expires_at IS NULL OR b.expires_at > CAST(sqlc.arg(now) AS INTEGER))
  AND ((b.kind = 'email_domain' AND b.value IN (SELECT value FROM json_each(CAST(sqlc.arg(domains) AS TEXT))))
    OR (b.kind = 'email_hash' AND b.value = CAST(sqlc.arg(email_hash) AS TEXT))
    OR b.kind = 'ip_range')
ORDER BY b.id;

-- ListSignupBlocksInEffectOfValues returns the blocks of a kind in effect at now whose value is
-- one of vals (a JSON array of strings), for an import to skip the values already blocked.
-- name: ListSignupBlocksInEffectOfValues :many
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at
FROM signup_blocks b
WHERE b.kind = CAST(sqlc.arg(kind) AS TEXT)
  AND b.value IN (SELECT value FROM json_each(CAST(sqlc.arg(vals) AS TEXT)))
  AND NOT EXISTS (SELECT 1 FROM signup_block_lifts l WHERE l.block_id = b.id)
  AND (b.expires_at IS NULL OR b.expires_at > CAST(sqlc.arg(now) AS INTEGER))
ORDER BY b.id;
