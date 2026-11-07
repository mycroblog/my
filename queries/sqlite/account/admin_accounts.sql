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
  AND (CAST(sqlc.arg(domain) AS TEXT) = '' OR a.domain = sqlc.arg(domain))
  AND (CAST(sqlc.arg(origin) AS TEXT) = ''
    OR (sqlc.arg(origin) = 'local' AND a.domain = '')
    OR (sqlc.arg(origin) = 'remote' AND a.domain <> ''))
  AND (CAST(sqlc.arg(username_prefix) AS TEXT) = '' OR EXISTS (
    SELECT 1 FROM account_handles h
    WHERE h.account_id = a.id AND substr(h.username_normalized, 1, length(sqlc.arg(username_prefix))) = sqlc.arg(username_prefix)))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR a.id < sqlc.arg(before))
ORDER BY a.id DESC
LIMIT sqlc.arg(lim);

-- ListAccountIDsInLifecycleState returns the accounts in a lifecycle state newest first, before
-- the account before (empty for the first page).
-- name: ListAccountIDsInLifecycleState :many
SELECT account_id FROM account_lifecycle_states
WHERE state = sqlc.arg(state) AND (CAST(sqlc.arg(before) AS TEXT) = '' OR account_id < sqlc.arg(before))
ORDER BY account_id DESC
LIMIT sqlc.arg(lim);
