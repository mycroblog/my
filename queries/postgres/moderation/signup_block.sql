-- SPDX-License-Identifier: AGPL-3.0-only

-- The sign-up block list (data-model 4.3, FR-719). Blocks and their lifts are events: nothing
-- updates a row.

-- name: InsertSignupBlock :exec
INSERT INTO signup_blocks (
  id, kind, value, severity, comment, moderation_action_id, created_by_user_id, expires_at, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetSignupBlock :one
SELECT id, kind, value, severity, comment, moderation_action_id, created_by_user_id, expires_at, created_at
FROM signup_blocks WHERE id = $1;

-- InsertSignupBlockLift inserts nothing when the block is already lifted.
-- name: InsertSignupBlockLift :execrows
INSERT INTO signup_block_lifts (block_id, lifted_by_user_id, lifted_at) VALUES ($1, $2, $3)
ON CONFLICT (block_id) DO NOTHING;

-- ListSignupBlocks returns the blocks with their lifts, newest first: those of a kind (empty for
-- every kind), only those in effect at in_effect_at unless it is NULL, and those before the block
-- before (empty for the first page).
-- name: ListSignupBlocks :many
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at, l.block_id AS lift_block_id, l.lifted_by_user_id, l.lifted_at
FROM signup_blocks b
LEFT JOIN signup_block_lifts l ON l.block_id = b.id
WHERE (sqlc.arg(kind)::text = '' OR b.kind = sqlc.arg(kind)::text)
  AND (sqlc.narg(in_effect_at)::timestamptz IS NULL
    OR (l.block_id IS NULL AND (b.expires_at IS NULL OR b.expires_at > sqlc.narg(in_effect_at)::timestamptz)))
  AND (sqlc.narg(before)::uuid IS NULL OR b.id < sqlc.narg(before)::uuid)
ORDER BY b.id DESC
LIMIT sqlc.arg(lim);

-- name: GetSignupBlockRecord :one
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at, l.block_id AS lift_block_id, l.lifted_by_user_id, l.lifted_at
FROM signup_blocks b
LEFT JOIN signup_block_lifts l ON l.block_id = b.id
WHERE b.id = $1;

-- ListSignupBlocksInEffect returns the blocks in effect at now that may match a sign-up: the
-- email_domain blocks of the domains, the email_hash blocks of email_hash, and every ip_range
-- block.
-- name: ListSignupBlocksInEffect :many
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at
FROM signup_blocks b
WHERE NOT EXISTS (SELECT 1 FROM signup_block_lifts l WHERE l.block_id = b.id)
  AND (b.expires_at IS NULL OR b.expires_at > sqlc.arg(now)::timestamptz)
  AND ((b.kind = 'email_domain' AND b.value = ANY(sqlc.arg(domains)::text[]))
    OR (b.kind = 'email_hash' AND b.value = sqlc.arg(email_hash)::text)
    OR b.kind = 'ip_range')
ORDER BY b.id;

-- ListSignupBlocksInEffectOfValues returns the blocks of a kind in effect at now whose value is
-- one of vals, for an import to skip the values already blocked.
-- name: ListSignupBlocksInEffectOfValues :many
SELECT b.id, b.kind, b.value, b.severity, b.comment, b.moderation_action_id, b.created_by_user_id,
  b.expires_at, b.created_at
FROM signup_blocks b
WHERE b.kind = sqlc.arg(kind)::text
  AND b.value = ANY(sqlc.arg(vals)::text[])
  AND NOT EXISTS (SELECT 1 FROM signup_block_lifts l WHERE l.block_id = b.id)
  AND (b.expires_at IS NULL OR b.expires_at > sqlc.arg(now)::timestamptz)
ORDER BY b.id;
