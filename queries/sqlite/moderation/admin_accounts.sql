-- SPDX-License-Identifier: AGPL-3.0-only

-- The account search of the moderators (FR-714, #344). Each query reads the tables of one module
-- and returns IDs newest first in keyset pages; internal/app/identity.AccountAdmin combines them.

-- ListRestrictedAccountIDs returns the accounts on which their own actions impose a restriction
-- in effect at now, newest first, before the account before (empty for the first page).
-- name: ListRestrictedAccountIDs :many
SELECT DISTINCT account_id FROM account_restrictions
WHERE restriction = sqlc.arg(restriction)
  AND (effective_until IS NULL OR effective_until > CAST(sqlc.arg(now) AS INTEGER))
  AND (CAST(sqlc.arg(before) AS TEXT) = '' OR account_id < sqlc.arg(before))
ORDER BY account_id DESC
LIMIT sqlc.arg(lim);
