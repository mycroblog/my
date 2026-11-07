-- SPDX-License-Identifier: AGPL-3.0-only

-- The aliases of local accounts (current value), ADR-0089.

-- InsertAccountAlias inserts nothing when the account has the alias already.
-- name: InsertAccountAlias :execrows
INSERT INTO account_aliases (account_id, alias_uri, added_at) VALUES (?, ?, ?)
ON CONFLICT (account_id, alias_uri) DO NOTHING;

-- name: DeleteAccountAlias :execrows
DELETE FROM account_aliases WHERE account_id = ? AND alias_uri = ?;

-- name: CountAccountAliases :one
SELECT COUNT(*) FROM account_aliases WHERE account_id = ?;

-- name: ListAccountAliases :many
SELECT account_id, alias_uri, added_at FROM account_aliases WHERE account_id = ?
ORDER BY added_at, alias_uri;

-- name: HasAccountAlias :one
SELECT EXISTS (SELECT 1 FROM account_aliases WHERE account_id = ? AND alias_uri = ?);
