-- SPDX-License-Identifier: AGPL-3.0-only

-- ListSealedInstanceSecrets returns a page of instance_secrets.sealed_secret ordered by id.
-- name: ListSealedInstanceSecrets :many
SELECT id AS row_id, sealed_secret AS sealed FROM instance_secrets WHERE id > CAST(sqlc.arg(after) AS TEXT) ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedInstanceSecrets rewrites instance_secrets.sealed_secret of a row that still holds old_sealed.
-- name: ReplaceSealedInstanceSecrets :execrows
UPDATE instance_secrets SET sealed_secret = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_secret = sqlc.arg(old_sealed);
