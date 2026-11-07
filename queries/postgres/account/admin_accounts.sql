-- SPDX-License-Identifier: AGPL-3.0-only

-- The account search of the moderators (FR-714, #344). Each query reads the tables of one module
-- and returns IDs newest first in keyset pages; internal/app/identity.AccountAdmin combines them.

-- SearchAccountIDs returns the accounts newest first, without the instance actor: those on a
-- domain (empty for any), local or remote (origin empty for both), whose handle's normalized
-- username starts with username_prefix (empty for any), before the account before (empty for
-- the first page).
-- name: SearchAccountIDs :many
SELECT a.id FROM accounts a
WHERE NOT (a.domain = '' AND a.actor_type = 'application')
  AND (sqlc.arg(domain)::text = '' OR a.domain = sqlc.arg(domain)::text)
  AND (sqlc.arg(origin)::text = ''
    OR (sqlc.arg(origin)::text = 'local' AND a.domain = '')
    OR (sqlc.arg(origin)::text = 'remote' AND a.domain <> ''))
  AND (sqlc.arg(username_prefix)::text = '' OR EXISTS (
    SELECT 1 FROM account_handles h
    WHERE h.account_id = a.id AND left(h.username_normalized, length(sqlc.arg(username_prefix)::text)) = sqlc.arg(username_prefix)::text))
  AND (sqlc.narg(before)::uuid IS NULL OR a.id < sqlc.narg(before)::uuid)
ORDER BY a.id DESC
LIMIT sqlc.arg(lim);

-- ListAccountIDsInLifecycleState returns the accounts in a lifecycle state newest first, before
-- the account before (empty for the first page).
-- name: ListAccountIDsInLifecycleState :many
SELECT account_id FROM account_lifecycle_states
WHERE state = sqlc.arg(state)::text AND (sqlc.narg(before)::uuid IS NULL OR account_id < sqlc.narg(before)::uuid)
ORDER BY account_id DESC
LIMIT sqlc.arg(lim);
