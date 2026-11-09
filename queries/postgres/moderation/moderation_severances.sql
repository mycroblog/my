-- SPDX-License-Identifier: AGPL-3.0-only

-- The follows that suspensions cut (event, FR-717).

-- InsertRelationshipSeverance inserts nothing when the account already has a severance of the
-- same action.
-- name: InsertRelationshipSeverance :execrows
INSERT INTO relationship_severances (id, domain_action_id, action_id, account_id, followers_lost, following_lost, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING;

-- InsertRelationshipSeverances is InsertRelationshipSeverance of the accounts of one action (the
-- unnests of a select list advance together); it returns the accounts it inserted.
-- name: InsertRelationshipSeverances :many
INSERT INTO relationship_severances (id, domain_action_id, action_id, account_id, followers_lost, following_lost, occurred_at)
SELECT unnest(sqlc.arg(ids)::uuid[]), sqlc.narg(domain_action_id)::uuid, sqlc.narg(action_id)::uuid, unnest(sqlc.arg(account_ids)::uuid[]),
  unnest(sqlc.arg(followers_lost)::int[]), unnest(sqlc.arg(following_lost)::int[]), sqlc.arg(occurred_at)::timestamptz
ON CONFLICT DO NOTHING
RETURNING account_id;

-- GetRelationshipSeverance returns a severance of an account with the domain or the account that
-- its action suspended.
-- name: GetRelationshipSeverance :one
SELECT s.id, s.domain_action_id, s.action_id, s.account_id, s.followers_lost, s.following_lost, s.occurred_at,
  d.domain AS target_domain, a.target_account_id
FROM relationship_severances s
LEFT JOIN domain_moderation_actions d ON d.id = s.domain_action_id
LEFT JOIN moderation_actions a ON a.id = s.action_id
WHERE s.id = $1 AND s.account_id = $2;

-- name: EraseRelationshipSeverances :exec
DELETE FROM relationship_severances WHERE account_id = sqlc.arg(account_id)::uuid;
