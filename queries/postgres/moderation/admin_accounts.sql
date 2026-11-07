-- SPDX-License-Identifier: AGPL-3.0-only

-- The account search of the moderators (FR-714, #344). Each query reads the tables of one module
-- and returns IDs newest first in keyset pages; internal/app/identity.AccountAdmin combines them.

-- ListRestrictedAccountIDs returns the accounts on which their own actions impose a restriction
-- in effect at now, newest first, before the account before (empty for the first page).
-- name: ListRestrictedAccountIDs :many
SELECT DISTINCT account_id FROM account_restrictions
WHERE restriction = sqlc.arg(restriction)::text
  AND (effective_until IS NULL OR effective_until > sqlc.arg(now)::timestamptz)
  AND (sqlc.narg(before)::uuid IS NULL OR account_id < sqlc.narg(before)::uuid)
ORDER BY account_id DESC
LIMIT sqlc.arg(lim);
