-- SPDX-License-Identifier: AGPL-3.0-only

-- ListSealedWebhookSecrets returns a page of webhook_secrets.sealed_secret ordered by id.
-- name: ListSealedWebhookSecrets :many
SELECT id AS row_id, sealed_secret AS sealed FROM webhook_secrets WHERE id > COALESCE(sqlc.narg(after)::uuid, '00000000-0000-0000-0000-000000000000') ORDER BY id LIMIT sqlc.arg(page_limit);

-- ReplaceSealedWebhookSecrets rewrites webhook_secrets.sealed_secret of a row that still holds old_sealed.
-- name: ReplaceSealedWebhookSecrets :execrows
UPDATE webhook_secrets SET sealed_secret = sqlc.arg(sealed) WHERE id = sqlc.arg(row_id) AND sealed_secret = sqlc.arg(old_sealed);
