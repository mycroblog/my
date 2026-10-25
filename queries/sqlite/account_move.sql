-- SPDX-License-Identifier: AGPL-3.0-only

-- Account moves (event), their cancellations (event), and account_move_states (projection),
-- ADR-0089.

-- InsertAccountMove inserts nothing when the account already moved to the target.
-- name: InsertAccountMove :execrows
INSERT INTO account_moves (id, account_id, target_account_id, initiated_by, activity_uri, occurred_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (account_id, target_account_id) DO NOTHING;

-- name: GetAccountMove :one
SELECT id, account_id, target_account_id, initiated_by, activity_uri, occurred_at
FROM account_moves WHERE id = ?;

-- GetLatestAccountMove returns the newest move of an account.
-- name: GetLatestAccountMove :one
SELECT id, account_id, target_account_id, initiated_by, activity_uri, occurred_at
FROM account_moves WHERE account_id = ?
ORDER BY occurred_at DESC, id DESC
LIMIT 1;

-- ListAccountMoveRecords returns every move with whether it is cancelled, the source of
-- account_move_states.
-- name: ListAccountMoveRecords :many
SELECT m.id, m.account_id, m.target_account_id, m.initiated_by, m.activity_uri, m.occurred_at,
  EXISTS (SELECT 1 FROM account_move_cancellations c WHERE c.move_id = m.id) AS cancelled
FROM account_moves m
ORDER BY m.account_id, m.occurred_at, m.id;

-- name: GetAccountMoveState :one
SELECT account_id, move_id, target_account_id, since FROM account_move_states WHERE account_id = ?;

-- name: UpsertAccountMoveState :exec
INSERT INTO account_move_states (account_id, move_id, target_account_id, since) VALUES (?, ?, ?, ?)
ON CONFLICT (account_id) DO UPDATE SET
  move_id = excluded.move_id,
  target_account_id = excluded.target_account_id,
  since = excluded.since;

-- name: DeleteAllAccountMoveStates :exec
DELETE FROM account_move_states;

-- name: ListAccountMoveStates :many
SELECT account_id, move_id, target_account_id, since FROM account_move_states ORDER BY account_id;
