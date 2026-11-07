-- SPDX-License-Identifier: AGPL-3.0-only

-- ListSealedAccountPrivateKeys returns a page of local_account_private_keys.private_key_enc ordered by account_key_id.
-- name: ListSealedAccountPrivateKeys :many
SELECT account_key_id AS row_id, private_key_enc AS sealed FROM local_account_private_keys WHERE account_key_id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY account_key_id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedAccountPrivateKeys rewrites local_account_private_keys.private_key_enc of a row that still holds old_sealed.
-- name: ReplaceSealedAccountPrivateKeys :execrows
UPDATE local_account_private_keys SET private_key_enc = sqlc.arg(sealed) WHERE account_key_id = sqlc.arg(row_id) AND private_key_enc = sqlc.arg(old_sealed);
